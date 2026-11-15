package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

// InviteView is an invite. Token is set only by CreateInvite: it is the one
// time the raw token exists.
type InviteView struct {
	domain.Invite
	Token string
}

func requireManager(p app.Principal) error {
	if !domain.CanManage(p.Role) {
		return fmt.Errorf("only owners and admins manage invites: %w", app.ErrForbidden)
	}
	return nil
}

// CreateInvite makes an invite for the principal's workspace (WSP-02). email
// may be empty for a link anyone can use once.
func (s *Service) CreateInvite(ctx context.Context, p app.Principal, email string, role app.Role) (InviteView, error) {
	if err := requireManager(p); err != nil {
		return InviteView{}, err
	}
	if err := domain.ValidateInviteRole(role); err != nil {
		return InviteView{}, err
	}
	email, err := domain.NormalizeInviteEmail(email)
	if err != nil {
		return InviteView{}, err
	}
	now := s.now().UTC()
	token := domain.NewToken()
	inv := domain.Invite{
		ID: newID(), WorkspaceID: p.WorkspaceID, Email: email, Role: role, CreatedBy: p.UserID,
		ExpiresAt: now.Add(domain.InviteTTL), CreatedAt: now,
	}
	if err := s.repo.InsertInvite(ctx, inv, domain.HashToken(token)); err != nil {
		return InviteView{}, err
	}
	return InviteView{Invite: inv, Token: token}, nil
}

// ListInvites returns the pending invites of the principal's workspace.
func (s *Service) ListInvites(ctx context.Context, p app.Principal) ([]domain.Invite, error) {
	if err := requireManager(p); err != nil {
		return nil, err
	}
	return s.repo.ListPendingInvites(ctx, p.WorkspaceID, s.now().UTC())
}

// RevokeInvite cancels a pending invite of the principal's workspace.
func (s *Service) RevokeInvite(ctx context.Context, p app.Principal, inviteID string) error {
	if err := requireManager(p); err != nil {
		return err
	}
	return s.repo.RevokeInvite(ctx, p.WorkspaceID, inviteID, s.now().UTC())
}

// AcceptInvite adds the caller to the invite's workspace with the invite's
// role. An unknown, expired, revoked or used token is ErrNotFound; an email
// invite for another address is ErrForbidden; already being a member (active
// or deactivated) is ErrConflict and leaves the invite unused.
func (s *Service) AcceptInvite(ctx context.Context, userID, token string) (WorkspaceView, error) {
	var joined WorkspaceView
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		inv, err := s.repo.InviteByTokenHash(ctx, domain.HashToken(token))
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if !inv.Usable(now) {
			return inv.CheckAccept(now, "")
		}
		email := ""
		if inv.Email != "" {
			// Fail closed: without the caller's email an email invite cannot be checked.
			email, err = s.emailOf(ctx, userID)
			if err != nil {
				return fmt.Errorf("cannot verify the invited email address: %w", app.ErrForbidden)
			}
		}
		if err := inv.CheckAccept(now, email); err != nil {
			return err
		}
		if _, err := s.repo.GetMember(ctx, inv.WorkspaceID, userID); err == nil {
			return fmt.Errorf("already a member of this workspace: %w", app.ErrConflict)
		} else if !errors.Is(err, app.ErrNotFound) {
			return err
		}
		if err := s.repo.InsertMember(ctx, domain.Member{WorkspaceID: inv.WorkspaceID, UserID: userID, Role: inv.Role, Status: domain.StatusActive, JoinedAt: now}); err != nil {
			return err
		}
		if err := s.repo.MarkInviteUsed(ctx, inv.WorkspaceID, inv.ID, userID, now); err != nil {
			return err
		}
		joined, err = s.repo.GetWorkspaceForUser(ctx, inv.WorkspaceID, userID)
		return err
	})
	return joined, err
}

func (s *Service) emailOf(ctx context.Context, userID string) (string, error) {
	users, err := s.dir.Users(ctx, []string{userID})
	if err != nil {
		return "", err
	}
	u, ok := users[userID]
	if !ok || u.Email == "" {
		return "", fmt.Errorf("user %s has no known email", userID)
	}
	return u.Email, nil
}
