package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// ListQuery selects a page of the caller's notifications, newest first.
type ListQuery struct {
	UnreadOnly bool
	BeforeID   string // cursor: return notifications older than this one
	Limit      int
}

// Page is one page of notifications.
type Page struct {
	Items   []domain.Notification
	HasMore bool
}

// List returns the caller's notifications in the caller's workspace (NOTI-04).
// Another workspace's rows never appear, because both IDs come from the
// Principal.
func (s *Service) List(ctx context.Context, p app.Principal, q ListQuery) (Page, error) {
	if q.BeforeID != "" {
		if _, err := uuid.Parse(q.BeforeID); err != nil {
			return Page{}, fmt.Errorf("before is not a valid ID: %w", app.ErrInvalid)
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	items, err := s.repo.List(ctx, p.WorkspaceID, p.UserID, q.UnreadOnly, q.BeforeID, limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{HasMore: len(items) > limit}
	if page.HasMore {
		items = items[:limit]
	}
	page.Items = items
	return page, nil
}

// UnreadCount is the number on the bell (NOTI-04).
func (s *Service) UnreadCount(ctx context.Context, p app.Principal) (int64, error) {
	return s.repo.UnreadCount(ctx, p.WorkspaceID, p.UserID)
}

// MarkRead marks one of the caller's notifications read. It is idempotent;
// another user's notification or an unknown one is app.ErrNotFound. Other
// devices of the caller hear about the change only when something changed.
func (s *Service) MarkRead(ctx context.Context, p app.Principal, id string) error {
	changed, err := s.repo.MarkRead(ctx, p.WorkspaceID, p.UserID, id, s.clock())
	if err != nil {
		return err
	}
	if !changed || s.ev == nil {
		return nil
	}
	count, err := s.repo.UnreadCount(ctx, p.WorkspaceID, p.UserID)
	if err != nil {
		return err
	}
	s.ev.Read(ctx, p.WorkspaceID, p.UserID, []string{id}, false, count)
	return nil
}

// ReadAll marks every unread notification of the caller read in this
// workspace. Nothing unread means nothing to tell other devices.
func (s *Service) ReadAll(ctx context.Context, p app.Principal) error {
	ids, err := s.repo.MarkAllRead(ctx, p.WorkspaceID, p.UserID, s.clock())
	if err != nil {
		return err
	}
	if len(ids) == 0 || s.ev == nil {
		return nil
	}
	s.ev.Read(ctx, p.WorkspaceID, p.UserID, ids, true, 0)
	return nil
}
