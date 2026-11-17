package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

const (
	// DefaultLimit and MaxLimit bound a page of requests.
	DefaultLimit = 20
	MaxLimit     = 100
)

// Deps are what a Service needs. Everything except Tx, Repo and Bus may be
// nil: the module then works without that part, the way it does when the
// module that provides it is disabled.
type Deps struct {
	Tx   app.TxManager
	Repo Repository
	// Bus receives events inside Tx, so they reach their listeners only after
	// the commit (PLT-10).
	Bus      app.Publisher
	Members  Members
	Managers Managers
	Names    Names
	Cards    Cards
	Storage  app.ObjectStorage
	Log      *slog.Logger
	Now      func() time.Time
}

// Service holds the approval flows. Every method that acts for a user takes
// the Principal and reads the workspace from it only.
type Service struct {
	Deps
}

// New builds a Service. A nil Log and Now get defaults; Now is injected so
// tests can control time.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{Deps: d}
}

// clock returns the current time in UTC, rounded to what Postgres stores.
func (s *Service) clock() time.Time { return s.Now().UTC().Truncate(time.Microsecond) }

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// Detail is a request with its history and files, for one request's page.
type Detail struct {
	Request     domain.Request
	History     []domain.HistoryEntry
	Attachments []domain.Attachment
}

func (s *Service) detail(ctx context.Context, r domain.Request) (Detail, error) {
	hist, err := s.Repo.History(ctx, r.ID)
	if err != nil {
		return Detail{}, err
	}
	files, err := s.Repo.AttachmentsOf(ctx, r.ID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Request: r, History: hist, Attachments: files}, nil
}

func (s *Service) publish(ctx context.Context, evt app.Event) error {
	if s.Bus == nil {
		return nil
	}
	return s.Bus.Publish(ctx, evt)
}
