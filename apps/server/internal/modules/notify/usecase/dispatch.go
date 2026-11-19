package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// pending is a notice that passed the user's preferences, with whether it
// arrives inside their do-not-disturb window.
type pending struct {
	n     domain.Notification
	quiet bool
}

// Dispatch runs every rule registered for the event's name, stores what they
// produce and delivers the rows that were actually inserted (NOTI-01..03).
//
// A rule that errors or panics is logged and skipped: it never stops the other
// rules and never reaches the publisher. Notifications without a user,
// workspace or kind are dropped (NOTI-02). The recipient's preferences drop a
// notice of a disabled kind or in a muted channel before it is stored; inside
// the do-not-disturb window it is stored and counted but delivered as quiet
// (NOTI-07). Rows from one event are inserted in one transaction, so either
// the whole event is stored before anything is delivered or nothing is. A
// repeated key inserts nothing and sends nothing.
func (s *Service) Dispatch(ctx context.Context, evt app.Event) error {
	if s.rules == nil || evt == nil {
		return nil
	}
	rules := s.rules.Rules()[evt.EventName()]
	if len(rules) == 0 {
		return nil
	}

	// The bus runs handlers on the publisher's request context. The message
	// is already committed, so a client that hangs up must not cancel storing
	// its notifications (NOTI-02).
	ctx = context.WithoutCancel(ctx)
	at := s.clock()
	var fresh []app.Notification
	for i, rule := range rules {
		for _, n := range s.runRule(ctx, evt, i, rule) {
			if !domain.Valid(n) {
				s.log.WarnContext(ctx, "notify: dropping an invalid notification", "event", evt.EventName(), "rule", i)
				continue
			}
			fresh = append(fresh, n)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	kept, err := s.applyPreferences(ctx, fresh, at)
	if err != nil {
		return err
	}
	if len(kept) == 0 {
		return nil
	}

	var stored []pending
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		for _, p := range kept {
			row, inserted, err := s.repo.Insert(ctx, p.n)
			if err != nil {
				return err
			}
			if inserted {
				stored = append(stored, pending{n: row, quiet: p.quiet})
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("notify: store notifications: %w", err)
	}
	s.deliver(ctx, stored)
	return nil
}

// runRule calls one rule and recovers its panic. A failed rule yields nothing.
func (s *Service) runRule(ctx context.Context, evt app.Event, i int, rule app.NotificationRule) (out []app.Notification) {
	defer func() {
		if p := recover(); p != nil {
			s.log.ErrorContext(ctx, "notify: rule panicked", "event", evt.EventName(), "rule", i, "panic", p)
			out = nil
		}
	}()
	out, err := rule(ctx, evt)
	if err != nil {
		s.log.ErrorContext(ctx, "notify: rule failed", "event", evt.EventName(), "rule", i, "error", err)
		return nil
	}
	return out
}

// deliver sends one frame per inserted row, after the transaction committed.
// The unread count is read once per user after the batch, so every frame of a
// burst carries the final number and the client's last frame is right.
func (s *Service) deliver(ctx context.Context, stored []pending) {
	if s.ev == nil || len(stored) == 0 {
		return
	}
	counts := map[[2]string]int64{}
	for _, p := range stored {
		n := p.n
		key := [2]string{n.WorkspaceID, n.UserID}
		count, ok := counts[key]
		if !ok {
			c, err := s.repo.UnreadCount(ctx, n.WorkspaceID, n.UserID)
			if err != nil {
				// Delivery is best effort and the list is the truth; the
				// client refetches the count when it reconnects.
				s.log.WarnContext(ctx, "notify: unread count for delivery failed", "user_id", n.UserID, "error", err)
			}
			count = c
			counts[key] = c
		}
		s.ev.Created(ctx, n, count, p.quiet)
	}
}
