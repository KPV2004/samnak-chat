package usecase

import (
	"context"
	"time"
)

// DueBatch is how many tasks one due-scan transaction claims.
const DueBatch = 100

// dueSoonWindow is how long before the due date the first reminder goes out.
const dueSoonWindow = 24 * time.Hour

// DueScan announces tasks that are due within 24 hours (task.due_soon) and
// tasks past their due date (task.overdue), once per due date (TASK-06). Each
// batch is one transaction that sets the sent marks and publishes the events
// together, so a failure leaves both undone and the next scan tries again.
// Two servers scanning at once claim different rows.
func (s *Service) DueScan(ctx context.Context) error {
	for _, step := range []struct {
		claim   func(ctx context.Context, now time.Time) ([]Due, error)
		publish func(ctx context.Context, d Due) error
	}{
		{
			claim: func(ctx context.Context, now time.Time) ([]Due, error) {
				return s.d.Repo.ClaimDueSoon(ctx, now, now.Add(dueSoonWindow), DueBatch)
			},
			publish: func(ctx context.Context, d Due) error {
				return s.publish(func(ev Events) error { return ev.DueSoon(ctx, d) })
			},
		},
		{
			claim: func(ctx context.Context, now time.Time) ([]Due, error) {
				return s.d.Repo.ClaimOverdue(ctx, now, DueBatch)
			},
			publish: func(ctx context.Context, d Due) error {
				return s.publish(func(ev Events) error { return ev.Overdue(ctx, d) })
			},
		},
	} {
		for {
			n := 0
			err := s.d.Tx.Do(ctx, func(ctx context.Context) error {
				due, err := step.claim(ctx, s.clock())
				if err != nil {
					return err
				}
				n = len(due)
				for _, d := range due {
					if err := step.publish(ctx, d); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			if n < DueBatch {
				break
			}
		}
	}
	return nil
}
