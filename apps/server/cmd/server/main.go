// Command server is the composition root: the only place that knows every module.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/config"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
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
var modules []app.Module

func main() {
	os.Exit(realMain())
}

// realMain returns the exit code, so deferred cleanup runs before os.Exit.
func realMain() int {
	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		if err := writeOpenAPI(os.Stdout); err != nil {
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

// writeOpenAPI prints the OpenAPI document as YAML. It needs no database.
func writeOpenAPI(w io.Writer) error {
	r := chi.NewRouter()
	api := httpx.NewAPI(r, version)
	httpx.RegisterMeta(api, serviceName, version, nil)
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
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	handler, err := newHandler(logger, cfg.Modules, available, infra{
		Tx:       database.NewTxManager(pool),
		Platform: map[string]any{"db": pool},
		Checks:   []httpx.Check{{Name: "db", Fn: pool.Ping}},
	})
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

	deps := &app.Deps{
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
