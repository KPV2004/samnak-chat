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
