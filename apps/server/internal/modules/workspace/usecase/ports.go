// Package usecase holds the workspace flows and the ports they need.
package usecase

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

// WorkspaceView is a workspace with the caller's role in it.
type WorkspaceView struct {
	domain.Workspace
	Role app.Role
}

// MemberCursor is the position after the last member of a page.
type MemberCursor struct {
	JoinedAt time.Time
	UserID   string
}

// Repository is the storage the use cases need. Every method that touches
// workspace data takes the workspace ID, which callers read from the
// Principal. Methods return an error wrapping app.ErrNotFound for a missing
// row and app.ErrConflict for a uniqueness violation.
type Repository interface {
	InsertWorkspace(ctx context.Context, w domain.Workspace) error
	ListWorkspaces(ctx context.Context, userID string) ([]WorkspaceView, error)
	GetWorkspaceForUser(ctx context.Context, workspaceID, userID string) (WorkspaceView, error)
	// LockWorkspace takes a row lock until the transaction ends. Member
	// changes call it first, so they run one at a time per workspace.
	LockWorkspace(ctx context.Context, workspaceID string) error

	InsertMember(ctx context.Context, m domain.Member) error
	GetMember(ctx context.Context, workspaceID, userID string) (domain.Member, error)
	// ActiveRole returns the role of an active member only.
	ActiveRole(ctx context.Context, workspaceID, userID string) (app.Role, error)
	// ListMembers returns up to limit members ordered by (joined_at, user_id),
	// after cursor when it is not nil.
	ListMembers(ctx context.Context, workspaceID string, after *MemberCursor, limit int) ([]domain.Member, error)
	CountActiveOwners(ctx context.Context, workspaceID string) (int, error)
	UpdateMember(ctx context.Context, m domain.Member) error
	DeleteMember(ctx context.Context, workspaceID, userID string) error
}

// User is what the use cases show about someone, read from the auth module.
type User struct {
	Email       string
	DisplayName string
}

// Directory looks up users by ID. A user it does not know is simply absent.
type Directory interface {
	Users(ctx context.Context, ids []string) (map[string]User, error)
}

// Events announces member changes. Methods are called after the transaction
// commits.
type Events interface {
	MemberRemoved(ctx context.Context, workspaceID, userID, actorID string)
	MemberDeactivated(ctx context.Context, workspaceID, userID, actorID string)
}
