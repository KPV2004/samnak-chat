package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

type errBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestWriteError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", app.ErrInvalid, 400, "invalid"},
		{"unauthorized", app.ErrUnauthorized, 401, "unauthorized"},
		{"forbidden", app.ErrForbidden, 403, "forbidden"},
		{"rate limited", app.ErrRateLimited, 429, "rate_limited"},
		{"not found", app.ErrNotFound, 404, "not_found"},
		{"conflict", app.ErrConflict, 409, "conflict"},
		{"wrapped", fmt.Errorf("channel: %w", app.ErrNotFound), 404, "not_found"},
		{"unknown", errors.New("pq: password for db.internal failed"), 500, "internal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httpx.WriteError(rec, httptest.NewRequest("GET", "/", nil), c.err)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q", ct)
			}
			var b errBody
			if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if b.Error.Code != c.code {
				t.Fatalf("code = %q, want %q", b.Error.Code, c.code)
			}
			if c.code == "internal" && strings.Contains(rec.Body.String(), "password") {
				t.Fatalf("leaked detail: %s", rec.Body.String())
			}
			if c.name == "wrapped" && b.Error.Message != "channel: not found" {
				t.Fatalf("message = %q", b.Error.Message)
			}
		})
	}
}

// TestClientAbortedIsNotAServerError: a client that hangs up cancels its own
// request, and the cancellation surfaces as an error. That is not a 500.
func TestClientAbortedIsNotAServerError(t *testing.T) {
	gone, hangUp := context.WithCancel(context.Background())
	hangUp()
	cancelled := fmt.Errorf("eventbus: write %q to the outbox: %w", "task.status_changed", context.Canceled)

	// Only a cancellation that ended the request counts. The same error on a
	// live request is a fault, and another error after the client left is too.
	cases := []struct {
		name    string
		ctx     context.Context
		err     error
		aborted bool
	}{
		{"the client hung up", gone, cancelled, true},
		{"cancelled while the request is live", context.Background(), cancelled, false},
		{"another error after the client left", gone, errors.New("disk full"), false},
		{"a deadline after the client left", gone, context.DeadlineExceeded, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := httpx.ClientAborted(c.ctx, c.err); got != c.aborted {
				t.Fatalf("ClientAborted = %v, want %v", got, c.aborted)
			}
		})
	}

	t.Run("WriteError answers 499 and hides the detail", func(t *testing.T) {
		rec := httptest.NewRecorder()
		httpx.WriteError(rec, httptest.NewRequest("GET", "/", nil).WithContext(gone), cancelled)
		if rec.Code != httpx.StatusClientClosedRequest {
			t.Fatalf("status = %d, want 499", rec.Code)
		}
		var b errBody
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		if b.Error.Code != "client_closed_request" || strings.Contains(rec.Body.String(), "outbox") {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusCreated, map[string]int{"a": 1})
	if rec.Code != 201 || strings.TrimSpace(rec.Body.String()) != `{"a":1}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestRequestIDAndAccessLog(t *testing.T) {
	var buf bytes.Buffer
	r := httpx.NewRouter(slog.New(slog.NewJSONHandler(&buf, nil)))
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	gen := rec.Header().Get("X-Request-ID")
	if gen == "" {
		t.Fatal("no generated request id")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Fatalf("request id = %q", got)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != "abc-123" || entry["status"] != float64(204) || entry["path"] != "/x" || entry["method"] != "GET" {
		t.Fatalf("log entry = %v", entry)
	}
}

func TestPanicRecovery(t *testing.T) {
	var buf bytes.Buffer
	r := httpx.NewRouter(slog.New(slog.NewJSONHandler(&buf, nil)))
	r.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("secret detail") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != 500 {
		t.Fatalf("status = %d", rec.Code)
	}
	var b errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Error.Code != "internal" || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHealthzAndReadyz(t *testing.T) {
	r := httpx.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	var failing error
	httpx.MountReadiness(r,
		httpx.Check{Name: "db", Fn: func(context.Context) error { return nil }},
		httpx.Check{Name: "redis", Fn: func(context.Context) error { return failing }},
	)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	if rec := get("/healthz"); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
	if rec := get("/readyz"); rec.Code != 200 {
		t.Fatalf("readyz ok = %d", rec.Code)
	}
	failing = errors.New("down")
	rec := get("/readyz")
	if rec.Code != 503 {
		t.Fatalf("readyz = %d", rec.Code)
	}
	var b struct {
		Failed []string `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Failed) != 1 || b.Failed[0] != "redis" {
		t.Fatalf("failed = %v", b.Failed)
	}
	if get("/healthz").Code != 200 {
		t.Fatal("healthz must stay 200")
	}
}
