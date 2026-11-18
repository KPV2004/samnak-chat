// Package httpadapter maps the helpdesk use cases to Huma operations.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "helpdesk"

// Handler serves the helpdesk endpoints.
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

// Register adds the endpoints to api, a group prefixed with "/helpdesk". Every
// operation requires a workspace member; the admin operations and the rules
// of each ticket are checked in the use cases.
func (h *Handler) Register(api huma.API, services *app.Services) {
	member := huma.Middlewares{httpx.RequireMember(api, services)}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
			Tags: []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
		}
	}
	huma.Register(api, op("helpdesk-list-queues", http.MethodGet, "/queues", "List the active queues with their agents and categories (every member)", 0), h.listQueues)
	huma.Register(api, op("helpdesk-create-queue", http.MethodPost, "/queues", "Create a queue; admins only", http.StatusCreated), h.createQueue)
	huma.Register(api, op("helpdesk-update-queue", http.MethodPatch, "/queues/{id}", "Rename, set or clear the channel, or archive a queue; admins only", 0), h.updateQueue)
	huma.Register(api, op("helpdesk-set-queue-agents", http.MethodPut, "/queues/{id}/agents", "Replace the agents of a queue; admins only; each must be an active member", http.StatusNoContent), h.setAgents)
	huma.Register(api, op("helpdesk-create-category", http.MethodPost, "/categories", "Create a category in an active queue; admins only", http.StatusCreated), h.createCategory)
	huma.Register(api, op("helpdesk-create-ticket", http.MethodPost, "/tickets", "Create a ticket in the queue of a category; with source_message_id the message is read as you (HD-03)", http.StatusCreated), h.createTicket)
	huma.Register(api, op("helpdesk-update-category", http.MethodPatch, "/categories/{id}", "Rename, move, make default or archive a category; admins only", 0), h.updateCategory)
}

func principal(ctx context.Context) (app.Principal, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.Principal{}, app.ErrUnauthorized
	}
	return p, nil
}

// fail maps contract errors to their status. Anything else is logged and
// answered as a plain 500 so internals never reach the client.
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
		h.log.ErrorContext(ctx, "helpdesk: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}

// nullable is a field that can be left out, set, or set to null, for PATCH
// bodies where null clears a value.
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

// emptyOutput is the answer of an operation with no body (204).
type emptyOutput struct{}

type helpdeskQueuesOutput struct {
	Body struct {
		Items []wire.HelpdeskQueue `json:"items" nullable:"false"`
	}
}

func (h *Handler) listQueues(ctx context.Context, _ *struct{}) (*helpdeskQueuesOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	list, err := h.uc.ListQueues(ctx, p)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &helpdeskQueuesOutput{}
	out.Body.Items = wire.ToQueues(list)
	return out, nil
}

type helpdeskQueueCreateInput struct {
	Body struct {
		Name      string `json:"name" doc:"1 to 100 characters"`
		Prefix    string `json:"prefix" doc:"2 to 6 letters A to Z, unique in the workspace; stored upper case"`
		ChannelID string `json:"channel_id,omitempty" doc:"The agents' channel; new tickets of this queue get a card there"`
	}
}

type helpdeskQueueOutput struct {
	Body wire.HelpdeskQueue
}

func (h *Handler) createQueue(ctx context.Context, in *helpdeskQueueCreateInput) (*helpdeskQueueOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	q, err := h.uc.CreateQueue(ctx, p, domain.QueueInput{Name: in.Body.Name, Prefix: in.Body.Prefix, ChannelID: in.Body.ChannelID})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &helpdeskQueueOutput{Body: wire.ToQueue(q, nil)}, nil
}

type helpdeskQueueUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name      *string          `json:"name,omitempty" doc:"1 to 100 characters"`
		ChannelID nullable[string] `json:"channel_id,omitempty" doc:"null clears the channel"`
		Archived  *bool            `json:"archived,omitempty" doc:"true hides the queue and its categories; false restores it"`
	}
}

func (h *Handler) updateQueue(ctx context.Context, in *helpdeskQueueUpdateInput) (*helpdeskQueueOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	patch := domain.QueuePatch{Name: in.Body.Name, Archived: in.Body.Archived}
	if in.Body.ChannelID.set {
		patch.ChannelSet = true
		if !in.Body.ChannelID.null {
			patch.ChannelID = in.Body.ChannelID.value
		}
	}
	q, err := h.uc.UpdateQueue(ctx, p, in.ID, patch)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &helpdeskQueueOutput{Body: wire.ToQueue(q, nil)}, nil
}

type helpdeskQueueAgentsInput struct {
	ID   string `path:"id"`
	Body struct {
		UserIDs []string `json:"user_ids" nullable:"false" doc:"The new agents; an empty list removes them all"`
	}
}

func (h *Handler) setAgents(ctx context.Context, in *helpdeskQueueAgentsInput) (*emptyOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.SetAgents(ctx, p, in.ID, in.Body.UserIDs); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &emptyOutput{}, nil
}

type helpdeskCategoryCreateInput struct {
	Body struct {
		QueueID   string `json:"queue_id"`
		Name      string `json:"name" doc:"1 to 100 characters"`
		IsDefault bool   `json:"is_default,omitempty" doc:"Takes the default from the category that had it"`
	}
}

type helpdeskCategoryOutput struct {
	Body wire.HelpdeskCategory
}

func (h *Handler) createCategory(ctx context.Context, in *helpdeskCategoryCreateInput) (*helpdeskCategoryOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	c, err := h.uc.CreateCategory(ctx, p, domain.CategoryInput{QueueID: in.Body.QueueID, Name: in.Body.Name, IsDefault: in.Body.IsDefault})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &helpdeskCategoryOutput{Body: wire.ToCategory(c)}, nil
}

type helpdeskCategoryUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name      *string `json:"name,omitempty" doc:"1 to 100 characters"`
		QueueID   *string `json:"queue_id,omitempty" doc:"Move the category to another active queue"`
		IsDefault *bool   `json:"is_default,omitempty"`
		Archived  *bool   `json:"archived,omitempty"`
	}
}

func (h *Handler) updateCategory(ctx context.Context, in *helpdeskCategoryUpdateInput) (*helpdeskCategoryOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	patch := domain.CategoryPatch{
		Name: in.Body.Name, QueueID: in.Body.QueueID, IsDefault: in.Body.IsDefault, Archived: in.Body.Archived,
	}
	c, err := h.uc.UpdateCategory(ctx, p, in.ID, patch)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &helpdeskCategoryOutput{Body: wire.ToCategory(c)}, nil
}

type helpdeskTicketCreateInput struct {
	Body struct {
		CategoryID      string `json:"category_id" doc:"The category; its queue takes the ticket"`
		Title           string `json:"title" doc:"1 to 200 characters"`
		Description     string `json:"description,omitempty" doc:"Up to 8000 characters; with source_message_id and no description, the message body"`
		Priority        string `json:"priority,omitempty" enum:"low,normal,high,urgent" doc:"Defaults to normal"`
		SourceMessageID string `json:"source_message_id,omitempty" doc:"A chat message you can read; needs chat"`
	}
}

type helpdeskTicketOutput struct {
	Body wire.HelpdeskTicket
}

func (h *Handler) createTicket(ctx context.Context, in *helpdeskTicketCreateInput) (*helpdeskTicketOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	t, err := h.uc.Create(ctx, p, usecase.CreateInput{
		CategoryID: in.Body.CategoryID, Title: in.Body.Title, Description: in.Body.Description,
		Priority: domain.Priority(in.Body.Priority), SourceMessageID: in.Body.SourceMessageID,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &helpdeskTicketOutput{Body: wire.ToTicket(t)}, nil
}
