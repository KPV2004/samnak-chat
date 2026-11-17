package task

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "task", task.Migrations).
var Migrations = migrations.FS

// The due scan runs every five minutes on one server (TASK-06).
const (
	DueScanJobKind     = "task.due_scan"
	DueScanJobInterval = 5 * time.Minute
)

// Module is the task module. It is optional: with it off, chat has no task
// card, command or menu item, and old task cards show their fallback text
// (PLT-11).
type Module struct {
	services *app.Services
	uc       *usecase.Service
	log      *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{} }

func (*Module) Name() string { return "task" }

// Requires is empty on purpose: chat, auth and workspace are core modules, and
// every service the module needs from them has a safe fallback.
func (*Module) Requires() []string { return nil }

// Provide exposes nothing: other modules listen for events.
func (*Module) Provide(*app.Services) {}

// Init builds the use cases and registers what the module adds to others: the
// card kind, the command and message action in chat, the notification rules
// and the due-scan job. It does no I/O: with no "db" in d.Platform (the
// openapi command) the repository fails when called, which never happens
// because no request is served. A service that is missing is skipped.
func (m *Module) Init(d *app.Deps) error {
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	m.services = d.Services
	pool, _ := d.Platform["db"].(*pgxpool.Pool)

	deps := usecase.Deps{
		Tx: d.Tx, Repo: postgres.New(pool), Events: publisher{bus: d.Bus}, Log: m.log, Now: time.Now,
	}
	if access, ok := app.Lookup[app.ChannelAccess](d.Services); ok {
		deps.Channels = access
	}
	var resolver app.MembershipResolver
	if r, ok := app.Lookup[app.MembershipResolver](d.Services); ok {
		resolver = r
	}
	deps.Members = members{resolver}
	var dir auth.Directory
	if v, ok := app.Lookup[auth.Directory](d.Services); ok {
		dir = v
	}
	deps.Directory = directory{dir}
	if cards, ok := app.Lookup[app.CardRegistry](d.Services); ok {
		cards.Register(app.CardKind{Kind: usecase.CardKind, Module: m.Name()})
		deps.Cards = cards
	}
	m.uc = usecase.New(deps)

	if d.Bus != nil {
		d.Bus.Declare(TaskCreated{}, TaskAssigned{}, TaskStatusChanged{}, TaskUpdated{}, TaskDueSoon{}, TaskOverdue{})
	}
	if ext, ok := app.Lookup[app.ChatExtensions](d.Services); ok {
		ext.RegisterCommand(app.Command{
			Name: CommandName, Module: m.Name(), Usage: "/task <title> [@assignee]",
			Description: "Create a task in this channel", Handler: m.command,
		})
		ext.RegisterMessageAction(app.MessageAction{ID: ActionCreateFromMessage, Module: m.Name()})
	}
	if d.Notifications != nil {
		d.Notifications.On(TaskAssigned{}.EventName(), m.notifyAssigned)
		d.Notifications.On(TaskDueSoon{}.EventName(), m.notifyDue(usecase.DueSoon))
		d.Notifications.On(TaskOverdue{}.EventName(), m.notifyDue(usecase.Overdue))
	}
	if d.Jobs != nil {
		d.Jobs.Handle(DueScanJobKind, m.dueScan, app.JobOptions{})
		d.Jobs.Every(DueScanJobInterval, DueScanJobKind, nil)
	}
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

// Subscribe keeps the task card current. Every task event ends in the same
// idempotent step: post the card if there is none, else replace its payload.
// The step runs after the commit (the outbox delivers it), never inside the
// transaction that wrote the task (PLT-10).
func (m *Module) Subscribe(bus app.EventBus) {
	if bus == nil || m.uc == nil {
		return
	}
	for name, ids := range map[string]func(app.Event) (workspaceID, taskID string, ok bool){
		TaskCreated{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskCreated)
			return v.WorkspaceID, v.TaskID, ok
		},
		TaskAssigned{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskAssigned)
			return v.WorkspaceID, v.TaskID, ok
		},
		TaskStatusChanged{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskStatusChanged)
			return v.WorkspaceID, v.TaskID, ok
		},
		TaskUpdated{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskUpdated)
			return v.WorkspaceID, v.TaskID, ok
		},
		TaskDueSoon{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskDueSoon)
			return v.WorkspaceID, v.TaskID, ok
		},
		TaskOverdue{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TaskOverdue)
			return v.WorkspaceID, v.TaskID, ok
		},
	} {
		bus.Subscribe(name, func(ctx context.Context, evt app.Event) error {
			workspaceID, taskID, ok := ids(evt)
			if !ok {
				return nil
			}
			// A client that hung up must not cancel updating the card of a
			// change that is already committed.
			return m.uc.SyncCard(context.WithoutCancel(ctx), workspaceID, taskID)
		})
	}
}

// command is the handler of "/task": it runs with the caller's Principal.
func (m *Module) command(ctx context.Context, in app.CommandInput) (app.CommandResult, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.CommandResult{}, app.ErrUnauthorized
	}
	return app.CommandResult{}, m.uc.CreateFromCommand(ctx, p, in)
}

func (m *Module) notifyAssigned(ctx context.Context, evt app.Event) ([]app.Notification, error) {
	e, ok := evt.(TaskAssigned)
	if !ok {
		return nil, nil
	}
	return m.uc.NotifyAssigned(ctx, usecase.AssignedInput{
		WorkspaceID: e.WorkspaceID, TaskID: e.TaskID, AssigneeID: e.AssigneeID, ActorID: e.ActorID, UpdatedAt: e.UpdatedAt,
	})
}

// notifyDue is the rule of task.due_soon and task.overdue.
func (m *Module) notifyDue(kind usecase.DueKind) app.NotificationRule {
	return func(ctx context.Context, evt app.Event) ([]app.Notification, error) {
		var in usecase.DueInput
		switch e := evt.(type) {
		case TaskDueSoon:
			in = usecase.DueInput{WorkspaceID: e.WorkspaceID, TaskID: e.TaskID, AssigneeID: e.AssigneeID, DueAt: e.DueAt}
		case TaskOverdue:
			in = usecase.DueInput{WorkspaceID: e.WorkspaceID, TaskID: e.TaskID, AssigneeID: e.AssigneeID, DueAt: e.DueAt}
		default:
			return nil, nil
		}
		return m.uc.NotifyDue(ctx, kind, in)
	}
}

// dueScan is the task.due_scan job handler. It ignores its args.
func (m *Module) dueScan(ctx context.Context, _ json.RawMessage) error {
	return m.uc.DueScan(ctx)
}
