// Package postgres implements the workspace Repository with sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

const uniqueViolation = "23505"

// Repo implements usecase.Repository. It runs on the transaction in the
// context when there is one, otherwise on the pool.
type Repo struct {
	pool *pgxpool.Pool
}

var _ usecase.Repository = (*Repo)(nil)

// New returns a Repo. pool may be nil when the process builds the OpenAPI
// document without a database; every call then fails with an error.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) q(ctx context.Context) (*gen.Queries, error) {
	if r.pool == nil {
		return nil, errors.New("workspace: database is not configured")
	}
	return gen.New(database.Querier(ctx, r.pool)), nil
}

// id parses an ID. Text that is not a UUID cannot match any row, so it is
// reported as not found instead of reaching the database.
func id(s string) (uuid.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%q: %w", s, app.ErrNotFound)
	}
	return u, nil
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == uniqueViolation {
		return fmt.Errorf("%s: %w", what, app.ErrConflict)
	}
	return fmt.Errorf("workspace: %s: %w", what, err)
}

func (r *Repo) InsertWorkspace(ctx context.Context, w domain.Workspace) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(w.ID)
	if err != nil {
		return err
	}
	return wrap(q.InsertWorkspace(ctx, gen.InsertWorkspaceParams{ID: wid, Name: w.Name, Slug: w.Slug, CreatedAt: w.CreatedAt.UTC()}), "insert workspace")
}

func (r *Repo) ListWorkspaces(ctx context.Context, userID string) ([]usecase.WorkspaceView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListWorkspacesForUser(ctx, uid)
	if err != nil {
		return nil, wrap(err, "list workspaces")
	}
	out := make([]usecase.WorkspaceView, len(rows))
	for i, row := range rows {
		out[i] = usecase.WorkspaceView{
			Workspace: domain.Workspace{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt.UTC()},
			Role:      app.Role(row.Role),
		}
	}
	return out, nil
}

func (r *Repo) GetWorkspaceForUser(ctx context.Context, workspaceID, userID string) (usecase.WorkspaceView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return usecase.WorkspaceView{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return usecase.WorkspaceView{}, err
	}
	uid, err := id(userID)
	if err != nil {
		return usecase.WorkspaceView{}, err
	}
	row, err := q.GetWorkspaceForUser(ctx, gen.GetWorkspaceForUserParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return usecase.WorkspaceView{}, wrap(err, "get workspace")
	}
	return usecase.WorkspaceView{
		Workspace: domain.Workspace{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt.UTC()},
		Role:      app.Role(row.Role),
	}, nil
}

func (r *Repo) LockWorkspace(ctx context.Context, workspaceID string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	_, err = q.LockWorkspace(ctx, wid)
	return wrap(err, "lock workspace")
}

func (r *Repo) InsertMember(ctx context.Context, m domain.Member) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(m.WorkspaceID)
	if err != nil {
		return err
	}
	uid, err := id(m.UserID)
	if err != nil {
		return err
	}
	return wrap(q.InsertMember(ctx, gen.InsertMemberParams{
		WorkspaceID: wid, UserID: uid, Role: string(m.Role), Status: string(m.Status), JoinedAt: m.JoinedAt.UTC(),
	}), "insert member")
}

func toMember(row gen.WorkspaceMember) domain.Member {
	return domain.Member{
		WorkspaceID: row.WorkspaceID.String(), UserID: row.UserID.String(),
		Role: app.Role(row.Role), Status: domain.Status(row.Status), JoinedAt: row.JoinedAt.UTC(),
	}
}

func (r *Repo) GetMember(ctx context.Context, workspaceID, userID string) (domain.Member, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Member{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return domain.Member{}, err
	}
	uid, err := id(userID)
	if err != nil {
		return domain.Member{}, err
	}
	row, err := q.GetMember(ctx, gen.GetMemberParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return domain.Member{}, wrap(err, "member")
	}
	return toMember(row), nil
}

func (r *Repo) ActiveRole(ctx context.Context, workspaceID, userID string) (app.Role, error) {
	q, err := r.q(ctx)
	if err != nil {
		return "", err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return "", err
	}
	uid, err := id(userID)
	if err != nil {
		return "", err
	}
	role, err := q.GetActiveMemberRole(ctx, gen.GetActiveMemberRoleParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return "", wrap(err, "member")
	}
	return app.Role(role), nil
}

func (r *Repo) ListMembers(ctx context.Context, workspaceID string, after *usecase.MemberCursor, limit int) ([]domain.Member, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	p := gen.ListMembersParams{WorkspaceID: wid, PageSize: int32(limit)}
	if after != nil {
		cursorUser, err := id(after.UserID)
		if err != nil {
			return nil, fmt.Errorf("cursor: %w", app.ErrInvalid)
		}
		p.UseCursor, p.AfterJoinedAt, p.AfterUserID = true, after.JoinedAt.UTC(), cursorUser
	}
	rows, err := q.ListMembers(ctx, p)
	if err != nil {
		return nil, wrap(err, "list members")
	}
	out := make([]domain.Member, len(rows))
	for i, row := range rows {
		out[i] = toMember(row)
	}
	return out, nil
}

func (r *Repo) CountActiveOwners(ctx context.Context, workspaceID string) (int, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountActiveOwners(ctx, wid)
	return int(n), wrap(err, "count owners")
}

func (r *Repo) UpdateMember(ctx context.Context, m domain.Member) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(m.WorkspaceID)
	if err != nil {
		return err
	}
	uid, err := id(m.UserID)
	if err != nil {
		return err
	}
	n, err := q.UpdateMember(ctx, gen.UpdateMemberParams{WorkspaceID: wid, UserID: uid, Role: string(m.Role), Status: string(m.Status)})
	if err != nil {
		return wrap(err, "update member")
	}
	if n == 0 {
		return fmt.Errorf("member: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) DeleteMember(ctx context.Context, workspaceID, userID string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	uid, err := id(userID)
	if err != nil {
		return err
	}
	n, err := q.DeleteMember(ctx, gen.DeleteMemberParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return wrap(err, "delete member")
	}
	if n == 0 {
		return fmt.Errorf("member: %w", app.ErrNotFound)
	}
	return nil
}
