package app

import (
	"context"
	"encoding/json"
)

// Envelope is one WebSocket message in either direction (SRS 14.3):
// {"type": "message.created", "data": {...}}.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// NewEnvelope marshals data into an Envelope.
func NewEnvelope(typ string, data any) (Envelope, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: typ, Data: b}, nil
}

// ClientHandler handles one client → server message type, such as "typing".
// It runs on the connection's reader goroutine, so it must return quickly.
type ClientHandler func(ctx context.Context, from Identity, data json.RawMessage) error

// Realtime delivers payloads to connected users on every server instance.
// It is implemented by internal/platform/realtime and knows no features:
// modules decide who receives what. Delivery is best effort; the database is
// the source of truth and clients catch up over REST after reconnecting.
type Realtime interface {
	// SendToUsers delivers env to every open connection of each user.
	SendToUsers(ctx context.Context, userIDs []string, env Envelope) error
	// Online returns the subset of userIDs with at least one connection on
	// any instance.
	Online(ctx context.Context, userIDs []string) ([]string, error)
	// DisconnectDevice closes every connection opened with that device's
	// tokens, on every instance (AUTH-08).
	DisconnectDevice(ctx context.Context, deviceID string) error
	// Handle registers h for a client message type. Call it from Init.
	// "auth" and "ping" are reserved by the hub.
	Handle(msgType string, h ClientHandler)
}
