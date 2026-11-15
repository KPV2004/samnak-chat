package realtime

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/coder/websocket"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Message types the hub owns. Every other client type goes to a handler
// registered with Hub.Handle.
const (
	typeAuth  = "auth"
	typePing  = "ping"
	typePong  = "pong"
	typeReady = "ready"
	typeError = "error"
)

// Close codes. 4401 is the application code for "authenticate again".
const (
	codeUnauthorized websocket.StatusCode = 4401
	codeIdle                              = websocket.StatusPolicyViolation
)

// Defaults from the M2 plan.
const (
	defaultAuthTimeout = 10 * time.Second
	defaultIdleTimeout = 60 * time.Second
	defaultSendQueue   = 256
	maxInboundBytes    = 64 * 1024
	writeTimeout       = 10 * time.Second
	authCallTimeout    = 5 * time.Second
	// minPresenceGap bounds Redis writes from a client that floods pings.
	minPresenceGap = time.Second
)

var pongFrame = []byte(`{"type":"pong"}`)

type authData struct {
	AccessToken string `json:"access_token"`
}

type readyData struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
}

type errorData struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func marshalEnvelope(typ string, data any) []byte {
	env, err := app.NewEnvelope(typ, data)
	if err != nil {
		return nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return b
}

// errorCode maps a handler error to the client-visible code. Details of
// unexpected errors never reach the client.
func errorCode(err error) (code, msg string) {
	switch {
	case errors.Is(err, app.ErrInvalid):
		return "invalid", err.Error()
	case errors.Is(err, app.ErrForbidden):
		return "forbidden", err.Error()
	case errors.Is(err, app.ErrNotFound):
		return "not_found", err.Error()
	case errors.Is(err, app.ErrRateLimited):
		return "rate_limited", err.Error()
	default:
		return "internal", "internal error"
	}
}
