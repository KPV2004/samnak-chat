package usecase

import (
	"context"
	"errors"
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
	tx     app.TxManager
	repo   Repository
	ws     Workspace
	perms  Permissions
	dir    Directory
	ev     Events
	store  app.ObjectStorage // nil when object storage is not configured
	ext    *Extensions       // nil until WithExtensions
	typing *typingLimiter
	log    *slog.Logger
	now    func() time.Time
}

// New builds a Service. now is injected so tests can control time.
func New(tx app.TxManager, repo Repository, ws Workspace, perms Permissions, dir Directory, ev Events, store app.ObjectStorage, log *slog.Logger, now func() time.Time) *Service {
	return &Service{tx: tx, repo: repo, ws: ws, perms: perms, dir: dir, ev: ev, store: store, typing: newTypingLimiter(typingInterval, now), log: log, now: now}
}

// WithExtensions sets the slash commands and message actions the service
// serves (CHAT-16). Without it every command is unknown.
func (s *Service) WithExtensions(x *Extensions) *Service {
	s.ext = x
	return s
}

// clock returns the current time in UTC, rounded to what Postgres stores.
func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// channel loads a channel as p sees it. A channel p may not know about is
// reported as not found, so private channels do not leak.
func (s *Service) channel(ctx context.Context, p app.Principal, channelID string) (ChannelView, error) {
	return s.visibleChannel(ctx, p.WorkspaceID, p.UserID, p.Role, channelID)
}

// visibleChannel is the one channel read rule: a channel the user is in, or a
// public one (archived or not) for everyone but guests (domain.Visible).
func (s *Service) visibleChannel(ctx context.Context, workspaceID, userID string, role app.Role, channelID string) (ChannelView, error) {
	views, err := s.repo.ChannelViews(ctx, workspaceID, userID, ViewOne, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	if len(views) == 0 || !domain.Visible(views[0].Channel, role, views[0].IsMember) {
		return ChannelView{}, fmt.Errorf("channel: %w", app.ErrNotFound)
	}
	return views[0], nil
}

// CanRead implements app.ChannelAccess with the rules of channel: the user
// must be an active member of the workspace, and either be in the channel or
// find it public (guests only see public channels they are in). A malformed
// ID or a channel of another workspace is a plain false.
func (s *Service) CanRead(ctx context.Context, workspaceID, channelID, userID string) (bool, error) {
	for _, id := range []string{workspaceID, channelID, userID} {
		if _, err := uuid.Parse(id); err != nil {
			return false, nil
		}
	}
	role, active, err := s.ws.MemberRole(ctx, workspaceID, userID)
	if err != nil || !active {
		return false, err
	}
	if _, err := s.visibleChannel(ctx, workspaceID, userID, role, channelID); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ReadMessage implements app.MessageReader: a message is readable exactly
// when its channel is (CanRead). A deleted message is not found, and so is
// one the user cannot read, so the answer does not reveal which it is.
func (s *Service) ReadMessage(ctx context.Context, workspaceID, messageID, userID string) (MessageView, error) {
	notFound := fmt.Errorf("message: %w", app.ErrNotFound)
	for _, id := range []string{workspaceID, messageID, userID} {
		if _, err := uuid.Parse(id); err != nil {
			return MessageView{}, notFound
		}
	}
	role, active, err := s.ws.MemberRole(ctx, workspaceID, userID)
	if err != nil {
		return MessageView{}, err
	}
	if !active {
		return MessageView{}, notFound
	}
	m, err := s.repo.Message(ctx, workspaceID, messageID)
	if err != nil {
		return MessageView{}, err
	}
	if m.Deleted() {
		return MessageView{}, notFound
	}
	if _, err := s.visibleChannel(ctx, workspaceID, userID, role, m.ChannelID); err != nil {
		return MessageView{}, err
	}
	return m, nil
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
