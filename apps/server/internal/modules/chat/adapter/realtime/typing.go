package realtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Typist handles a typing notification of one user.
type Typist interface {
	Typing(ctx context.Context, userID, channelID string) error
}

// TypingHandler returns the handler for the client frame
// {"type":"typing","data":{"channel_id":"..."}}. The hub reports the errors it
// returns to the client as an error frame and keeps the connection open.
func TypingHandler(t Typist) app.ClientHandler {
	return func(ctx context.Context, from app.Identity, data json.RawMessage) error {
		var in struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(data, &in); err != nil {
			return fmt.Errorf("typing needs {\"channel_id\": ...}: %w", app.ErrInvalid)
		}
		return t.Typing(ctx, from.UserID, in.ChannelID)
	}
}
