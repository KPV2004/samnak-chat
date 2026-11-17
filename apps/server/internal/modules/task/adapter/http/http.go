// Package httpadapter maps the task use cases to Huma operations.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "task"

// Handler serves the task endpoints.
type Handler struct {
	uc  *usecase.Service
	log *slog.Logger
}

func New(uc *usecase.Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{uc: uc, log: log}
}

// Register adds the endpoints to api, a group prefixed with "/task". Every
// operation requires a workspace member; the use cases scope the answer to the
// X-Workspace-ID workspace and to the channels the caller can read.
func (h *Handler) Register(api huma.API, services *app.Services) {
	member := huma.Middlewares{httpx.RequireMember(api, services)}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
			Tags: []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
		}
	}
	huma.Register(api, op("task-create-item", http.MethodPost, "/items", "Create a task in a channel", http.StatusCreated), h.create)
	huma.Register(api, op("task-list-items", http.MethodGet, "/items", "List the tasks of channels the caller can read", 0), h.list)
	huma.Register(api, op("task-get-item", http.MethodGet, "/items/{id}", "Get one task", 0), h.get)
	huma.Register(api, op("task-update-item", http.MethodPatch, "/items/{id}", "Edit a task: creator, assignee, owner or admin", 0), h.update)
}

func principal(ctx context.Context) (app.Principal, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.Principal{}, app.ErrUnauthorized
	}
	return p, nil
}

// fail maps contract errors to their status. A client that hung up is not a
// server fault and is not logged. Anything else is logged and answered as a
// plain 500 so internals never reach the client.
func (h *Handler) fail(ctx context.Context, err error) error {
	var status int
	switch {
	case errors.Is(err, app.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, app.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, app.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, app.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrConflict):
		status = http.StatusConflict
	case httpx.ClientAborted(ctx, err):
		return huma.NewError(httpx.StatusClientClosedRequest, "client closed request")
	default:
		h.log.ErrorContext(ctx, "task: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}

// CreateTaskRequest is the body of POST /task/items.
type CreateTaskRequest struct {
	ChannelID       string     `json:"channel_id" doc:"The channel the task belongs to; the caller must be able to read it"`
	Title           string     `json:"title" minLength:"1" maxLength:"200"`
	Description     string     `json:"description,omitempty" maxLength:"4000"`
	AssigneeID      string     `json:"assignee_id,omitempty" doc:"An active member who can read the channel"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	Priority        string     `json:"priority,omitempty" enum:"low,normal,high" doc:"Defaults to normal"`
	SourceMessageID string     `json:"source_message_id,omitempty" doc:"The message the task was created from"`
}

type createInput struct {
	Body CreateTaskRequest
}

type taskOutput struct {
	Body wire.Task
}

func (h *Handler) create(ctx context.Context, in *createInput) (*taskOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	b := in.Body
	t, err := h.uc.Create(ctx, p, usecase.CreateInput{
		ChannelID: b.ChannelID, Title: b.Title, Description: b.Description, AssigneeID: b.AssigneeID, DueAt: b.DueAt,
		Priority: domain.Priority(b.Priority), SourceMessageID: b.SourceMessageID,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &taskOutput{Body: wire.ToTask(t)}, nil
}

type listInput struct {
	Assignee  string `query:"assignee" enum:"me,any" doc:"me keeps tasks assigned to the caller; any (the default) does not filter"`
	Creator   string `query:"creator" enum:"me" doc:"me keeps tasks the caller created"`
	Status    string `query:"status" doc:"Comma-separated statuses: todo, doing, done"`
	ChannelID string `query:"channel_id"`
	Due       string `query:"due" enum:"overdue,today,week" doc:"overdue: past due and not done. today: due this UTC day. week: due from the start of this UTC day for seven days"`
	Cursor    string `query:"cursor" doc:"next_cursor of the previous page"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Page size"`
}

// TaskPage is one page of tasks, newest first.
type TaskPage struct {
	Items      []wire.Task `json:"items" nullable:"false"`
	NextCursor *string     `json:"next_cursor" doc:"Pass it as cursor for the next page; null on the last page"`
}

type listOutput struct {
	Body TaskPage
}

func (h *Handler) list(ctx context.Context, in *listInput) (*listOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	q := usecase.ListQuery{
		Assignee: in.Assignee, Creator: in.Creator, ChannelID: in.ChannelID, Due: in.Due, Cursor: in.Cursor, Limit: in.Limit,
	}
	for _, s := range strings.Split(in.Status, ",") {
		if s = strings.TrimSpace(s); s != "" {
			q.Statuses = append(q.Statuses, s)
		}
	}
	page, err := h.uc.List(ctx, p, q)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listOutput{Body: TaskPage{Items: wire.ToTasks(page.Items)}}
	if page.NextCursor != "" {
		out.Body.NextCursor = &page.NextCursor
	}
	return out, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (h *Handler) get(ctx context.Context, in *idInput) (*taskOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	t, err := h.uc.Get(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &taskOutput{Body: wire.ToTask(t)}, nil
}

// nullable is a JSON field that can be absent (leave alone), null (clear) or
// a value (set).
type nullable[T any] struct {
	set, null bool
	value     T
}

func (n *nullable[T]) UnmarshalJSON(b []byte) error {
	n.set = true
	if string(b) == "null" {
		n.null = true
		return nil
	}
	return json.Unmarshal(b, &n.value)
}

// Schema describes the field as the value type or null.
func (nullable[T]) Schema(r huma.Registry) *huma.Schema {
	s := *r.Schema(reflect.TypeFor[T](), false, "")
	s.Nullable = true
	return &s
}

// UpdateTaskRequest is the body of PATCH /task/items/{id}. A field that is
// left out stays as it is; assignee_id and due_at can be null to clear them.
type UpdateTaskRequest struct {
	Title       *string             `json:"title,omitempty" minLength:"1" maxLength:"200"`
	Description *string             `json:"description,omitempty" maxLength:"4000"`
	AssigneeID  nullable[string]    `json:"assignee_id,omitempty" doc:"An active member who can read the channel; null clears it"`
	DueAt       nullable[time.Time] `json:"due_at,omitempty" doc:"null clears it; a new date is announced again"`
	Priority    *string             `json:"priority,omitempty" enum:"low,normal,high"`
	Status      *string             `json:"status,omitempty" enum:"todo,doing,done"`
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateTaskRequest
}

func (h *Handler) update(ctx context.Context, in *updateInput) (*taskOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	b := in.Body
	patch := domain.Patch{Title: b.Title, Description: b.Description}
	if b.Priority != nil {
		pr := domain.Priority(*b.Priority)
		patch.Priority = &pr
	}
	if b.Status != nil {
		st := domain.Status(*b.Status)
		patch.Status = &st
	}
	if b.AssigneeID.set {
		patch.AssigneeSet = true
		if !b.AssigneeID.null {
			patch.AssigneeID = b.AssigneeID.value
		}
	}
	if b.DueAt.set {
		patch.DueSet = true
		if !b.DueAt.null {
			due := b.DueAt.value
			patch.DueAt = &due
		}
	}
	t, err := h.uc.Update(ctx, p, in.ID, patch)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &taskOutput{Body: wire.ToTask(t)}, nil
}
