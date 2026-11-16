// Package usecase holds the notify flows: turning events into stored
// notifications (NOTI-01, NOTI-02) and serving the inbox (NOTI-04).
package usecase

import (
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	// DefaultLimit and MaxLimit bound a page of notifications.
	DefaultLimit = 20
	MaxLimit     = 50
)

// Service holds the notify flows. Every method that acts for a user takes the
// Principal and reads the workspace from it only.
type Service struct {
	tx app.TxManager
	// rules is what other modules registered in Init. It is nil when the
	// process collects no rules, e.g. a bare test.
	rules app.NotificationRuleSource
	repo  Repository
	ev    Events // nil tolerates realtime being unconfigured
	log   *slog.Logger
	now   func() time.Time
}

// New builds a Service. rules may be nil; ev may be nil when realtime is not
// configured. now is injected so tests can control time.
func New(tx app.TxManager, repo Repository, rules app.NotificationRuleSource, ev Events, log *slog.Logger, now func() time.Time) *Service {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{tx: tx, repo: repo, rules: rules, ev: ev, log: log, now: now}
}

// clock returns the current time in UTC, rounded to what Postgres stores.
func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func newID() string { return uuid.Must(uuid.NewV7()).String() }
