package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Deps are the ports a Service needs. Permissions and Members may be nil: then
// nobody is an admin or an active member, which is the safe answer.
type Deps struct {
	Tx          app.TxManager
	Repo        Repository
	Permissions Permissions
	Members     Members
	// Bus receives the events inside Tx, so they reach their listeners only
	// after the commit (PLT-10). Nil in tests that do not look at events.
	Bus app.Publisher
	// Messages is nil when chat is not available; source_message_id is then
	// refused (HD-03).
	Messages Messages
	// Storage presigns uploads and downloads of files (HD-03). Nil in tests
	// that do not look at files.
	Storage app.ObjectStorage
	Log     *slog.Logger
	// Now is injected so tests control time.
	Now func() time.Time
}

// Service holds the helpdesk flows. Every method that acts for a user takes
// the Principal and reads the workspace from it only.
type Service struct {
	d Deps
}

// New builds a Service. A nil Log and Now get defaults.
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

// validID reports whether s is a UUID.
func validID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, app.ErrInvalid)...)
}

// isAdmin reports whether p holds helpdesk.admin (owners and admins).
func (s *Service) isAdmin(p app.Principal) bool {
	return s.d.Permissions != nil && s.d.Permissions.Can(p, ActionAdmin)
}

// requireAdmin answers 403 for everyone else.
func (s *Service) requireAdmin(p app.Principal) error {
	if !s.isAdmin(p) {
		return fmt.Errorf("only owners and admins manage helpdesk: %w", app.ErrForbidden)
	}
	return nil
}

// publish sends evt on the bus when there is one.
func (s *Service) publish(ctx context.Context, evt app.Event) error {
	if s.d.Bus == nil {
		return nil
	}
	return s.d.Bus.Publish(ctx, evt)
}

// active reports whether userID is an active member of the workspace.
func (s *Service) active(ctx context.Context, workspaceID, userID string) bool {
	return s.d.Members != nil && s.d.Members.Active(ctx, workspaceID, userID)
}

// isNotFound reports whether err wraps app.ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, app.ErrNotFound) }
