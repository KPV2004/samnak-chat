package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// QueueSummary is a queue with its categories that are not archived, for the
// new-ticket form and the admin page (HD-01, HD-02).
type QueueSummary struct {
	Queue      domain.Queue
	Categories []domain.Category
}

// ListQueues returns the active queues of the workspace, each with its
// active categories. Every member may read them, because the new-ticket form
// needs them.
func (s *Service) ListQueues(ctx context.Context, p app.Principal) ([]QueueSummary, error) {
	queues, err := s.d.Repo.Queues(ctx, p.WorkspaceID, false)
	if err != nil {
		return nil, err
	}
	cats, err := s.d.Repo.Categories(ctx, p.WorkspaceID, false)
	if err != nil {
		return nil, err
	}
	out := make([]QueueSummary, len(queues))
	index := make(map[string]int, len(queues))
	for i, q := range queues {
		out[i] = QueueSummary{Queue: q, Categories: []domain.Category{}}
		index[q.ID] = i
	}
	for _, c := range cats {
		if i, ok := index[c.QueueID]; ok {
			out[i].Categories = append(out[i].Categories, c)
		}
	}
	return out, nil
}

// CreateQueue adds a queue. Admins only (HD-01); a duplicate prefix is a
// conflict.
func (s *Service) CreateQueue(ctx context.Context, p app.Principal, in domain.QueueInput) (domain.Queue, error) {
	if err := s.requireAdmin(p); err != nil {
		return domain.Queue{}, err
	}
	if in.ChannelID != "" && !validID(in.ChannelID) {
		return domain.Queue{}, invalid("channel_id is not a valid ID")
	}
	q, err := domain.NewQueue(in, newID(), p.WorkspaceID, s.clock())
	if err != nil {
		return domain.Queue{}, err
	}
	if err := s.d.Repo.InsertQueue(ctx, q); err != nil {
		return domain.Queue{}, err
	}
	return q, nil
}

// UpdateQueue renames a queue, sets or clears its agents' channel, and
// archives or restores it. Admins only.
func (s *Service) UpdateQueue(ctx context.Context, p app.Principal, id string, patch domain.QueuePatch) (domain.Queue, error) {
	if err := s.requireAdmin(p); err != nil {
		return domain.Queue{}, err
	}
	if patch.ChannelSet && patch.ChannelID != "" && !validID(patch.ChannelID) {
		return domain.Queue{}, invalid("channel_id is not a valid ID")
	}
	q, err := s.d.Repo.Queue(ctx, p.WorkspaceID, id)
	if err != nil {
		return domain.Queue{}, err
	}
	if err := q.Apply(patch, s.clock()); err != nil {
		return domain.Queue{}, err
	}
	if err := s.d.Repo.UpdateQueue(ctx, q); err != nil {
		return domain.Queue{}, err
	}
	return q, nil
}

// SetAgents replaces the agents of a queue (HD-01). Each must be an active
// member of the workspace; an empty list leaves the queue without agents.
// Admins only.
func (s *Service) SetAgents(ctx context.Context, p app.Principal, queueID string, userIDs []string) error {
	if err := s.requireAdmin(p); err != nil {
		return err
	}
	if _, err := s.d.Repo.Queue(ctx, p.WorkspaceID, queueID); err != nil {
		return err
	}
	agents, err := s.checkAgents(ctx, p.WorkspaceID, userIDs)
	if err != nil {
		return err
	}
	return s.d.Tx.Do(ctx, func(ctx context.Context) error {
		return s.d.Repo.SetAgents(ctx, p.WorkspaceID, queueID, agents, s.clock())
	})
}

// checkAgents returns the IDs without repeats, in the order given. Each must
// be a UUID of an active member (400 otherwise).
func (s *Service) checkAgents(ctx context.Context, workspaceID string, userIDs []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(userIDs))
	for _, raw := range userIDs {
		if !validID(raw) {
			return nil, invalid("user_ids must be UUIDs")
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		if !s.active(ctx, workspaceID, raw) {
			return nil, fmt.Errorf("user %s is not an active member of this workspace: %w", raw, app.ErrInvalid)
		}
		out = append(out, raw)
	}
	return out, nil
}
