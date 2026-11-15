package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

const (
	maxMemberIDs = 100
	// DefaultLimit and MaxLimit bound a page of messages (CHAT-09).
	DefaultLimit = 50
	MaxLimit     = 100
)

// Service holds the chat flows. Every method that acts for a user takes the
// Principal and reads the workspace from it only.
type Service struct {
	tx    app.TxManager
	repo  Repository
	ws    Workspace
	perms Permissions
	dir   Directory
	ev    Events
	store app.ObjectStorage // nil when object storage is not configured
	log   *slog.Logger
	now   func() time.Time
}

// New builds a Service. now is injected so tests can control time.
func New(tx app.TxManager, repo Repository, ws Workspace, perms Permissions, dir Directory, ev Events, store app.ObjectStorage, log *slog.Logger, now func() time.Time) *Service {
	return &Service{tx: tx, repo: repo, ws: ws, perms: perms, dir: dir, ev: ev, store: store, log: log, now: now}
}

// clock returns the current time in UTC, rounded to what Postgres stores.
func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// channel loads a channel as p sees it. A channel p may not know about is
// reported as not found, so private channels do not leak.
func (s *Service) channel(ctx context.Context, p app.Principal, channelID string) (ChannelView, error) {
	views, err := s.repo.ChannelViews(ctx, p.WorkspaceID, p.UserID, ViewOne, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	if len(views) == 0 || !domain.Visible(views[0].Channel, p.Role, views[0].IsMember) {
		return ChannelView{}, fmt.Errorf("channel: %w", app.ErrNotFound)
	}
	return views[0], nil
}

// memberChannel is channel for actions that need p to be in the channel.
func (s *Service) memberChannel(ctx context.Context, p app.Principal, channelID string) (ChannelView, error) {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	if !v.IsMember {
		return ChannelView{}, fmt.Errorf("join the channel first: %w", app.ErrForbidden)
	}
	return v, nil
}

// checkWorkspaceMembers fails with ErrInvalid unless every user is an active
// member of the caller's workspace.
func (s *Service) checkWorkspaceMembers(ctx context.Context, workspaceID string, userIDs []string) error {
	for _, uid := range userIDs {
		ok, err := s.ws.IsActiveMember(ctx, workspaceID, uid)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("user %s is not a member of this workspace: %w", uid, app.ErrInvalid)
		}
	}
	return nil
}

// others parses ids, drops the caller and checks the count.
func others(self string, userIDs []string) ([]string, error) {
	if len(userIDs) > maxMemberIDs {
		return nil, fmt.Errorf("at most %d users at a time: %w", maxMemberIDs, app.ErrInvalid)
	}
	parsed, err := domain.ParseIDs(userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parsed))
	for _, id := range parsed {
		if id != self {
			out = append(out, id)
		}
	}
	return out, nil
}
