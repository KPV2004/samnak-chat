package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/postgres"
	rtadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/realtime"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/usecase"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "notify", notify.Migrations).
var Migrations = migrations.FS

// The prune job runs daily and deletes read notifications older than 90 days
// (SRS section 8). Every server registers it; jobs.Every keeps it on one.
const (
	PruneJobKind     = "notify.prune"
	PruneJobInterval = 24 * time.Hour
)

// Module is the notify module. It is optional: turning it off leaves every
// other module working, without notifications (NOTI-05).
type Module struct {
	services *app.Services
	// source is the rules other modules registered in Init. It is nil when
	// Deps.Notifications does not collect rules; then nothing is subscribed.
	source app.NotificationRuleSource
	uc     *usecase.Service
	log    *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{} }

func (*Module) Name() string { return "notify" }

// Requires is empty on purpose: notify adds a table of its own and reads what
// other modules publish, so nothing here can stop it from starting.
func (*Module) Requires() []string { return nil }

// Provide exposes nothing: modules talk to notify through rules and events.
func (*Module) Provide(*app.Services) {}

// Init builds the use cases and registers the prune job. It does no I/O: with
// no "db" in d.Platform (the openapi command) the repository fails when
// called, which never happens because no request is served.
func (m *Module) Init(d *app.Deps) error {
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	m.services = d.Services
	pool, _ := d.Platform["db"].(*pgxpool.Pool)
	// The server passes app.NewNotificationRules; a noop registry collects
	// nothing, so the assertion leaves source nil and Subscribe does nothing.
	m.source, _ = d.Notifications.(app.NotificationRuleSource)
	// Without auth the do-not-disturb window reads in UTC (NOTI-07).
	var zones usecase.Directory
	if d.Services != nil {
		if dir, ok := app.Lookup[auth.Directory](d.Services); ok {
			zones = timezones{dir}
		}
	}
	m.uc = usecase.New(d.Tx, postgres.New(pool), m.source, rtadapter.New(d.Realtime, m.log), zones, m.log, time.Now)

	if d.Jobs != nil {
		d.Jobs.Handle(PruneJobKind, m.prune, app.JobOptions{})
		d.Jobs.Every(PruneJobInterval, PruneJobKind, nil)
	}
	return nil
}

// timezones reads each user's time zone from auth.Directory (NOTI-07). Users
// with no zone are left out, so they read in UTC.
type timezones struct{ dir auth.Directory }

func (t timezones) Timezones(ctx context.Context, userIDs []string) (map[string]string, error) {
	users, err := t.dir.Users(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(users))
	for _, u := range users {
		if u.Timezone != "" {
			out[u.ID] = u.Timezone
		}
	}
	return out, nil
}

// prune is the notify.prune job handler. It ignores its args.
func (m *Module) prune(ctx context.Context, _ json.RawMessage) error {
	n, err := m.uc.Prune(ctx)
	if err != nil {
		return err
	}
	m.log.InfoContext(ctx, "notify: pruned read notifications", "deleted", n)
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

// Subscribe registers one bus handler per event name that has rules. A rule
// that fails is logged by the use case and never reaches the publisher; the
// handler only returns storage errors, which the bus logs (NOTI-01, NOTI-02).
func (m *Module) Subscribe(bus app.EventBus) {
	if bus == nil || m.source == nil || m.uc == nil {
		return
	}
	for name := range m.source.Rules() {
		name := name
		bus.Subscribe(name, func(ctx context.Context, evt app.Event) error {
			if evt == nil || evt.EventName() != name {
				return nil
			}
			return m.uc.Dispatch(ctx, evt)
		})
	}
}
