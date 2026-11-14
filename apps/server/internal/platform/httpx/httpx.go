package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const requestIDHeader = "X-Request-ID"

type ctxKey struct{}

// RequestID returns the request ID stored by the router middleware.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Check is a named readiness check.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// NewRouter returns a router with request ID, access log and panic recovery
// middleware, and GET /healthz.
func NewRouter(logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	r.Use(requestID, accessLog(logger), recoverer(logger))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return r
}

// MountReadiness registers GET /readyz. It returns 503 listing failing check names.
func MountReadiness(r chi.Router, checks ...Check) {
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		failed := []string{}
		for _, c := range checks {
			if err := c.Fn(req.Context()); err != nil {
				failed = append(failed, c.Name)
			}
		}
		if len(failed) > 0 {
			sort.Strings(failed)
			WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable", "failed": failed})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError maps err to the error JSON. Unknown errors become 500 "internal"
// without detail.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	msg := "internal error"
	switch {
	case errors.Is(err, app.ErrInvalid):
		status, code = http.StatusBadRequest, "invalid"
	case errors.Is(err, app.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, app.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, app.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	}
	if status == http.StatusInternalServerError {
		slog.Default().ErrorContext(r.Context(), "request failed", "request_id", RequestID(r.Context()), "error", err)
	} else {
		msg = err.Error()
	}
	WriteJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				logger.InfoContext(r.Context(), "http request",
					"request_id", RequestID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					logger.ErrorContext(r.Context(), "panic recovered",
						"request_id", RequestID(r.Context()), "panic", rec, "stack", string(debug.Stack()))
					WriteJSON(w, http.StatusInternalServerError, map[string]any{
						"error": map[string]string{"code": "internal", "message": "internal error"},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
