// Package postgres implements the approval Repository with sqlc queries.
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
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
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
		return nil, errors.New("approval: database is not configured")
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

// ids parses every ID, or fails like id.
func ids(ss []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, len(ss))
	for i, s := range ss {
		u, err := id(s)
		if err != nil {
			return nil, err
		}
		out[i] = u
	}
	return out, nil
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	}
	return fmt.Errorf("approval: %s: %w", what, err)
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func uuidString(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func toRequest(row gen.ApprovalRequest) domain.Request {
	return domain.Request{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), Type: domain.Type(row.Type),
		RequesterID: row.RequesterID.String(), ApproverID: row.ApproverID.String(), Status: domain.Status(row.Status),
		Payload: row.Payload, ChannelID: uuidString(row.ChannelID), MessageID: uuidString(row.MessageID),
		DecidedAt: utc(row.DecidedAt), Reason: row.Reason, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func toAttachment(row gen.ApprovalAttachment) domain.Attachment {
	return domain.Attachment{
		ID: row.ID.String(), RequestID: uuidString(row.RequestID), WorkspaceID: row.WorkspaceID.String(),
		UploaderID: row.UploaderID.String(), ObjectKey: row.ObjectKey, FileName: row.FileName, Size: row.Size,
		MimeType: row.MimeType, CreatedAt: row.CreatedAt.UTC(),
	}
}

func (r *Repo) InsertRequest(ctx context.Context, req domain.Request, step domain.Step, h domain.HistoryEntry) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertRequestParams{
		Type: string(req.Type), Status: string(req.Status), Payload: req.Payload,
		CreatedAt: req.CreatedAt.UTC(), UpdatedAt: req.UpdatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, req.ID}, {&p.WorkspaceID, req.WorkspaceID}, {&p.RequesterID, req.RequesterID}, {&p.ApproverID, req.ApproverID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	if err := q.InsertRequest(ctx, p); err != nil {
		return wrap(err, "insert request")
	}
	sp := gen.InsertStepParams{StepNo: int32(step.StepNo), Status: string(step.Status)}
	if sp.RequestID, err = id(step.RequestID); err != nil {
		return err
	}
	if sp.ApproverID, err = id(step.ApproverID); err != nil {
		return err
	}
	if err := q.InsertStep(ctx, sp); err != nil {
		return wrap(err, "insert step")
	}
	return r.InsertHistory(ctx, h)
}

func (r *Repo) Request(ctx context.Context, workspaceID, requestID string) (domain.Request, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Request{}, err
	}
	rid, err := id(requestID)
	if err != nil {
		return domain.Request{}, err
	}
	row, err := q.GetRequest(ctx, gen.GetRequestParams{ID: rid, WorkspaceID: ws})
	if err != nil {
		return domain.Request{}, wrap(err, "request")
	}
	return toRequest(row), nil
}

func (r *Repo) List(ctx context.Context, f usecase.ListFilter) ([]domain.Request, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := id(f.WorkspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(f.UserID)
	if err != nil {
		return nil, err
	}
	statuses := make([]string, len(f.Statuses))
	for i, s := range f.Statuses {
		statuses[i] = string(s)
	}
	p := gen.ListRequestsParams{
		WorkspaceID: ws, UserID: uid, Statuses: statuses, PageSize: int32(f.Limit),
		AsRequester: f.Role != "approver", AsApprover: f.Role != "requester",
	}
	if f.BeforeID != "" {
		bid, err := id(f.BeforeID)
		if err != nil {
			return nil, err
		}
		cursor, err := q.GetRequest(ctx, gen.GetRequestParams{ID: bid, WorkspaceID: ws})
		if err != nil {
			return nil, wrap(err, "cursor")
		}
		created := cursor.CreatedAt.UTC()
		p.BeforeCreatedAt, p.BeforeID = &created, &bid
	}
	rows, err := q.ListRequests(ctx, p)
	if err != nil {
		return nil, wrap(err, "list requests")
	}
	out := make([]domain.Request, len(rows))
	for i, row := range rows {
		out[i] = toRequest(row)
	}
	return out, nil
}

func (r *Repo) Decide(ctx context.Context, workspaceID, requestID string, to domain.Status, reason string, at time.Time) (domain.Request, bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Request{}, false, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Request{}, false, err
	}
	rid, err := id(requestID)
	if err != nil {
		return domain.Request{}, false, err
	}
	t := at.UTC()
	row, err := q.DecideRequest(ctx, gen.DecideRequestParams{
		Status: string(to), Reason: reason, At: &t, ID: rid, WorkspaceID: ws,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Not pending any more, or not there: the stored row tells which.
		cur, err := r.Request(ctx, workspaceID, requestID)
		return cur, false, err
	}
	if err != nil {
		return domain.Request{}, false, wrap(err, "decide request")
	}
	if err := q.DecideStep(ctx, gen.DecideStepParams{Status: string(to), Reason: reason, At: &t, RequestID: rid}); err != nil {
		return domain.Request{}, false, wrap(err, "decide step")
	}
	return toRequest(row), true, nil
}

func (r *Repo) InsertHistory(ctx context.Context, h domain.HistoryEntry) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertHistoryParams{Action: string(h.Action), Reason: h.Reason, At: h.At.UTC()}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, h.ID}, {&p.RequestID, h.RequestID}, {&p.ActorID, h.ActorID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	return wrap(q.InsertHistory(ctx, p), "insert history")
}

func (r *Repo) History(ctx context.Context, requestID string) ([]domain.HistoryEntry, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := id(requestID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListHistory(ctx, rid)
	if err != nil {
		return nil, wrap(err, "history")
	}
	out := make([]domain.HistoryEntry, len(rows))
	for i, row := range rows {
		out[i] = domain.HistoryEntry{
			ID: row.ID.String(), RequestID: row.RequestID.String(), ActorID: row.ActorID.String(),
			Action: domain.Action(row.Action), Reason: row.Reason, At: row.At.UTC(),
		}
	}
	return out, nil
}

func (r *Repo) SetCard(ctx context.Context, workspaceID, requestID, channelID, messageID string) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	var p gen.SetCardParams
	var channel, message uuid.UUID
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&p.ID, requestID}, {&channel, channelID}, {&message, messageID}} {
		if *f.dst, err = id(f.src); err != nil {
			return false, err
		}
	}
	p.ChannelID, p.MessageID = &channel, &message
	n, err := q.SetCard(ctx, p)
	if err != nil {
		return false, wrap(err, "set card")
	}
	return n > 0, nil
}

func (r *Repo) InsertAttachment(ctx context.Context, a domain.Attachment) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertAttachmentParams{
		ObjectKey: a.ObjectKey, FileName: a.FileName, Size: a.Size, MimeType: a.MimeType, CreatedAt: a.CreatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, a.ID}, {&p.WorkspaceID, a.WorkspaceID}, {&p.UploaderID, a.UploaderID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	return wrap(q.InsertAttachment(ctx, p), "insert attachment")
}

func (r *Repo) Attachment(ctx context.Context, workspaceID, attachmentID string) (domain.Attachment, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Attachment{}, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Attachment{}, err
	}
	aid, err := id(attachmentID)
	if err != nil {
		return domain.Attachment{}, err
	}
	row, err := q.GetAttachment(ctx, gen.GetAttachmentParams{ID: aid, WorkspaceID: ws})
	if err != nil {
		return domain.Attachment{}, wrap(err, "attachment")
	}
	return toAttachment(row), nil
}

func (r *Repo) AttachmentsByIDs(ctx context.Context, workspaceID string, attachmentIDs []string) ([]domain.Attachment, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uids, err := ids(attachmentIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetAttachmentsByIDs(ctx, gen.GetAttachmentsByIDsParams{WorkspaceID: ws, Ids: uids})
	if err != nil {
		return nil, wrap(err, "attachments")
	}
	out := make([]domain.Attachment, len(rows))
	for i, row := range rows {
		out[i] = toAttachment(row)
	}
	return out, nil
}

func (r *Repo) LinkAttachments(ctx context.Context, workspaceID, requestID, uploaderID string, attachmentIDs []string) (int64, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, err
	}
	var p gen.LinkAttachmentsParams
	var request uuid.UUID
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&request, requestID}, {&p.UploaderID, uploaderID}} {
		if *f.dst, err = id(f.src); err != nil {
			return 0, err
		}
	}
	p.RequestID = &request
	if p.Ids, err = ids(attachmentIDs); err != nil {
		return 0, err
	}
	n, err := q.LinkAttachments(ctx, p)
	return n, wrap(err, "link attachments")
}

func (r *Repo) AttachmentsOf(ctx context.Context, requestID string) ([]domain.Attachment, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := id(requestID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRequestAttachments(ctx, &rid)
	if err != nil {
		return nil, wrap(err, "request attachments")
	}
	out := make([]domain.Attachment, len(rows))
	for i, row := range rows {
		out[i] = toAttachment(row)
	}
	return out, nil
}
