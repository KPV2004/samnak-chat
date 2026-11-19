// Package auth owns accounts, devices and tokens (SRS section 5).
// This file is the module's composition root; api.go is the only part other
// modules may import besides events.go.
package auth

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/jwt"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/memory"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/postgres"
	redisadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/redis"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
)

// Migrations holds the goose files of this module under "auth/". Pass it to
// migrations.Run with the owner "auth".
var Migrations embed.FS = migrations.FS

const (
	// Failed logins allowed per email and per IP in one window (AUTH-06).
	loginFailureLimit  = 5
	loginFailureWindow = time.Minute
)

// Module is the auth module.
type Module struct {
	opts     options
	svc      *usecase.Service
	services *app.Services
	logger   *slog.Logger
}

var _ app.Module = (*Module)(nil)

// defaultWebURL is where the web app runs in development (SRS 15, M6).
const defaultWebURL = "http://localhost:5173"

type options struct {
	now     func() time.Time
	limiter usecase.LoginLimiter
	webURL  string
}

// Option changes how New builds the module. Both exist for tests and for
// swapping the limiter for a shared one.
type Option func(*options)

// WithClock sets the time source.
func WithClock(now func() time.Time) Option { return func(o *options) { o.now = now } }

// WithLoginLimiter replaces the failed-login counter, in memory or Redis.
func WithLoginLimiter(l usecase.LoginLimiter) Option { return func(o *options) { o.limiter = l } }

// WithWebURL sets the base of the links in mail, such as
// https://chat.example.com. Without it the WEB_URL variable is used, then
// http://localhost:5173.
func WithWebURL(u string) Option { return func(o *options) { o.webURL = u } }

func New(opts ...Option) *Module {
	m := &Module{}
	for _, o := range opts {
		o(&m.opts)
	}
	return m
}

func (m *Module) Name() string       { return "auth" }
func (m *Module) Requires() []string { return nil }

// Provide registers app.Authenticator and Directory. Both delegate to the use
// case service that Init builds, so they work once Init has run.
func (m *Module) Provide(s *app.Services) {
	app.Provide[app.Authenticator](s, authenticator{m})
	app.Provide[Directory](s, directory{m})
}

// Init builds the service. It does no I/O, so `server openapi` can run it with
// no database: without a pool the use cases exist but cannot be called.
//
// It reads the database pool from d.Platform["db"] and the token secret from
// d.Platform["jwt_secret"], falling back to the JWT_SECRET variable. When
// d.Platform["redis"] holds a *redis.Client, failed logins are counted there,
// shared by every server; otherwise they are counted in memory. WithLoginLimiter
// overrides both.
//
// The email job (AUTH-09) is registered only when d.Jobs is set. Mail is sent
// with d.Mailer; without one the job logs and skips.
func (m *Module) Init(d *app.Deps) error {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	pool, _ := d.Platform["db"].(*pgxpool.Pool)
	secret, _ := d.Platform["jwt_secret"].(string)
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if pool != nil && secret == "" {
		return errors.New("auth: JWT_SECRET is required")
	}
	now := m.opts.now
	if now == nil {
		now = time.Now
	}
	limiter := m.opts.limiter
	if limiter == nil {
		if rdb, ok := d.Platform["redis"].(*goredis.Client); ok && rdb != nil {
			limiter = redisadapter.NewLoginLimiter(rdb, loginFailureLimit, loginFailureWindow)
		} else {
			limiter = memory.NewLoginLimiter(loginFailureLimit, loginFailureWindow, now)
		}
	}
	var repo usecase.Repository
	if pool != nil {
		repo = postgres.NewRepository(pool)
	}
	webURL := m.opts.webURL
	if webURL == "" {
		webURL = os.Getenv("WEB_URL")
	}
	if webURL == "" {
		webURL = defaultWebURL
	}
	m.svc = usecase.NewService(usecase.Deps{
		Repo: repo, Tx: d.Tx, Limiter: limiter, Tokens: jwt.New([]byte(secret)),
		Events: busEvents{bus: d.Bus, log: logger}, Now: now,
		Jobs: d.Jobs, Mailer: d.Mailer, WebURL: strings.TrimRight(webURL, "/"), Logger: logger,
	})
	if d.Jobs != nil {
		d.Jobs.Handle(usecase.EmailJobKind, m.svc.SendEmail, app.JobOptions{})
	}
	m.services = d.Services
	m.logger = logger
	return nil
}

func (m *Module) Routes(api huma.API) {
	httpadapter.NewHandlers(m.svc, m.services, m.logger).Register(api)
}

func (m *Module) Subscribe(app.EventBus) {}

// busEvents publishes auth events on the bus after a transaction committed.
// The bus never fails the publisher, so an error is only logged.
type busEvents struct {
	bus app.Publisher
	log *slog.Logger
}

func (b busEvents) DeviceRevoked(ctx context.Context, userID, deviceID string) {
	if b.bus == nil {
		return
	}
	evt := DeviceRevoked{UserID: userID, DeviceID: deviceID}
	if err := b.bus.Publish(ctx, evt); err != nil {
		b.log.ErrorContext(ctx, "auth: publish failed", "event", evt.EventName(), "error", err)
	}
}

type authenticator struct{ m *Module }

func (a authenticator) Authenticate(ctx context.Context, token string) (app.Identity, error) {
	return a.m.svc.Authenticate(ctx, token)
}

type directory struct{ m *Module }

func (d directory) Users(ctx context.Context, ids []string) ([]UserSummary, error) {
	users, err := d.m.svc.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]UserSummary, len(users))
	for i, u := range users {
		out[i] = UserSummary{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Timezone: u.Timezone}
	}
	return out, nil
}
