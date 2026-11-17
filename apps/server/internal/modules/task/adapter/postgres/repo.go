// Package postgres implements the task Repository with sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

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
		return nil, errors.New("task: database is not configured")
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

// optID parses an ID that may be empty, which is NULL.
func optID(s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	u, err := id(s)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func str(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	}
	return fmt.Errorf("task: %s: %w", what, err)
}

func toDomain(row gen.TaskItem) domain.Task {
	return domain.Task{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), ChannelID: row.ChannelID.String(),
		SourceMessageID: str(row.SourceMessageID), MessageID: str(row.MessageID),
		Title: row.Title, Description: row.Description, CreatorID: row.CreatorID.String(), AssigneeID: str(row.AssigneeID),
		DueAt: utc(row.DueAt), Priority: domain.Priority(row.Priority), Status: domain.Status(row.Status),
		DueSoonSentAt: utc(row.DueSoonSentAt), OverdueSentAt: utc(row.OverdueSentAt),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func (r *Repo) Insert(ctx context.Context, t domain.Task) (domain.Task, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	p := gen.InsertTaskParams{
		Title: t.Title, Description: t.Description, DueAt: utc(t.DueAt),
		Priority: string(t.Priority), Status: string(t.Status), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, t.ID}, {&p.WorkspaceID, t.WorkspaceID}, {&p.ChannelID, t.ChannelID}, {&p.CreatorID, t.CreatorID}} {
		if *f.dst, err = id(f.src); err != nil {
			return domain.Task{}, err
		}
	}
	if p.SourceMessageID, err = optID(t.SourceMessageID); err != nil {
		return domain.Task{}, err
	}
	if p.AssigneeID, err = optID(t.AssigneeID); err != nil {
		return domain.Task{}, err
	}
	row, err := q.InsertTask(ctx, p)
	if err != nil {
		return domain.Task{}, wrap(err, "insert task")
	}
	return toDomain(row), nil
}

func (r *Repo) get(ctx context.Context, workspaceID, taskID string, lock bool) (domain.Task, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return domain.Task{}, err
	}
	tid, err := id(taskID)
	if err != nil {
		return domain.Task{}, err
	}
	var row gen.TaskItem
	if lock {
		row, err = q.GetTaskForUpdate(ctx, gen.GetTaskForUpdateParams{ID: tid, WorkspaceID: wid})
	} else {
		row, err = q.GetTask(ctx, gen.GetTaskParams{ID: tid, WorkspaceID: wid})
	}
	if err != nil {
		return domain.Task{}, wrap(err, "get task")
	}
	return toDomain(row), nil
}

func (r *Repo) Get(ctx context.Context, workspaceID, taskID string) (domain.Task, error) {
	return r.get(ctx, workspaceID, taskID, false)
}

func (r *Repo) GetForUpdate(ctx context.Context, workspaceID, taskID string) (domain.Task, error) {
	return r.get(ctx, workspaceID, taskID, true)
}

func (r *Repo) Update(ctx context.Context, t domain.Task) (domain.Task, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	p := gen.UpdateTaskParams{
		Title: t.Title, Description: t.Description, DueAt: utc(t.DueAt), Priority: string(t.Priority), Status: string(t.Status),
		DueSoonSentAt: utc(t.DueSoonSentAt), OverdueSentAt: utc(t.OverdueSentAt), UpdatedAt: t.UpdatedAt.UTC(),
	}
	if p.ID, err = id(t.ID); err != nil {
		return domain.Task{}, err
	}
	if p.WorkspaceID, err = id(t.WorkspaceID); err != nil {
		return domain.Task{}, err
	}
	if p.AssigneeID, err = optID(t.AssigneeID); err != nil {
		return domain.Task{}, err
	}
	row, err := q.UpdateTask(ctx, p)
	if err != nil {
		return domain.Task{}, wrap(err, "update task")
	}
	return toDomain(row), nil
}

func (r *Repo) SetMessage(ctx context.Context, workspaceID, taskID, messageID string) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	p := gen.SetTaskMessageParams{}
	if p.WorkspaceID, err = id(workspaceID); err != nil {
		return false, err
	}
	if p.ID, err = id(taskID); err != nil {
		return false, err
	}
	mid, err := id(messageID)
	if err != nil {
		return false, err
	}
	p.MessageID = &mid
	n, err := q.SetTaskMessage(ctx, p)
	if err != nil {
		return false, wrap(err, "set task message")
	}
	return n == 1, nil
}

func (r *Repo) List(ctx context.Context, workspaceID string, f usecase.Filter, afterID string, limit int) ([]domain.Task, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	p := gen.ListTasksParams{
		Statuses: make([]string, len(f.Statuses)), DueFrom: utc(f.DueFrom), DueBefore: utc(f.DueBefore),
		OpenOnly: f.OpenOnly, PageSize: int32(limit),
	}
	for i, s := range f.Statuses {
		p.Statuses[i] = string(s)
	}
	if p.WorkspaceID, err = id(workspaceID); err != nil {
		return nil, err
	}
	for _, o := range []struct {
		dst **uuid.UUID
		src string
	}{{&p.AssigneeID, f.AssigneeID}, {&p.CreatorID, f.CreatorID}, {&p.ChannelID, f.ChannelID}, {&p.AfterID, afterID}} {
		if *o.dst, err = optID(o.src); err != nil {
			return nil, err
		}
	}
	rows, err := q.ListTasks(ctx, p)
	if err != nil {
		return nil, wrap(err, "list tasks")
	}
	out := make([]domain.Task, len(rows))
	for i, row := range rows {
		out[i] = toDomain(row)
	}
	return out, nil
}

func due(rows []gen.ClaimDueSoonRow) []usecase.Due {
	out := make([]usecase.Due, 0, len(rows))
	for _, r := range rows {
		if r.DueAt == nil {
			continue // the query only returns rows with a due date
		}
		out = append(out, usecase.Due{TaskID: r.ID.String(), WorkspaceID: r.WorkspaceID.String(), AssigneeID: str(r.AssigneeID), DueAt: r.DueAt.UTC()})
	}
	return out
}

func (r *Repo) ClaimDueSoon(ctx context.Context, now, horizon time.Time, batch int) ([]usecase.Due, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ClaimDueSoon(ctx, gen.ClaimDueSoonParams{Now: now.UTC(), Horizon: horizon.UTC(), Batch: int32(batch)})
	if err != nil {
		return nil, wrap(err, "claim due soon")
	}
	return due(rows), nil
}

func (r *Repo) ClaimOverdue(ctx context.Context, now time.Time, batch int) ([]usecase.Due, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ClaimOverdue(ctx, gen.ClaimOverdueParams{Now: now.UTC(), Batch: int32(batch)})
	if err != nil {
		return nil, wrap(err, "claim overdue")
	}
	out := make([]gen.ClaimDueSoonRow, len(rows))
	for i, row := range rows {
		out[i] = gen.ClaimDueSoonRow(row)
	}
	return due(out), nil
}
