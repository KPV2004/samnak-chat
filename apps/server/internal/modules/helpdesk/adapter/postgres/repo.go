// Package postgres implements the helpdesk Repository with sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
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
		return nil, errors.New("helpdesk: database is not configured")
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

// wrap maps a missing row to app.ErrNotFound and anything else to a plain
// error that names the operation.
func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	}
	return fmt.Errorf("helpdesk: %s: %w", what, err)
}

// uniqueViolation reports whether err is a unique constraint violation (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// conflict answers 409 with msg when err is a unique violation, and wraps err
// otherwise.
func conflict(err error, what, msg string) error {
	if uniqueViolation(err) {
		return fmt.Errorf("%s: %w", msg, app.ErrConflict)
	}
	return wrap(err, what)
}

func toQueue(row gen.HelpdeskQueue, agents []uuid.UUID) domain.Queue {
	out := domain.Queue{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), Name: row.Name, Prefix: row.Prefix,
		ChannelID: str(row.ChannelID), AgentIDs: []string{}, ArchivedAt: utc(row.ArchivedAt),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
	for _, a := range agents {
		out.AgentIDs = append(out.AgentIDs, a.String())
	}
	return out
}

func toCategory(row gen.HelpdeskCategory) domain.Category {
	return domain.Category{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), QueueID: row.QueueID.String(), Name: row.Name,
		IsDefault: row.IsDefault, ArchivedAt: utc(row.ArchivedAt), CreatedAt: row.CreatedAt.UTC(),
	}
}

func (r *Repo) InsertQueue(ctx context.Context, q domain.Queue) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertQueueParams{Name: q.Name, Prefix: q.Prefix, CreatedAt: q.CreatedAt.UTC(), UpdatedAt: q.UpdatedAt.UTC()}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, q.ID}, {&p.WorkspaceID, q.WorkspaceID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	if p.ChannelID, err = optID(q.ChannelID); err != nil {
		return err
	}
	return conflict(qs.InsertQueue(ctx, p), "insert queue", "prefix is already used in this workspace")
}

func (r *Repo) UpdateQueue(ctx context.Context, q domain.Queue) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.UpdateQueueParams{Name: q.Name, ArchivedAt: utc(q.ArchivedAt), UpdatedAt: q.UpdatedAt.UTC()}
	if p.ID, err = id(q.ID); err != nil {
		return err
	}
	if p.WorkspaceID, err = id(q.WorkspaceID); err != nil {
		return err
	}
	if p.ChannelID, err = optID(q.ChannelID); err != nil {
		return err
	}
	n, err := qs.UpdateQueue(ctx, p)
	if err != nil {
		return wrap(err, "update queue")
	}
	if n == 0 {
		return fmt.Errorf("queue: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) Queue(ctx context.Context, workspaceID, queueID string) (domain.Queue, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return domain.Queue{}, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Queue{}, err
	}
	qid, err := id(queueID)
	if err != nil {
		return domain.Queue{}, err
	}
	row, err := qs.GetQueue(ctx, gen.GetQueueParams{ID: qid, WorkspaceID: ws})
	if err != nil {
		return domain.Queue{}, wrap(err, "queue")
	}
	agents, err := qs.ListQueueAgents(ctx, qid)
	if err != nil {
		return domain.Queue{}, wrap(err, "list agents")
	}
	return toQueue(row, agents), nil
}

func (r *Repo) Queues(ctx context.Context, workspaceID string, includeArchived bool) ([]domain.Queue, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListQueues(ctx, gen.ListQueuesParams{WorkspaceID: ws, IncludeArchived: includeArchived})
	if err != nil {
		return nil, wrap(err, "list queues")
	}
	agentRows, err := qs.ListQueueAgentsOfWorkspace(ctx, ws)
	if err != nil {
		return nil, wrap(err, "list agents")
	}
	agents := map[uuid.UUID][]uuid.UUID{}
	for _, a := range agentRows {
		agents[a.QueueID] = append(agents[a.QueueID], a.UserID)
	}
	out := make([]domain.Queue, len(rows))
	for i, row := range rows {
		out[i] = toQueue(row, agents[row.ID])
	}
	return out, nil
}

func (r *Repo) SetAgents(ctx context.Context, workspaceID, queueID string, userIDs []string, at time.Time) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return err
	}
	qid, err := id(queueID)
	if err != nil {
		return err
	}
	if _, err := qs.GetQueue(ctx, gen.GetQueueParams{ID: qid, WorkspaceID: ws}); err != nil {
		return wrap(err, "queue")
	}
	if err := qs.DeleteQueueAgents(ctx, qid); err != nil {
		return wrap(err, "clear agents")
	}
	for _, raw := range userIDs {
		uid, err := id(raw)
		if err != nil {
			return err
		}
		if err := qs.InsertQueueAgent(ctx, gen.InsertQueueAgentParams{QueueID: qid, UserID: uid, AddedAt: at.UTC()}); err != nil {
			return wrap(err, "insert agent")
		}
	}
	return nil
}

func (r *Repo) InsertCategory(ctx context.Context, c domain.Category) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertCategoryParams{
		Name: c.Name, IsDefault: c.IsDefault, ArchivedAt: utc(c.ArchivedAt), CreatedAt: c.CreatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, c.ID}, {&p.WorkspaceID, c.WorkspaceID}, {&p.QueueID, c.QueueID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	return conflict(qs.InsertCategory(ctx, p), "insert category", "the workspace already has a default category")
}

func (r *Repo) UpdateCategory(ctx context.Context, c domain.Category) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.UpdateCategoryParams{Name: c.Name, IsDefault: c.IsDefault, ArchivedAt: utc(c.ArchivedAt)}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, c.ID}, {&p.WorkspaceID, c.WorkspaceID}, {&p.QueueID, c.QueueID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	n, err := qs.UpdateCategory(ctx, p)
	if err != nil {
		return conflict(err, "update category", "the workspace already has a default category")
	}
	if n == 0 {
		return fmt.Errorf("category: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) Category(ctx context.Context, workspaceID, categoryID string) (domain.Category, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return domain.Category{}, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Category{}, err
	}
	cid, err := id(categoryID)
	if err != nil {
		return domain.Category{}, err
	}
	row, err := qs.GetCategory(ctx, gen.GetCategoryParams{ID: cid, WorkspaceID: ws})
	if err != nil {
		return domain.Category{}, wrap(err, "category")
	}
	return toCategory(row), nil
}

func (r *Repo) Categories(ctx context.Context, workspaceID string, includeArchived bool) ([]domain.Category, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListCategories(ctx, gen.ListCategoriesParams{WorkspaceID: ws, IncludeArchived: includeArchived})
	if err != nil {
		return nil, wrap(err, "list categories")
	}
	out := make([]domain.Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *Repo) ClearDefault(ctx context.Context, workspaceID, exceptID string) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return err
	}
	except, err := optID(exceptID)
	if err != nil {
		return err
	}
	return wrap(qs.ClearDefaultCategory(ctx, gen.ClearDefaultCategoryParams{WorkspaceID: ws, ExceptID: except}), "clear default")
}

// ticketParams turns a ticket into the insert parameters. The IDs are parsed
// here, so text that is not a UUID is refused before it reaches the database.
func ticketParams(t domain.Ticket) (gen.InsertTicketParams, error) {
	var err error
	p := gen.InsertTicketParams{
		Number: t.Number, Title: t.Title, Description: t.Description, Status: string(t.Status),
		Priority: string(t.Priority), ResolvedAt: utc(t.ResolvedAt), ClosedAt: utc(t.ClosedAt),
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, t.ID}, {&p.WorkspaceID, t.WorkspaceID}, {&p.QueueID, t.QueueID}, {&p.CategoryID, t.CategoryID}, {&p.RequesterID, t.RequesterID}} {
		if *f.dst, err = id(f.src); err != nil {
			return gen.InsertTicketParams{}, err
		}
	}
	for _, f := range []struct {
		dst **uuid.UUID
		src string
	}{
		{&p.AssigneeID, t.AssigneeID}, {&p.SourceChannelID, t.SourceChannelID}, {&p.SourceMessageID, t.SourceMessageID},
		{&p.MessageID, t.MessageID}, {&p.QueueMessageID, t.QueueMessageID},
	} {
		if *f.dst, err = optID(f.src); err != nil {
			return gen.InsertTicketParams{}, err
		}
	}
	return p, nil
}

// NextTicketNumber takes the next number of an active queue (HD-04).
func (r *Repo) NextTicketNumber(ctx context.Context, workspaceID, queueID string) (string, int64, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return "", 0, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return "", 0, err
	}
	qid, err := id(queueID)
	if err != nil {
		return "", 0, err
	}
	row, err := qs.NextTicketNumber(ctx, gen.NextTicketNumberParams{ID: qid, WorkspaceID: ws})
	if err != nil {
		return "", 0, wrap(err, "queue")
	}
	return row.Prefix, row.Number, nil
}

func (r *Repo) InsertTicket(ctx context.Context, t domain.Ticket) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p, err := ticketParams(t)
	if err != nil {
		return err
	}
	return conflict(qs.InsertTicket(ctx, p), "insert ticket", "ticket number is already taken in this queue")
}

func (r *Repo) InsertHistory(ctx context.Context, h domain.HistoryEntry) error {
	qs, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertHistoryParams{
		Action: string(h.Action), FromValue: h.From, ToValue: h.To, Internal: h.Internal, At: h.At.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, h.ID}, {&p.TicketID, h.TicketID}, {&p.ActorID, h.ActorID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	return wrap(qs.InsertHistory(ctx, p), "insert history")
}

// toTicket converts a row of the ticket tables; prefix builds the key.
func toTicket(row gen.HelpdeskTicket, prefix string) domain.Ticket {
	return domain.Ticket{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), QueueID: row.QueueID.String(),
		CategoryID: row.CategoryID.String(), Number: row.Number, Key: domain.TicketKey(prefix, row.Number),
		Title: row.Title, Description: row.Description, RequesterID: row.RequesterID.String(),
		AssigneeID: str(row.AssigneeID), Status: domain.Status(row.Status), Priority: domain.Priority(row.Priority),
		SourceChannelID: str(row.SourceChannelID), SourceMessageID: str(row.SourceMessageID),
		MessageID: str(row.MessageID), QueueMessageID: str(row.QueueMessageID),
		ResolvedAt: utc(row.ResolvedAt), ClosedAt: utc(row.ClosedAt),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func (r *Repo) Ticket(ctx context.Context, workspaceID, ticketID string) (domain.Ticket, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return domain.Ticket{}, err
	}
	ws, err := id(workspaceID)
	if err != nil {
		return domain.Ticket{}, err
	}
	tid, err := id(ticketID)
	if err != nil {
		return domain.Ticket{}, err
	}
	row, err := qs.GetTicket(ctx, gen.GetTicketParams{ID: tid, WorkspaceID: ws})
	if err != nil {
		return domain.Ticket{}, wrap(err, "ticket")
	}
	return toTicket(row.HelpdeskTicket, row.Prefix), nil
}

func (r *Repo) ListTickets(ctx context.Context, f usecase.TicketFilter) ([]domain.Ticket, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	p := gen.ListTicketsParams{IsAdmin: f.Admin, NoAssignee: f.NoAssignee, PageSize: int32(f.Limit), Statuses: []string{}}
	for _, st := range f.Statuses {
		p.Statuses = append(p.Statuses, string(st))
	}
	if p.WorkspaceID, err = id(f.WorkspaceID); err != nil {
		return nil, err
	}
	if p.UserID, err = id(f.UserID); err != nil {
		return nil, err
	}
	for _, opt := range []struct {
		dst **uuid.UUID
		src string
	}{{&p.QueueID, f.QueueID}, {&p.AssigneeID, f.AssigneeID}, {&p.RequesterID, f.RequesterID}, {&p.BeforeID, f.BeforeID}} {
		if *opt.dst, err = optID(opt.src); err != nil {
			return nil, err
		}
	}
	rows, err := qs.ListTickets(ctx, p)
	if err != nil {
		return nil, wrap(err, "list tickets")
	}
	out := make([]domain.Ticket, len(rows))
	for i, row := range rows {
		out[i] = toTicket(row.HelpdeskTicket, row.Prefix)
	}
	return out, nil
}

func (r *Repo) IsAgent(ctx context.Context, workspaceID, queueID, userID string) (bool, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	p := gen.IsQueueAgentParams{}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&p.QueueID, queueID}, {&p.UserID, userID}} {
		if *f.dst, err = id(f.src); err != nil {
			return false, nil // text that is not an ID is not an agent
		}
	}
	ok, err := qs.IsQueueAgent(ctx, p)
	if err != nil {
		return false, wrap(err, "is agent")
	}
	return ok, nil
}

func (r *Repo) History(ctx context.Context, ticketID string) ([]domain.HistoryEntry, error) {
	qs, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	tid, err := id(ticketID)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListHistory(ctx, tid)
	if err != nil {
		return nil, wrap(err, "list history")
	}
	out := make([]domain.HistoryEntry, len(rows))
	for i, row := range rows {
		out[i] = domain.HistoryEntry{
			ID: row.ID.String(), TicketID: row.TicketID.String(), ActorID: row.ActorID.String(),
			Action: domain.Action(row.Action), From: row.FromValue, To: row.ToValue, Internal: row.Internal, At: row.At.UTC(),
		}
	}
	return out, nil
}
