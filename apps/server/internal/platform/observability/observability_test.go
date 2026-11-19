package observability_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/observability"
)

// scrape returns /metrics as a Prometheus scraper would read it.
func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	observability.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d", rec.Code)
	}
	return rec.Body.String()
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// Each test uses its own route pattern: the registry is process-wide, so a
// shared pattern would make the counts depend on test order.
func TestHTTPMetricsLabelsRequestsByRoutePattern(t *testing.T) {
	r := chi.NewRouter()
	r.Use(observability.HTTPMetrics)
	r.Get("/t1/channels/{id}", ok)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/t1/channels/0190f2c4-7a5e-7000-8000-000000000001", nil))

	body := scrape(t)
	want := `http_request_duration_seconds_count{method="GET",route="/t1/channels/{id}",status="200"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("missing series %s", want)
	}
	if strings.Contains(body, "0190f2c4") {
		t.Fatal("a raw ID leaked into a label")
	}
}

func TestHTTPMetricsUsesFixedLabelsForUnmatchedRequestsAndMethods(t *testing.T) {
	r := chi.NewRouter()
	r.Use(observability.HTTPMetrics)
	r.Get("/t2/known", ok)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/t2/wp-admin/x", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/t2/known", nil))

	body := scrape(t)
	for _, want := range []string{
		`http_request_duration_seconds_count{method="GET",route="unmatched",status="404"} 1`,
		// chi sets no route pattern for a method it does not serve, so it is unmatched.
		`http_request_duration_seconds_count{method="OTHER",route="unmatched",status="405"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing series %s", want)
		}
	}
	if strings.Contains(body, "wp-admin") || strings.Contains(body, "BREW") {
		t.Fatal("a raw path or method leaked into a label")
	}
}

// A socket stays open as long as the client keeps it; timing it would bury the
// request durations, so an upgrade records nothing.
func TestHTTPMetricsSkipsWebSocketUpgrades(t *testing.T) {
	r := chi.NewRouter()
	r.Use(observability.HTTPMetrics)
	r.Get("/t3/ws", ok)

	req := httptest.NewRequest(http.MethodGet, "/t3/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if strings.Contains(scrape(t), `route="/t3/ws"`) {
		t.Fatal("a WebSocket upgrade was timed")
	}
}

func TestHandlerServesGoAndProcessMetrics(t *testing.T) {
	body := scrape(t)
	for _, name := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if !strings.Contains(body, "\n"+name+" ") && !strings.HasPrefix(body, name+" ") {
			t.Fatalf("missing %s", name)
		}
	}
}
