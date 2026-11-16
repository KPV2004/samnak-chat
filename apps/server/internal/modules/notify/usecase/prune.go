package usecase

import (
	"context"
	"time"
)

// PruneAfter is how long a read notification is kept. The notify.prune job
// runs daily, so a row lives between 90 and 91 days (SRS section 8).
const PruneAfter = 90 * 24 * time.Hour

// Prune deletes the caller-less old rows: read notifications created before
// PruneAfter. It never deletes unread ones.
func (s *Service) Prune(ctx context.Context) (int64, error) {
	return s.repo.Prune(ctx, s.clock().Add(-PruneAfter))
}
