package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// Service runs the workspace use cases.
type Service struct {
	tx     app.TxManager
	repo   Repository
	dir    Directory
	events Events
	log    *slog.Logger
	now    func() time.Time
}

// New builds a Service. now is injected so tests control time.
func New(tx app.TxManager, repo Repository, dir Directory, events Events, log *slog.Logger, now func() time.Time) *Service {
	return &Service{tx: tx, repo: repo, dir: dir, events: events, log: log, now: now}
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// CreateWorkspace creates a workspace and makes userID its owner (WSP-01).
func (s *Service) CreateWorkspace(ctx context.Context, userID, name, slug string) (WorkspaceView, error) {
	name, err := domain.NormalizeName(name)
	if err != nil {
		return WorkspaceView{}, err
	}
	if err := domain.ValidateSlug(slug); err != nil {
		return WorkspaceView{}, err
	}
	now := s.now().UTC()
	w := domain.Workspace{ID: newID(), Name: name, Slug: slug, CreatedAt: now}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.InsertWorkspace(ctx, w); err != nil {
			if errors.Is(err, app.ErrConflict) {
				return fmt.Errorf("slug %q is taken: %w", slug, app.ErrConflict)
			}
			return err
		}
		return s.repo.InsertMember(ctx, domain.Member{WorkspaceID: w.ID, UserID: userID, Role: app.RoleOwner, Status: domain.StatusActive, JoinedAt: now})
	})
	if err != nil {
		return WorkspaceView{}, err
	}
	return WorkspaceView{Workspace: w, Role: app.RoleOwner}, nil
}

// ListWorkspaces returns the workspaces userID is an active member of.
func (s *Service) ListWorkspaces(ctx context.Context, userID string) ([]WorkspaceView, error) {
	return s.repo.ListWorkspaces(ctx, userID)
}

// ActiveRole is the app.MembershipResolver behaviour: only active members
// have a role; everyone else gets ErrForbidden. It never caches (WSP-08).
func (s *Service) ActiveRole(ctx context.Context, workspaceID, userID string) (app.Role, error) {
	role, err := s.repo.ActiveRole(ctx, workspaceID, userID)
	if errors.Is(err, app.ErrNotFound) {
		return "", fmt.Errorf("not an active member: %w", app.ErrForbidden)
	}
	return role, err
}

// MemberView is a member with the name and email the auth module knows.
type MemberView struct {
	domain.Member
	DisplayName string
	Email       string
}

// MemberPage is one page of members. NextCursor is empty on the last page.
type MemberPage struct {
	Items      []MemberView
	NextCursor string
}

// ListMembers returns a page of the principal's workspace, any status.
func (s *Service) ListMembers(ctx context.Context, p app.Principal, cursor string, limit int) (MemberPage, error) {
	after, err := decodeCursor(cursor)
	if err != nil {
		return MemberPage{}, err
	}
	if limit < 1 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)
	rows, err := s.repo.ListMembers(ctx, p.WorkspaceID, after, limit+1)
	if err != nil {
		return MemberPage{}, err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeCursor(MemberCursor{JoinedAt: last.JoinedAt, UserID: last.UserID})
	}
	return MemberPage{Items: s.withNames(ctx, rows), NextCursor: next}, nil
}

// withNames fills names and emails from the directory. When the directory is
// unavailable the strings stay empty: a members list is still useful.
func (s *Service) withNames(ctx context.Context, members []domain.Member) []MemberView {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	users, err := s.dir.Users(ctx, ids)
	if err != nil {
		s.log.WarnContext(ctx, "workspace: user directory unavailable, listing members without names", "error", err)
		users = nil
	}
	out := make([]MemberView, len(members))
	for i, m := range members {
		u := users[m.UserID]
		out[i] = MemberView{Member: m, DisplayName: u.DisplayName, Email: u.Email}
	}
	return out
}

// UpdateMember changes a member's role and/or status (WSP-04, WSP-08).
func (s *Service) UpdateMember(ctx context.Context, p app.Principal, targetID string, role *app.Role, status *domain.Status) (MemberView, error) {
	var updated domain.Member
	var deactivated bool
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		target, actorRole, owners, err := s.loadForChange(ctx, p, targetID)
		if err != nil {
			return err
		}
		updated, err = domain.CheckChange(actorRole, target, role, status, owners)
		if err != nil {
			return err
		}
		deactivated = target.Status == domain.StatusActive && updated.Status == domain.StatusDeactivated
		if err := s.repo.UpdateMember(ctx, updated); err != nil {
			return err
		}
		return s.record(ctx, p, domain.AuditMemberUpdated, domain.TargetMember, updated.UserID, memberChanges(target, updated, role, status))
	})
	if err != nil {
		return MemberView{}, err
	}
	if deactivated {
		s.events.MemberDeactivated(ctx, p.WorkspaceID, updated.UserID, p.UserID)
	}
	return s.withNames(ctx, []domain.Member{updated})[0], nil
}

// RemoveMember removes a member from the principal's workspace (WSP-04).
func (s *Service) RemoveMember(ctx context.Context, p app.Principal, targetID string) error {
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		target, actorRole, owners, err := s.loadForChange(ctx, p, targetID)
		if err != nil {
			return err
		}
		if err := domain.CheckRemove(actorRole, target, owners); err != nil {
			return err
		}
		if err := s.repo.DeleteMember(ctx, p.WorkspaceID, target.UserID); err != nil {
			return err
		}
		if err := s.repo.DeleteGroupMemberships(ctx, p.WorkspaceID, target.UserID); err != nil {
			return err
		}
		return s.record(ctx, p, domain.AuditMemberRemoved, domain.TargetMember, target.UserID, nil)
	})
	if err != nil {
		return err
	}
	s.events.MemberRemoved(ctx, p.WorkspaceID, targetID, p.UserID)
	return nil
}

// loadForChange locks the workspace, then reads the target, the actor's
// current role and the owner count. The actor's role is read again under the
// lock because the one in the Principal may be stale, and the owner count
// must be read after any concurrent change has committed.
func (s *Service) loadForChange(ctx context.Context, p app.Principal, targetID string) (target domain.Member, actorRole app.Role, owners int, err error) {
	if err = s.repo.LockWorkspace(ctx, p.WorkspaceID); err != nil {
		return
	}
	if actorRole, err = s.ActiveRole(ctx, p.WorkspaceID, p.UserID); err != nil {
		return
	}
	if !domain.CanManage(actorRole) {
		err = fmt.Errorf("only owners and admins manage members: %w", app.ErrForbidden)
		return
	}
	if target, err = s.repo.GetMember(ctx, p.WorkspaceID, targetID); err != nil {
		return
	}
	owners, err = s.repo.CountActiveOwners(ctx, p.WorkspaceID)
	return
}

// A cursor is base64url("<joined_at RFC 3339 nano>|<user id>"). Clients treat
// it as opaque.
func encodeCursor(c MemberCursor) string {
	raw := c.JoinedAt.UTC().Format(time.RFC3339Nano) + "|" + c.UserID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (*MemberCursor, error) {
	if s == "" {
		return nil, nil
	}
	bad := fmt.Errorf("cursor is not valid: %w", app.ErrInvalid)
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, bad
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, bad
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, bad
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, bad
	}
	return &MemberCursor{JoinedAt: t, UserID: id}, nil
}
