package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// AddReaction puts the caller's emoji on a message of a channel they are in
// (CHAT-18). Adding the same emoji twice is fine and announces nothing.
func (s *Service) AddReaction(ctx context.Context, p app.Principal, messageID, emoji string) error {
	emoji, err := domain.NormalizeEmoji(emoji)
	if err != nil {
		return err
	}
	m, ch, err := s.reactable(ctx, p, messageID)
	if err != nil {
		return err
	}
	if !hasReaction(m, emoji) && len(m.Reactions) >= domain.MaxReactionKinds {
		return fmt.Errorf("a message holds at most %d different emoji: %w", domain.MaxReactionKinds, app.ErrInvalid)
	}
	added, err := s.repo.AddReaction(ctx, p.WorkspaceID, m.ID, p.UserID, emoji, s.clock())
	if err != nil || !added {
		return err
	}
	s.reactionsChanged(ctx, p.WorkspaceID, ch.ID, m.ID)
	return nil
}

// RemoveReaction takes the caller's emoji off a message. Removing one that is
// not there is fine and announces nothing.
func (s *Service) RemoveReaction(ctx context.Context, p app.Principal, messageID, emoji string) error {
	emoji, err := domain.NormalizeEmoji(emoji)
	if err != nil {
		return err
	}
	m, ch, err := s.reactable(ctx, p, messageID)
	if err != nil {
		return err
	}
	removed, err := s.repo.RemoveReaction(ctx, p.WorkspaceID, m.ID, p.UserID, emoji)
	if err != nil || !removed {
		return err
	}
	s.reactionsChanged(ctx, p.WorkspaceID, ch.ID, m.ID)
	return nil
}

// reactable loads a live message of a channel the caller is a member of.
func (s *Service) reactable(ctx context.Context, p app.Principal, messageID string) (MessageView, ChannelView, error) {
	m, ch, err := s.message(ctx, p, messageID)
	if err != nil {
		return MessageView{}, ChannelView{}, err
	}
	if m.Deleted() {
		return MessageView{}, ChannelView{}, fmt.Errorf("message: %w", app.ErrNotFound)
	}
	if !ch.IsMember {
		return MessageView{}, ChannelView{}, fmt.Errorf("join the channel first: %w", app.ErrForbidden)
	}
	if ch.Archived() {
		return MessageView{}, ChannelView{}, fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	return m, ch, nil
}

func hasReaction(m MessageView, emoji string) bool {
	for _, g := range m.Reactions {
		if g.Emoji == emoji {
			return true
		}
	}
	return false
}

// reactionsChanged sends the message with its new reactions to the channel.
func (s *Service) reactionsChanged(ctx context.Context, workspaceID, channelID, messageID string) {
	updated, err := s.repo.Message(ctx, workspaceID, messageID)
	if err != nil {
		s.log.WarnContext(ctx, "chat: reload message after reaction", "message_id", messageID, "error", err)
		return
	}
	s.deliver(ctx, workspaceID, channelID, func(r []string) { s.ev.MessageUpdated(ctx, updated, r) })
}
