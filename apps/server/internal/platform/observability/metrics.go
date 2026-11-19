package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// unmatchedRoute labels a request that no route matched, so a scanner
	// walking random paths cannot grow the label set.
	unmatchedRoute = "unmatched"
	// otherMethod labels a method outside the standard set, for the same reason.
	otherMethod = "OTHER"
)

var standardMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodConnect: true, http.MethodOptions: true, http.MethodTrace: true,
}

var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "Duration of HTTP requests by chi route pattern, method and status.",
	Buckets: prometheus.DefBuckets,
}, []string{"route", "method", "status"})

// HTTPMetrics times every request into http_request_duration_seconds. The
// route label is the chi route pattern, never the raw path, so IDs stay out of
// the labels. Install it outermost, so the status it records is the one the
// client receives, including a 500 written by the recoverer.
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			httpDuration.WithLabelValues(routePattern(r), methodLabel(r), strconv.Itoa(status)).
				Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(ww, r)
	})
}

// isUpgrade reports a WebSocket handshake. A socket lasts as long as the client
// keeps it, so timing it would bury the request durations.
func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// routePattern reads the pattern after the handler has routed the request. The
// route context is shared with the router, so it is filled in by now.
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil || rctx.RoutePattern() == "" {
		return unmatchedRoute
	}
	return rctx.RoutePattern()
}

func methodLabel(r *http.Request) string {
	if standardMethods[r.Method] {
		return r.Method
	}
	return otherMethod
}

// Handler serves every registered metric in the Prometheus text format, with
// the Go runtime and process collectors that the default registry includes. It
// has no authentication, so the production compose file must not publish it.
func Handler() http.Handler {
	return promhttp.Handler()
}
