// Command server is the composition root: the only place that knows every module.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/config"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/jobs"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/observability"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/realtime"
	redisclient "github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/storage"
)

const (
	serviceName     = "samnak-chat"
	shutdownTimeout = 15 * time.Second
)

// version is the value /meta reports. Release builds override it with
// -ldflags "-X main.version=...".
var version = "0.0.0-dev"

// modules is every module this binary can run. MODULES_ENABLED picks from it.
// Modules are added here as they land.
var modules = []app.Module{
	auth.New(),
	workspace.New(),
	chat.New(),
	notify.New(),
	task.New(),
	approval.New(),
	helpdesk.New(),
}

func main() {
	os.Exit(realMain())
}

// realMain returns the exit code, so deferred cleanup runs before os.Exit.
func realMain() int {
	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		if err := writeOpenAPI(os.Stdout, modules); err != nil {
			fmt.Fprintln(os.Stderr, "openapi:", err)
			return 1
		}
		return 0
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, modules); err != nil {
		logger.Error("server stopped", "error", err)
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// writeOpenAPI prints the OpenAPI document as YAML for every module in
// available. It needs no database: Module.Init must not do I/O.
func writeOpenAPI(w io.Writer, available []app.Module) error {
	names := make([]string, 0, len(available))
	for _, m := range available {
		names = append(names, m.Name())
	}
	reg := app.NewRegistry(available...)
	enabled, err := reg.Enable(names)
	if err != nil {
		return fmt.Errorf("enable modules: %w", err)
	}
	logger := slog.New(slog.DiscardHandler)
	api := httpx.NewAPI(chi.NewRouter(), version)
	httpx.RegisterMeta(api, serviceName, version, nil)
	// Modules need config to Init but never use it here; the placeholder
	// secret signs nothing.
	deps := newDeps(logger, infra{Platform: map[string]any{"jwt_secret": "openapi-only"}})
	if err := reg.Start(enabled, deps, api); err != nil {
		return fmt.Errorf("start modules: %w", err)
	}
	spec, err := api.OpenAPI().YAML()
	if err != nil {
		return err
	}
	_, err = w.Write(spec)
	return err
}

// run starts the server and blocks until ctx is cancelled and in-flight
// requests have finished, or until startup or serving fails.
func run(ctx context.Context, logger *slog.Logger, available []app.Module) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rdb, err := openRedis(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store, err := storage.New(ctx, storage.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		PathStyle: true, // MinIO; AWS also accepts path-style URLs
	})
	if err != nil {
		return fmt.Errorf("object storage, check S3_* settings: %w", err)
	}

	// The hub looks up app.Authenticator at handshake time, so it shares the
	// Services that modules provide into.
	services := app.NewServices()
	hub := realtime.NewHub(logger, services, rdb)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Close(closeCtx)
	}()

	queue := jobs.New(pool, logger)
	bus := eventbus.NewOutbox(pool, logger)
	bus.RegisterJobs(queue)
	handler, err := newHandler(logger, cfg.Modules, available, infra{
		Services: services,
		Tx:       database.NewTxManager(pool),
		Jobs:     queue,
		Bus:      bus,
		Realtime: hub,
		Storage:  store,
		Mount:    hub.Mount,
		Platform: map[string]any{"db": pool, "jwt_secret": cfg.JWTSecret},
		Checks:   []httpx.Check{{Name: "db", Fn: pool.Ping}},
	}.withRedis(rdb))
	if err != nil {
		return err
	}

	// Modules registered their handlers in Init, so the queue starts after
	// reg.Start. It stops after serve returns, once HTTP has drained (NFR-11).
	if err := queue.Start(ctx); err != nil {
		return fmt.Errorf("start jobs: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := queue.Stop(stopCtx); err != nil {
			logger.Error("jobs: stop", "error", err)
		}
	}()

	// The relay delivers outbox events to handlers that may enqueue jobs, so it
	// starts after the queue and, being deferred later, stops before it.
	if err := bus.Start(ctx); err != nil {
		return fmt.Errorf("start event relay: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := bus.Stop(stopCtx); err != nil {
			logger.Error("eventbus: stop relay", "error", err)
		}
	}()

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	logger.Info("server listening", "addr", ln.Addr().String(), "version", version)
	return serve(ctx, ln, handler, logger, shutdownTimeout)
}

// infra is what newHandler needs from the process: handles that tests replace.
type infra struct {
	// Services is shared with the realtime hub; nil means a fresh registry.
	Services *app.Services
	Realtime app.Realtime
	Storage  app.ObjectStorage
	// Mount adds non-Huma routes to the root router, such as GET /api/v1/ws.
	Mount func(chi.Router)
	Tx    app.TxManager
	// Jobs is the background queue; nil in tests and the openapi command.
	Jobs app.Jobs
	// Bus is the event bus with the outbox; nil means an in-memory bus, as in
	// tests and the openapi command.
	Bus      app.EventBus
	Platform map[string]any
	Checks   []httpx.Check
}

// withRedis returns in with the Redis client in Platform["redis"] and a
// "redis" readiness check. It leaves in's own map and slice untouched.
func (in infra) withRedis(c *goredis.Client) infra {
	platform := make(map[string]any, len(in.Platform)+1)
	maps.Copy(platform, in.Platform)
	platform["redis"] = c
	in.Platform = platform
	in.Checks = append(slices.Clone(in.Checks), httpx.Check{
		Name: "redis",
		Fn:   func(ctx context.Context) error { return c.Ping(ctx).Err() },
	})
	return in
}

// openRedis connects to the server named by url and fails with a message that
// points at REDIS_URL, so a bad setting is easy to find at startup.
func openRedis(ctx context.Context, url string) (*goredis.Client, error) {
	if url == "" {
		return nil, errors.New("REDIS_URL is required")
	}
	c, err := redisclient.Open(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Redis, check REDIS_URL: %w", err)
	}
	return c, nil
}

// newHandler enables and starts the named modules from available and returns
// the HTTP handler. Any dependency problem is returned as an error naming the
// module, so the caller can refuse to start.
func newHandler(logger *slog.Logger, names []string, available []app.Module, in infra) (http.Handler, error) {
	names = skipUnbuiltCoreModules(logger, names, available)
	reg := app.NewRegistry(available...)
	enabled, err := reg.Enable(names)
	if err != nil {
		return nil, fmt.Errorf("enable modules: %w", err)
	}

	r := httpx.NewRouter(logger, observability.HTTPMetrics)
	httpx.MountReadiness(r, in.Checks...)
	r.Get("/metrics", observability.Handler().ServeHTTP)
	if in.Mount != nil {
		in.Mount(r)
	}

	deps := newDeps(logger, in)
	started := make([]string, 0, len(enabled))
	for _, m := range enabled {
		started = append(started, m.Name())
	}

	var startErr error
	r.Route("/api/v1", func(v1 chi.Router) {
		api := httpx.NewAPI(v1, version)
		httpx.RegisterMeta(api, serviceName, version, started)
		startErr = reg.Start(enabled, deps, api)
	})
	if startErr != nil {
		return nil, fmt.Errorf("start modules: %w", startErr)
	}
	subscribeDeviceRevoked(logger, deps.Bus, deps.Realtime)
	logger.Info("modules started", "modules", started)
	return r, nil
}

// userDisconnector is the extra method a revoke of every device needs.
// app.Realtime only knows devices, so the hub optionally implements this.
type userDisconnector interface {
	DisconnectUser(ctx context.Context, userID string) error
}

// subscribeDeviceRevoked closes the sockets of a device whose sessions were
// revoked (AUTH-08), or of every device of the user when the event carries no
// device ID. The bus never fails the publisher, so a failure only logs.
func subscribeDeviceRevoked(logger *slog.Logger, bus app.EventBus, rt app.Realtime) {
	if bus == nil || rt == nil {
		return
	}
	bus.Subscribe(auth.DeviceRevoked{}.EventName(), func(ctx context.Context, evt app.Event) error {
		e, ok := evt.(auth.DeviceRevoked)
		if !ok {
			return nil
		}
		if e.DeviceID != "" {
			if err := rt.DisconnectDevice(ctx, e.DeviceID); err != nil {
				logger.ErrorContext(ctx, "realtime: disconnect revoked device", "device_id", e.DeviceID, "error", err)
			}
			return nil
		}
		u, ok := rt.(userDisconnector)
		if !ok {
			logger.WarnContext(ctx, "realtime: hub cannot close every device of a user", "user_id", e.UserID)
			return nil
		}
		if err := u.DisconnectUser(ctx, e.UserID); err != nil {
			logger.ErrorContext(ctx, "realtime: disconnect revoked user", "user_id", e.UserID, "error", err)
		}
		return nil
	})
}

func newDeps(logger *slog.Logger, in infra) *app.Deps {
	services := in.Services
	if services == nil {
		services = app.NewServices()
	}
	bus := in.Bus
	if bus == nil {
		bus = eventbus.New(logger)
	}
	return &app.Deps{
		Logger:        logger,
		Services:      services,
		Realtime:      in.Realtime,
		Storage:       in.Storage,
		Bus:           bus,
		Notifications: app.NewNotificationRules(),
		Tx:            in.Tx,
		Jobs:          in.Jobs,
		Platform:      in.Platform,
	}
}

// skipUnbuiltCoreModules drops core and default modules this binary does not
// contain yet. config.Modules lists them even before their code exists (notify
// arrives in M3); without this, every start would fail with "unknown module".
// Other names are never dropped, so a typo in MODULES_ENABLED still stops the
// server. Remove this function when every core and default module is built.
func skipUnbuiltCoreModules(logger *slog.Logger, names []string, available []app.Module) []string {
	have := map[string]bool{}
	for _, m := range available {
		have[m.Name()] = true
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if (slices.Contains(app.CoreModules, n) || slices.Contains(app.DefaultModules, n)) && !have[n] {
			logger.Warn("core module not built yet, skipping", "module", n)
			continue
		}
		out = append(out, n)
	}
	return out
}

// serve runs handler on ln until ctx is cancelled, then stops accepting
// connections and waits up to timeout for in-flight requests to finish.
func serve(ctx context.Context, ln net.Listener, handler http.Handler, logger *slog.Logger, timeout time.Duration) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.Serve(ln) }()

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down", "timeout", timeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-failed; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}
