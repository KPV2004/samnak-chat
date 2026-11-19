package main

import (
	"net/http"
	"strings"
	"testing"
)

// seriesValue returns the value of the exposition line that starts with
// series, or -1 when the scrape has no such line.
func seriesValue(body, series string) int {
	for _, line := range strings.Split(body, "\n") {
		if rest, ok := strings.CutPrefix(line, series+" "); ok {
			var n int
			for _, c := range rest {
				if c < '0' || c > '9' {
					break
				}
				n = n*10 + int(c-'0')
			}
			return n
		}
	}
	return -1
}

func TestMetricsEndpointCountsRequestsByRoutePattern(t *testing.T) {
	h, err := build(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	const series = `http_request_duration_seconds_count{method="GET",route="/api/v1/meta",status="200"}`

	// A series that is not in the scrape yet has counted nothing.
	before := max(seriesValue(getPath(h, "/metrics").Body.String(), series), 0)
	if rec := getPath(h, "/api/v1/meta"); rec.Code != http.StatusOK {
		t.Fatalf("meta status = %d", rec.Code)
	}

	rec := getPath(h, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d", rec.Code)
	}
	body := rec.Body.String()
	if got := seriesValue(body, series); got != before+1 {
		t.Fatalf("%s = %d, want %d", series, got, before+1)
	}
	for _, name := range []string{"ws_connections", "ws_messages_sent_total", "go_goroutines"} {
		if seriesValue(body, name) < 0 {
			t.Fatalf("/metrics has no %s series", name)
		}
	}
}
