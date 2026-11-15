package workspace

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "workspace", workspace.Migrations).
var Migrations = migrations.FS

// Module is the workspace module (core, always on).
type Module struct {
	policy   *Policy
	services *app.Services
	uc       *usecase.Service
	log      *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{policy: NewAuthorizer()} }

func (*Module) Name() string { return "workspace" }

// Requires is empty on purpose: the user directory is optional (member names
// fall back to empty strings). The auth module is a core module and is
// always enabled with this one.
func (*Module) Requires() []string { return nil }

func (m *Module) Provide(s *app.Services) {
	app.Provide[app.MembershipResolver](s, resolver{m})
	app.Provide[Authorizer](s, m.policy)
	app.Provide[ActionRegistry](s, m.policy)
}

// Init builds the use cases. It does no I/O: with no "db" in d.Platform (the
// openapi command) the repository fails when called, which never happens
// because no request is served.
func (m *Module) Init(d *app.Deps) error {
	m.services = d.Services
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	pool, _ := d.Platform["db"].(*pgxpool.Pool)
	m.uc = usecase.New(d.Tx, postgres.New(pool), directory{d.Services}, busEvents{bus: d.Bus, log: m.log}, m.log, time.Now)
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

func (*Module) Subscribe(app.EventBus) {}

// resolver is the app.MembershipResolver. It reads m.uc on every call because
// Provide runs before Init.
type resolver struct{ m *Module }

func (r resolver) Role(ctx context.Context, workspaceID, userID string) (app.Role, error) {
	if r.m.uc == nil {
		return "", errors.New("workspace: module is not initialised")
	}
	return r.m.uc.ActiveRole(ctx, workspaceID, userID)
}

// directory adapts auth.Directory to usecase.Directory. It looks the service
// up on every call, so a disabled or late auth module is just an error the
// use case handles.
type directory struct{ s *app.Services }

func (d directory) Users(ctx context.Context, ids []string) (map[string]usecase.User, error) {
	dir, ok := app.Lookup[auth.Directory](d.s)
	if !ok {
		return nil, errors.New("auth.Directory is not provided")
	}
	list, err := dir.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]usecase.User, len(list))
	for _, u := range list {
		out[u.ID] = usecase.User{Email: u.Email, DisplayName: u.DisplayName}
	}
	return out, nil
}

// busEvents publishes member events on the bus. The bus never fails the
// publisher, so an error is only logged.
type busEvents struct {
	bus app.Publisher
	log *slog.Logger
}

func (b busEvents) publish(ctx context.Context, evt app.Event) {
	if b.bus == nil {
		return
	}
	if err := b.bus.Publish(ctx, evt); err != nil {
		b.log.ErrorContext(ctx, "workspace: publish failed", "event", evt.EventName(), "error", err)
	}
}

func (b busEvents) MemberRemoved(ctx context.Context, workspaceID, userID, actorID string) {
	b.publish(ctx, MemberRemoved{WorkspaceID: workspaceID, UserID: userID, ActorID: actorID})
}

func (b busEvents) MemberDeactivated(ctx context.Context, workspaceID, userID, actorID string) {
	b.publish(ctx, MemberDeactivated{WorkspaceID: workspaceID, UserID: userID, ActorID: actorID})
}
