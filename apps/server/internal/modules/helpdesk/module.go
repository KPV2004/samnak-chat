package helpdesk

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "helpdesk", helpdesk.Migrations).
var Migrations = migrations.FS

// Module is the helpdesk module. It is optional: with it off, no ticket
// exists, its chat extensions disappear and its cards show their fallback
// text (PLT-11).
type Module struct {
	services *app.Services
	uc       *usecase.Service
	log      *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{} }

func (*Module) Name() string { return "helpdesk" }

// Requires is empty on purpose: auth, workspace and chat are core modules, and
// every service the module takes from them has a safe fallback.
func (*Module) Requires() []string { return nil }

// Provide exposes nothing: other modules listen to the events in events.go.
func (*Module) Provide(*app.Services) {}

// Init builds the use cases and registers the admin action with workspace
// (owners and admins). It does no I/O: with no "db" in d.Platform (the openapi
// command) the repository fails when called, which never happens because no
// request is served.
func (m *Module) Init(d *app.Deps) error {
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	m.services = d.Services
	pool, _ := d.Platform["db"].(*pgxpool.Pool)

	if reg, ok := app.Lookup[workspace.ActionRegistry](d.Services); ok {
		if err := reg.Register(workspace.Action(usecase.ActionAdmin), app.RoleOwner, app.RoleAdmin); err != nil {
			return err
		}
	}
	// The card kind is registered by the module that posts it; chat refuses an
	// unregistered kind (HD-08).
	var cards usecase.Cards
	if reg, ok := app.Lookup[app.CardRegistry](d.Services); ok {
		reg.Register(app.CardKind{Kind: usecase.CardKind, Module: m.Name()})
		cards = reg
	}
	deps := usecase.Deps{
		Tx: d.Tx, Repo: postgres.New(pool), Permissions: permissions{d.Services}, Members: members{d.Services},
		Bus: d.Bus, Storage: d.Storage, Log: m.log, Now: time.Now,
		Cards: cards, Names: names{d.Services},
	}
	// Without chat's reader a source message is refused (HD-03), so the
	// dependency stays nil.
	if r, ok := app.Lookup[app.MessageReader](d.Services); ok {
		deps.Messages = r
	}
	m.uc = usecase.New(deps)
	if d.Bus != nil {
		// These events are published inside transactions, so the outbox must
		// know how to decode them (PLT-10).
		d.Bus.Declare(eventTypes...)
	}
	if d.Notifications != nil {
		// notify reads the rules in Subscribe, after every Init ran. With a
		// no-op registry these lines do nothing (NOTI-01, NOTI-05).
		d.Notifications.On(TicketCreated{}.EventName(), m.uc.Notify)
		d.Notifications.On(TicketAssigned{}.EventName(), m.uc.Notify)
		d.Notifications.On(StatusChanged{}.EventName(), m.uc.Notify)
		d.Notifications.On(CommentAdded{}.EventName(), m.uc.Notify)
	}
	if ext, ok := app.Lookup[app.ChatExtensions](d.Services); ok {
		ext.RegisterCommand(app.Command{
			Name: usecase.CommandName, Module: m.Name(), Usage: "/ticket <title>",
			Description: "Open a helpdesk ticket in this channel", Handler: m.command,
		})
		ext.RegisterMessageAction(app.MessageAction{ID: usecase.ActionCreateFromMessage, Module: m.Name()})
	}
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

// Subscribe keeps both cards of a ticket in step with it: every helpdesk event
// posts a card when there is none and rewrites it otherwise (HD-08). The
// handler works from the stored row, so a late or repeated event is safe.
func (m *Module) Subscribe(bus app.EventBus) {
	if bus == nil || m.uc == nil {
		return
	}
	for name, ids := range map[string]func(app.Event) (workspaceID, ticketID string, ok bool){
		TicketCreated{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TicketCreated)
			return v.WorkspaceID, v.TicketID, ok
		},
		TicketAssigned{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TicketAssigned)
			return v.WorkspaceID, v.TicketID, ok
		},
		StatusChanged{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(StatusChanged)
			return v.WorkspaceID, v.TicketID, ok
		},
		TicketResolved{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TicketResolved)
			return v.WorkspaceID, v.TicketID, ok
		},
		TicketUpdated{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(TicketUpdated)
			return v.WorkspaceID, v.TicketID, ok
		},
		CommentAdded{}.EventName(): func(e app.Event) (string, string, bool) {
			v, ok := e.(CommentAdded)
			return v.WorkspaceID, v.TicketID, ok
		},
	} {
		bus.Subscribe(name, func(ctx context.Context, evt app.Event) error {
			workspaceID, ticketID, ok := ids(evt)
			if !ok {
				return nil
			}
			// The change is committed; a client that hung up must not stop
			// the card.
			return m.uc.SyncCard(context.WithoutCancel(ctx), workspaceID, ticketID)
		})
	}
}

// command runs "/ticket <title>" for the caller of the command.
func (m *Module) command(ctx context.Context, in app.CommandInput) (app.CommandResult, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.CommandResult{}, app.ErrUnauthorized
	}
	return app.CommandResult{}, m.uc.CreateFromCommand(ctx, p, in)
}

// names adapts auth's directory; without it names are empty.
type names struct{ s *app.Services }

func (n names) Names(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	dir, ok := app.Lookup[auth.Directory](n.s)
	if !ok {
		return out
	}
	users, err := dir.Users(ctx, ids)
	if err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = u.DisplayName
	}
	return out
}
