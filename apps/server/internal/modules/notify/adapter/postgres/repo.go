// Package postgres implements the notify Repository with sqlc queries.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/usecase"
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
		return nil, errors.New("notify: database is not configured")
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
	return fmt.Errorf("notify: %s: %w", what, err)
}

func toDomain(row gen.NotifyNotification) domain.Notification {
	data := map[string]any{}
	if len(row.Data) > 0 {
		// The column round-trips what rules put there; a value that does not
		// decode would be a bug in the writer, so an empty map is safe.
		_ = json.Unmarshal(row.Data, &data)
	}
	return domain.Notification{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), UserID: row.UserID.String(),
		Kind: row.Kind, Title: row.Title, Body: row.Body, Link: row.Link, Data: data, Key: row.Key,
		ReadAt: utc(row.ReadAt), CreatedAt: row.CreatedAt.UTC(),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (r *Repo) Insert(ctx context.Context, n domain.Notification) (domain.Notification, bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Notification{}, false, err
	}
	var p gen.InsertNotificationParams
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, n.ID}, {&p.WorkspaceID, n.WorkspaceID}, {&p.UserID, n.UserID}} {
		if *f.dst, err = id(f.src); err != nil {
			return domain.Notification{}, false, err
		}
	}
	data := n.Data
	if data == nil {
		data = map[string]any{}
	}
	if p.Data, err = json.Marshal(data); err != nil {
		return domain.Notification{}, false, fmt.Errorf("notify: encode data: %w", err)
	}
	p.Kind, p.Title, p.Body, p.Link, p.Key, p.CreatedAt = n.Kind, n.Title, n.Body, n.Link, n.Key, n.CreatedAt.UTC()
	row, err := q.InsertNotification(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING: the key was already stored.
		return domain.Notification{}, false, nil
	}
	if err != nil {
		return domain.Notification{}, false, wrap(err, "insert notification")
	}
	return toDomain(row), true, nil
}

func (r *Repo) List(ctx context.Context, workspaceID, userID string, unreadOnly bool, beforeID string, limit int) ([]domain.Notification, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	p := gen.ListNotificationsParams{
		UserID: uid, WorkspaceID: wid, UnreadOnly: unreadOnly, PageSize: int32(limit),
	}
	if beforeID != "" {
		bid, err := id(beforeID)
		if err != nil {
			return nil, err
		}
		cursor, err := q.GetNotification(ctx, gen.GetNotificationParams{ID: bid, WorkspaceID: wid, UserID: uid})
		if err != nil {
			return nil, wrap(err, "get cursor")
		}
		at := cursor.CreatedAt.UTC()
		p.BeforeCreatedAt, p.BeforeID = &at, &bid
	}
	rows, err := q.ListNotifications(ctx, p)
	if err != nil {
		return nil, wrap(err, "list notifications")
	}
	out := make([]domain.Notification, len(rows))
	for i, row := range rows {
		out[i] = toDomain(row)
	}
	return out, nil
}

func (r *Repo) UnreadCount(ctx context.Context, workspaceID, userID string) (int64, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return 0, err
	}
	uid, err := id(userID)
	if err != nil {
		return 0, err
	}
	n, err := q.UnreadCount(ctx, gen.UnreadCountParams{UserID: uid, WorkspaceID: wid})
	return n, wrap(err, "unread count")
}

func (r *Repo) MarkRead(ctx context.Context, workspaceID, userID, notificationID string, at time.Time) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return false, err
	}
	uid, err := id(userID)
	if err != nil {
		return false, err
	}
	nid, err := id(notificationID)
	if err != nil {
		return false, err
	}
	t := at.UTC()
	_, err = q.MarkRead(ctx, gen.MarkReadParams{ReadAt: &t, ID: nid, WorkspaceID: wid, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the row is not the user's or it was already read; only the
		// first is an error, because marking read is idempotent.
		if _, err := q.GetNotification(ctx, gen.GetNotificationParams{ID: nid, WorkspaceID: wid, UserID: uid}); err != nil {
			return false, wrap(err, "mark read")
		}
		return false, nil
	}
	if err != nil {
		return false, wrap(err, "mark read")
	}
	return true, nil
}

func (r *Repo) MarkAllRead(ctx context.Context, workspaceID, userID string, at time.Time) ([]string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	t := at.UTC()
	rows, err := q.MarkAllRead(ctx, gen.MarkAllReadParams{ReadAt: &t, WorkspaceID: wid, UserID: uid})
	if err != nil {
		return nil, wrap(err, "mark all read")
	}
	out := make([]string, len(rows))
	for i, u := range rows {
		out[i] = u.String()
	}
	return out, nil
}

func (r *Repo) Prune(ctx context.Context, before time.Time) (int64, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.PruneNotifications(ctx, before.UTC())
	return n, wrap(err, "prune notifications")
}
