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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/config"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
	redisclient "github.com/KPV2004/samnak-chat/apps/server/internal/platform/redis"
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

	handler, err := newHandler(logger, cfg.Modules, available, infra{
		Tx:       database.NewTxManager(pool),
		Platform: map[string]any{"db": pool, "jwt_secret": cfg.JWTSecret},
		Checks:   []httpx.Check{{Name: "db", Fn: pool.Ping}},
	}.withRedis(rdb))
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	logger.Info("server listening", "addr", ln.Addr().String(), "version", version)
	return serve(ctx, ln, handler, logger, shutdownTimeout)
}

// infra is what newHandler needs from the process: handles that tests replace.
type infra struct {
	Tx       app.TxManager
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

	r := httpx.NewRouter(logger)
	httpx.MountReadiness(r, in.Checks...)

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
	logger.Info("modules started", "modules", started)
	return r, nil
}

func newDeps(logger *slog.Logger, in infra) *app.Deps {
	return &app.Deps{
		Logger:        logger,
		Services:      app.NewServices(),
		Bus:           eventbus.New(logger),
		Notifications: app.NoopNotificationRegistry{},
		// Cards stays nil until the chat module provides the registry (M2).
		// Modules that register cards must not be enabled before then.
		Cards:    nil,
		Tx:       in.Tx,
		Platform: in.Platform,
	}
}

// skipUnbuiltCoreModules drops core modules this binary does not contain yet.
// config.Modules always lists the core modules, but auth, workspace, chat and
// notify arrive in M1 and M2; until then they would make every start fail with
// "unknown module". Names that are not core modules are never dropped, so a
// typo in MODULES_ENABLED still stops the server. Remove this function when
// the core modules are registered.
func skipUnbuiltCoreModules(logger *slog.Logger, names []string, available []app.Module) []string {
	have := map[string]bool{}
	for _, m := range available {
		have[m.Name()] = true
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if slices.Contains(app.CoreModules, n) && !have[n] {
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
