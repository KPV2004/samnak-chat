// Package usecase holds the task flows: creating, listing and editing tasks
// (TASK-01, 02, 04, 05), the task card (TASK-03), notification rules and the
// due scan (TASK-06), and the chat commands (CHAT-16).
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

const (
	// DefaultLimit and MaxLimit bound a page of tasks.
	DefaultLimit = 50
	MaxLimit     = 100
)

// Deps are the ports a Service needs. Channels, Members and Directory may be
// nil when the module providing them is missing: nothing is then readable,
// nobody is a member and names are empty. Cards may be nil: then no card is
// posted. Events may be nil in tests that do not look at events.
type Deps struct {
	Tx        app.TxManager
	Repo      Repository
	Channels  Channels
	Members   Members
	Directory Directory
	Cards     Cards
	Events    Events
	Log       *slog.Logger
	// Now is injected so tests control time.
	Now func() time.Time
}

// Service holds the task flows. Every method that acts for a user takes the
// Principal and reads the workspace from it only.
type Service struct {
	d Deps
}

// New builds a Service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// clock returns the current time in UTC, rounded to what Postgres stores.
func (s *Service) clock() time.Time { return s.d.Now().UTC().Truncate(time.Microsecond) }

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// validID reports whether s is a UUID; what is not cannot name a row.
func validID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, app.ErrInvalid)...)
}

// canRead asks chat whether the user can read the channel. Without chat's
// ChannelAccess nobody can, so tasks stay hidden rather than leak.
func (s *Service) canRead(ctx context.Context, workspaceID, channelID, userID string) (bool, error) {
	if s.d.Channels == nil {
		return false, nil
	}
	return s.d.Channels.CanRead(ctx, workspaceID, channelID, userID)
}

// readable returns the task when the caller can read its channel and
// app.ErrNotFound otherwise, so a hidden task looks like a missing one.
func (s *Service) readable(ctx context.Context, p app.Principal, t domain.Task, err error) (domain.Task, error) {
	if err != nil {
		return domain.Task{}, err
	}
	ok, err := s.canRead(ctx, p.WorkspaceID, t.ChannelID, p.UserID)
	if err != nil {
		return domain.Task{}, err
	}
	if !ok {
		return domain.Task{}, fmt.Errorf("task %q: %w", t.ID, app.ErrNotFound)
	}
	return t, nil
}

// checkAssignee requires an active member of the workspace who can read the
// channel (400 otherwise).
func (s *Service) checkAssignee(ctx context.Context, workspaceID, channelID, userID string) error {
	if !validID(userID) {
		return invalid("assignee_id is not a valid ID")
	}
	member := false
	if s.d.Members != nil {
		var err error
		if member, err = s.d.Members.IsActiveMember(ctx, workspaceID, userID); err != nil {
			return err
		}
	}
	if !member {
		return invalid("assignee is not an active member of the workspace")
	}
	ok, err := s.canRead(ctx, workspaceID, channelID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return invalid("assignee cannot read the channel")
	}
	return nil
}
