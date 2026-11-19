package realtime_test

import (
	"testing"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

// metricValue reads a process-wide gauge or counter from the default registry.
func metricValue(t *testing.T, name string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		m := f.GetMetric()[0]
		if g := m.GetGauge(); g != nil {
			return g.GetValue()
		}
		return m.GetCounter().GetValue()
	}
	t.Fatalf("metric %s is not registered", name)
	return 0
}

// The metrics are shared by every test in the process, so each check is a
// change from the value seen before the test's own traffic.
func TestMetricsCountOpenConnectionsAndSentFrames(t *testing.T) {
	_, srv := newHub(t, testAuth())
	connsBefore := metricValue(t, "ws_connections")
	sentBefore := metricValue(t, "ws_messages_sent_total")

	a := login(t, srv, "tok-alice") // "ready" is the first frame sent
	eventually(t, "one more open connection", func() bool {
		return metricValue(t, "ws_connections") == connsBefore+1
	})

	ping(t, a) // "pong" is the second
	eventually(t, "ready and pong counted", func() bool {
		return metricValue(t, "ws_messages_sent_total") == sentBefore+2
	})

	if err := a.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, "connection released", func() bool {
		return metricValue(t, "ws_connections") == connsBefore
	})
}
