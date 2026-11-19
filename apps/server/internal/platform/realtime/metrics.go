package realtime

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// The metrics are process-wide, registered on the default registry that
// /metrics serves, so every hub in the process adds to the same series.
var (
	wsConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ws_connections",
		Help: "Open WebSocket connections on this instance, authenticated or not.",
	})
	wsMessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ws_messages_sent_total",
		Help: "Messages written to WebSocket clients on this instance.",
	})
)
