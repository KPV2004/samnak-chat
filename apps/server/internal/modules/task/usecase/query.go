package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// Get returns one task the caller can read (TASK-05). A task in a channel the
// caller cannot read is app.ErrNotFound, like a missing one.
func (s *Service) Get(ctx context.Context, p app.Principal, id string) (domain.Task, error) {
	t, err := s.d.Repo.Get(ctx, p.WorkspaceID, id)
	return s.readable(ctx, p, t, err)
}

// ListQuery selects a page of tasks, newest first. Empty fields do not filter.
type ListQuery struct {
	Assignee  string   // "me" or "any"; both are allowed, only "me" filters
	Creator   string   // "me"
	Statuses  []string // todo, doing, done
	ChannelID string
	// Due is "overdue" (past due and not done), "today" or "week" (this UTC
	// day, or the next seven days from the start of it).
	Due    string
	Cursor string // the ID of the last task of the previous page
	Limit  int
}

// Page is one page of tasks. NextCursor is empty on the last page.
type Page struct {
	Items      []domain.Task
	NextCursor string
}

// The list looks at the database in batches and keeps what the caller can
// read, so a person who sees few channels is not shown an empty page while
// more of their tasks wait further back.
const (
	scanMin    = 50
	scanRounds = 20
)

// List returns the tasks the caller can read: those whose channel passes
// ChannelAccess (TASK-04, TASK-05).
func (s *Service) List(ctx context.Context, p app.Principal, q ListQuery) (Page, error) {
	flt, err := s.filter(p, q)
	if err != nil {
		return Page{}, err
	}
	if q.Cursor != "" && !validID(q.Cursor) {
		return Page{}, invalid("cursor is not valid")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)

	readable := map[string]bool{}
	can := func(channelID string) (bool, error) {
		if ok, seen := readable[channelID]; seen {
			return ok, nil
		}
		ok, err := s.canRead(ctx, p.WorkspaceID, channelID, p.UserID)
		if err == nil {
			readable[channelID] = ok
		}
		return ok, err
	}
	if flt.ChannelID != "" {
		if ok, err := can(flt.ChannelID); err != nil || !ok {
			return Page{}, err
		}
	}

	batch := max(limit+1, scanMin)
	after := q.Cursor
	var items []domain.Task
	for range scanRounds {
		rows, err := s.d.Repo.List(ctx, p.WorkspaceID, flt, after, batch)
		if err != nil {
			return Page{}, err
		}
		for _, t := range rows {
			ok, err := can(t.ChannelID)
			if err != nil {
				return Page{}, err
			}
			if ok {
				items = append(items, t)
			}
		}
		if len(items) > limit {
			items = items[:limit+1]
			break
		}
		if len(rows) < batch {
			return Page{Items: items}, nil
		}
		after = rows[len(rows)-1].ID
	}
	if len(items) > limit {
		// More tasks exist: continue after the last one shown.
		items = items[:limit]
		return Page{Items: items, NextCursor: items[limit-1].ID}, nil
	}
	// The scan gave up before reaching the end; continue where it stopped.
	return Page{Items: items, NextCursor: after}, nil
}

func (s *Service) filter(p app.Principal, q ListQuery) (Filter, error) {
	f := Filter{ChannelID: q.ChannelID}
	switch q.Assignee {
	case "", "any":
	case "me":
		f.AssigneeID = p.UserID
	default:
		return Filter{}, invalid("assignee must be me or any")
	}
	switch q.Creator {
	case "":
	case "me":
		f.CreatorID = p.UserID
	default:
		return Filter{}, invalid("creator must be me")
	}
	for _, name := range q.Statuses {
		st := domain.Status(name)
		if !st.Valid() {
			return Filter{}, invalid("status must be todo, doing or done")
		}
		f.Statuses = append(f.Statuses, st)
	}
	if q.ChannelID != "" && !validID(q.ChannelID) {
		return Filter{}, invalid("channel_id is not a valid ID")
	}
	now := s.clock()
	day := now.Truncate(24 * time.Hour)
	switch q.Due {
	case "":
	case "overdue":
		f.DueBefore, f.OpenOnly = &now, true
	case "today":
		end := day.Add(24 * time.Hour)
		f.DueFrom, f.DueBefore = &day, &end
	case "week":
		end := day.Add(7 * 24 * time.Hour)
		f.DueFrom, f.DueBefore = &day, &end
	default:
		return Filter{}, invalid("due must be overdue, today or week")
	}
	return f, nil
}
