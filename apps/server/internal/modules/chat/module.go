package chat

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/postgres"
	rtadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/realtime"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "chat", chat.Migrations).
var Migrations = migrations.FS

// Module is the chat module (core, always on).
type Module struct {
	services *app.Services
	uc       *usecase.Service
	ext      *usecase.Extensions
	log      *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{ext: usecase.NewExtensions()} }

func (*Module) Name() string { return "chat" }

// Requires is empty on purpose: workspace and auth are core modules, and every
// service chat needs from them has a safe fallback.
func (*Module) Requires() []string { return nil }

func (m *Module) Provide(s *app.Services) {
	app.Provide[app.CardRegistry](s, newCardRegistry(m))
	app.Provide[app.ChatExtensions](s, m.ext)
	app.Provide[app.ChannelAccess](s, channelAccess{m})
	app.Provide[app.MessageReader](s, messageReader{m})
}

// Init builds the use cases. It does no I/O: with no "db" in d.Platform (the
// openapi command) the repository fails when called, which never happens
// because no request is served.
func (m *Module) Init(d *app.Deps) error {
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	if reg, ok := app.Lookup[workspace.ActionRegistry](d.Services); ok {
		everyone := []app.Role{app.RoleOwner, app.RoleAdmin, app.RoleManager, app.RoleMember}
		admins := []app.Role{app.RoleOwner, app.RoleAdmin}
		for _, a := range []struct {
			action workspace.Action
			roles  []app.Role
		}{
			{ActionChannelCreate, everyone},
			{ActionChannelManage, admins},
			{ActionMessageModerate, admins},
		} {
			if err := reg.Register(a.action, a.roles...); err != nil {
				return err
			}
		}
	}
	pool, _ := d.Platform["db"].(*pgxpool.Pool)
	events := rtadapter.New(d.Realtime, m.log, publisher{bus: d.Bus, log: m.log}.messageCreated)
	m.uc = usecase.New(d.Tx, postgres.New(pool), members{d.Services}, permissions{d.Services}, directory{d.Services}, events, d.Storage, m.log, time.Now).WithExtensions(m.ext)
	m.services = d.Services
	if d.Notifications != nil {
		// notify reads the rules in Subscribe, after every Init ran. With a
		// no-op registry this line does nothing and chat works as before
		// (NOTI-01, NOTI-05).
		d.Notifications.On(MessageCreated{}.EventName(), m.notifyMessageCreated)
	}
	if d.Realtime != nil {
		d.Realtime.Handle("typing", rtadapter.TypingHandler(m.uc))
	}
	return nil
}

// notifyMessageCreated is chat's notification rule (NOTI-06). It turns a
// committed message into one notification per recipient, at most; notify
// decides how to store and deliver them.
func (m *Module) notifyMessageCreated(ctx context.Context, evt app.Event) ([]app.Notification, error) {
	e, ok := evt.(MessageCreated)
	if !ok {
		return nil, nil
	}
	return m.uc.NotifyMessageCreated(ctx, usecase.NotifyInput{
		WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, MessageID: e.MessageID, Seq: e.Seq, SenderID: e.SenderID,
		ThreadRootID: e.ThreadRootID, Mentions: e.Mentions, MentionsChannel: e.MentionsChannel,
	})
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.ext, m.log).Register(api, m.services)
}

// Subscribe removes a member from every channel when the workspace removes
// them (WSP-04).
func (m *Module) Subscribe(bus app.EventBus) {
	if bus == nil {
		return
	}
	bus.Subscribe(workspace.MemberRemoved{}.EventName(), func(ctx context.Context, evt app.Event) error {
		e, ok := evt.(workspace.MemberRemoved)
		if !ok {
			return nil
		}
		return m.uc.MemberRemoved(ctx, e.WorkspaceID, e.UserID)
	})
}

// members adapts app.MembershipResolver to usecase.Workspace. It looks the
// service up on every call; a missing one is an error, never "everyone is in".
type members struct{ s *app.Services }

func (m members) IsActiveMember(ctx context.Context, workspaceID, userID string) (bool, error) {
	r, ok := app.Lookup[app.MembershipResolver](m.s)
	if !ok {
		return false, errors.New("app.MembershipResolver is not provided")
	}
	if _, err := r.Role(ctx, workspaceID, userID); err != nil {
		if errors.Is(err, app.ErrForbidden) || errors.Is(err, app.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m members) MemberRole(ctx context.Context, workspaceID, userID string) (app.Role, bool, error) {
	r, ok := app.Lookup[app.MembershipResolver](m.s)
	if !ok {
		return "", false, errors.New("app.MembershipResolver is not provided")
	}
	role, err := r.Role(ctx, workspaceID, userID)
	if err != nil {
		if errors.Is(err, app.ErrForbidden) || errors.Is(err, app.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return role, true, nil
}

// permissions adapts workspace.Authorizer; without it nothing is allowed.
type permissions struct{ s *app.Services }

func (p permissions) Can(pr app.Principal, action string) bool {
	a, ok := app.Lookup[workspace.Authorizer](p.s)
	return ok && a.Can(pr, workspace.Action(action))
}

// directory adapts auth.Directory; without it names are empty.
type directory struct{ s *app.Services }

func (d directory) Names(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	for id, p := range d.Profiles(ctx, ids) {
		out[id] = p.DisplayName
	}
	return out
}

func (d directory) Profiles(ctx context.Context, ids []string) map[string]usecase.Profile {
	out := map[string]usecase.Profile{}
	dir, ok := app.Lookup[auth.Directory](d.s)
	if !ok {
		return out
	}
	users, err := dir.Users(ctx, ids)
	if err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = usecase.Profile{DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
	}
	return out
}

// publisher puts chat.message_created on the bus. The bus never fails the
// publisher, so an error is only logged.
type publisher struct {
	bus app.Publisher
	log *slog.Logger
}

func (p publisher) messageCreated(ctx context.Context, ch domain.Channel, m usecase.MessageView) {
	if p.bus == nil {
		return
	}
	evt := MessageCreated{
		WorkspaceID: m.WorkspaceID, ChannelID: m.ChannelID, MessageID: m.ID, Seq: m.Seq, SenderID: m.SenderID,
		ChannelType: string(ch.Type), ThreadRootID: m.ThreadRootID, Mentions: m.Mentions, MentionsChannel: m.MentionsChannel,
	}
	if evt.Mentions == nil {
		evt.Mentions = []string{}
	}
	if err := p.bus.Publish(ctx, evt); err != nil {
		p.log.ErrorContext(ctx, "chat: publish failed", "event", evt.EventName(), "error", err)
	}
}
