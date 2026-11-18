package helpdesk

import (
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
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
	m.uc = usecase.New(usecase.Deps{
		Tx: d.Tx, Repo: postgres.New(pool), Permissions: permissions{d.Services}, Members: members{d.Services},
		Log: m.log, Now: time.Now,
	})
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

// Subscribe is empty until the card and notification handlers land (M5-03).
func (*Module) Subscribe(app.EventBus) {}
