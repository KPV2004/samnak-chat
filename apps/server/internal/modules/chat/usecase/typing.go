package usecase

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	// typingInterval is the shortest gap between two typing events of one
	// user in one channel (CHAT-17).
	typingInterval = 3 * time.Second
	// maxTypingEntries is when the limiter starts dropping expired entries.
	maxTypingEntries = 4096
)

// typingLimiter lets one typing event per user and channel through per
// interval. It lives in one process: with several instances a user typing
// through two connections on two instances can exceed the limit, which only
// costs a few extra small frames.
type typingLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	now      func() time.Time
	last     map[[2]string]time.Time
}

func newTypingLimiter(interval time.Duration, now func() time.Time) *typingLimiter {
	return &typingLimiter{interval: interval, now: now, last: map[[2]string]time.Time{}}
}

// allow reports whether an event may go out, and counts it if so.
func (l *typingLimiter) allow(userID, channelID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	key := [2]string{userID, channelID}
	if at, ok := l.last[key]; ok && now.Sub(at) < l.interval {
		return false
	}
	if len(l.last) >= maxTypingEntries {
		for k, at := range l.last {
			if now.Sub(at) >= l.interval {
				delete(l.last, k)
			}
		}
	}
	l.last[key] = now
	return true
}

func (l *typingLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.last)
}

// Typing tells the other members of a channel that userID is typing (CHAT-17).
// The frame names no workspace, so the channel membership decides which one
// it is. Events over the rate limit are dropped without an error.
func (s *Service) Typing(ctx context.Context, userID, channelID string) error {
	if _, err := uuid.Parse(channelID); err != nil {
		return fmt.Errorf("channel_id is not a valid ID: %w", app.ErrInvalid)
	}
	if !s.typing.allow(userID, channelID) {
		return nil
	}
	workspaceID, err := s.repo.MemberWorkspace(ctx, channelID, userID)
	if err != nil {
		return fmt.Errorf("channel: %w", app.ErrNotFound)
	}
	active, err := s.ws.IsActiveMember(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("you are not a member of this workspace: %w", app.ErrForbidden)
	}
	members, err := s.repo.MemberIDs(ctx, workspaceID, channelID)
	if err != nil {
		return err
	}
	recipients := slices.DeleteFunc(members, func(id string) bool { return id == userID })
	s.ev.Typing(ctx, workspaceID, channelID, userID, recipients)
	return nil
}
