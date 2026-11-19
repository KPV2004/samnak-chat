package httpx_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// The extra middleware wraps the built-in ones, so it sees every route,
// including /healthz, and the 500 that the recoverer writes for a panic.
func TestNewRouterWrapsEveryRouteWithExtraMiddleware(t *testing.T) {
	var seen []string
	mark := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
	r := httpx.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), mark)
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	for _, path := range []string{"/healthz", "/x"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if len(seen) != 2 || seen[0] != "/healthz" || seen[1] != "/x" {
		t.Fatalf("middleware saw %v", seen)
	}
}

func TestExtraMiddlewareSeesStatusOfRecoveredPanic(t *testing.T) {
	var status int
	record := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() { status = ww.Status() }()
			next.ServeHTTP(ww, r)
		})
	}
	r := httpx.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), record)
	r.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status seen by middleware = %d, want 500", status)
	}
}
