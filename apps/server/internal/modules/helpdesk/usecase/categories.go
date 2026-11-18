package usecase

import (
	"context"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// CreateCategory adds a category to an active queue (HD-02). A default
// category takes the default from the others of the workspace. Admins only.
func (s *Service) CreateCategory(ctx context.Context, p app.Principal, in domain.CategoryInput) (domain.Category, error) {
	if err := s.requireAdmin(p); err != nil {
		return domain.Category{}, err
	}
	if err := s.checkQueue(ctx, p.WorkspaceID, in.QueueID); err != nil {
		return domain.Category{}, err
	}
	c, err := domain.NewCategory(in, newID(), p.WorkspaceID, s.clock())
	if err != nil {
		return domain.Category{}, err
	}
	err = s.d.Tx.Do(ctx, func(ctx context.Context) error {
		if c.IsDefault {
			if err := s.d.Repo.ClearDefault(ctx, p.WorkspaceID, ""); err != nil {
				return err
			}
		}
		return s.d.Repo.InsertCategory(ctx, c)
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// UpdateCategory renames, moves, makes default or archives a category. A
// category moved to another queue keeps its number on the tickets it has.
// Admins only.
func (s *Service) UpdateCategory(ctx context.Context, p app.Principal, id string, patch domain.CategoryPatch) (domain.Category, error) {
	if err := s.requireAdmin(p); err != nil {
		return domain.Category{}, err
	}
	c, err := s.d.Repo.Category(ctx, p.WorkspaceID, id)
	if err != nil {
		return domain.Category{}, err
	}
	if patch.QueueID != nil && *patch.QueueID != c.QueueID {
		if err := s.checkQueue(ctx, p.WorkspaceID, *patch.QueueID); err != nil {
			return domain.Category{}, err
		}
	}
	if err := c.Apply(patch, s.clock()); err != nil {
		return domain.Category{}, err
	}
	err = s.d.Tx.Do(ctx, func(ctx context.Context) error {
		if c.IsDefault {
			if err := s.d.Repo.ClearDefault(ctx, p.WorkspaceID, c.ID); err != nil {
				return err
			}
		}
		return s.d.Repo.UpdateCategory(ctx, c)
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// checkQueue requires an existing, active queue of the workspace. The queue
// is named in the body, so a missing one is a 400, not a 404.
func (s *Service) checkQueue(ctx context.Context, workspaceID, queueID string) error {
	if !validID(queueID) {
		return invalid("queue_id is not a valid ID")
	}
	q, err := s.d.Repo.Queue(ctx, workspaceID, queueID)
	if err != nil {
		if isNotFound(err) {
			return invalid("queue_id is not a queue of this workspace")
		}
		return err
	}
	if q.ArchivedAt != nil {
		return invalid("queue %s is archived", q.Prefix)
	}
	return nil
}
