package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// Pin puts a channel at the top of the caller's list. Pins are for members:
// a channel the caller is not in is not found. A user may pin at most
// domain.MaxPinnedChannels channels in a workspace, and pinning a channel that
// is pinned already changes nothing.
func (s *Service) Pin(ctx context.Context, p app.Principal, channelID string) error {
	return s.setPin(ctx, p, channelID, true)
}

// Unpin takes a channel off the top of the caller's list. Unpinning a channel
// that is not pinned changes nothing.
func (s *Service) Unpin(ctx context.Context, p app.Principal, channelID string) error {
	return s.setPin(ctx, p, channelID, false)
}

// setPin pins or unpins a channel for the caller and tells their own devices
// when that changed anything.
func (s *Service) setPin(ctx context.Context, p app.Principal, channelID string, pin bool) error {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return err
	}
	if !v.IsMember {
		return fmt.Errorf("channel: %w", app.ErrNotFound)
	}
	changed := false
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		// LockPins holds the caller's memberships, so the count below is exact
		// even when two pins arrive at once.
		pinned, err := s.repo.LockPins(ctx, p.WorkspaceID, p.UserID)
		if err != nil {
			return err
		}
		if pinnedAlready := slices.Contains(pinned, v.ID); pinnedAlready == pin {
			return nil // already in the state asked for
		}
		var at *time.Time
		if pin {
			if len(pinned) >= domain.MaxPinnedChannels {
				return fmt.Errorf("you can pin at most %d channels: %w", domain.MaxPinnedChannels, app.ErrConflict)
			}
			now := s.clock()
			at = &now
		}
		found, err := s.repo.SetPin(ctx, p.WorkspaceID, v.ID, p.UserID, at)
		if err != nil {
			return err
		}
		if !found {
			// The caller left the channel after it was loaded.
			return fmt.Errorf("channel: %w", app.ErrNotFound)
		}
		changed = true
		return nil
	})
	if err != nil || !changed {
		return err
	}
	v, err = s.channel(ctx, p, channelID)
	if err != nil {
		return err
	}
	s.ev.ChannelPinned(ctx, v, p.UserID)
	return nil
}
