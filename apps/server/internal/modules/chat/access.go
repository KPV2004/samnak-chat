package chat

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// channelAccess is the app.ChannelAccess chat provides. It exists from
// Provide on and reaches the use cases once chat's own Init has built them.
type channelAccess struct{ m *Module }

var _ app.ChannelAccess = channelAccess{}

func (a channelAccess) CanRead(ctx context.Context, workspaceID, channelID, userID string) (bool, error) {
	if a.m.uc == nil {
		return false, fmt.Errorf("chat has not been initialised: %w", app.ErrInvalid)
	}
	return a.m.uc.CanRead(ctx, workspaceID, channelID, userID)
}

// messageReader is the app.MessageReader chat provides, with the same
// lifetime as channelAccess.
type messageReader struct{ m *Module }

var _ app.MessageReader = messageReader{}

func (r messageReader) ReadMessage(ctx context.Context, workspaceID, messageID, userID string) (app.MessageSnapshot, error) {
	if r.m.uc == nil {
		return app.MessageSnapshot{}, fmt.Errorf("chat has not been initialised: %w", app.ErrInvalid)
	}
	v, err := r.m.uc.ReadMessage(ctx, workspaceID, messageID, userID)
	if err != nil {
		return app.MessageSnapshot{}, err
	}
	out := app.MessageSnapshot{
		ID: v.ID, ChannelID: v.ChannelID, SenderID: v.SenderID, Kind: v.Kind, Body: v.Body, CreatedAt: v.CreatedAt,
	}
	for _, a := range v.Attachments {
		out.Attachments = append(out.Attachments, app.MessageAttachment{
			FileName: a.FileName, MimeType: a.MimeType, Size: a.Size, ObjectKey: a.ObjectKey,
		})
	}
	return out, nil
}
