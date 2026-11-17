package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// Get returns one request with its history and files. Only its requester and
// its approver see it; everyone else, in any workspace, gets not found
// (APR-06, APR-07).
func (s *Service) Get(ctx context.Context, p app.Principal, id string) (Detail, error) {
	req, err := s.Repo.Request(ctx, p.WorkspaceID, id)
	if err != nil {
		return Detail{}, err
	}
	if !req.CanSee(p.UserID) {
		return Detail{}, fmt.Errorf("request: %w", app.ErrNotFound)
	}
	return s.detail(ctx, req)
}

// ListQuery filters the caller's requests. Role is "requester" ("my
// requests"), "approver" ("waiting for me") or empty for both. Status is a
// comma-separated list, empty for all.
type ListQuery struct {
	Role   string
	Status string
	Cursor string
	Limit  int
}

// Page is one page of requests, newest first. NextCursor is empty on the last
// page.
type Page struct {
	Items      []domain.Request
	NextCursor string
}

// List returns the requests the caller sent or must decide (APR-06).
func (s *Service) List(ctx context.Context, p app.Principal, q ListQuery) (Page, error) {
	switch q.Role {
	case "", "requester", "approver":
	default:
		return Page{}, fmt.Errorf("role must be requester or approver: %w", app.ErrInvalid)
	}
	var statuses []domain.Status
	if q.Status != "" {
		for _, raw := range strings.Split(q.Status, ",") {
			st, err := domain.ParseStatus(strings.TrimSpace(raw))
			if err != nil {
				return Page{}, err
			}
			statuses = append(statuses, st)
		}
	}
	limit := q.Limit
	switch {
	case limit < 0:
		return Page{}, fmt.Errorf("limit must not be negative: %w", app.ErrInvalid)
	case limit == 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	if q.Cursor != "" {
		if _, err := uuid.Parse(q.Cursor); err != nil {
			return Page{}, fmt.Errorf("cursor is not valid: %w", app.ErrInvalid)
		}
	}

	rows, err := s.Repo.List(ctx, ListFilter{
		WorkspaceID: p.WorkspaceID, UserID: p.UserID, Role: q.Role, Statuses: statuses, BeforeID: q.Cursor, Limit: limit + 1,
	})
	if err != nil {
		if q.Cursor != "" && isNotFound(err) {
			return Page{}, fmt.Errorf("cursor is not valid: %w", app.ErrInvalid)
		}
		return Page{}, err
	}
	page := Page{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		page.NextCursor = rows[limit-1].ID
	}
	return page, nil
}
