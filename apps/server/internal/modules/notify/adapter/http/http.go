// Package httpadapter maps the notify use cases to Huma operations.
package httpadapter

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "notify"

// Handler serves the notify endpoints.
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

// Register adds the endpoints to api, a group prefixed with "/notify". Every
// operation requires a workspace member, and the use cases scope the answer to
// the caller and the X-Workspace-ID workspace.
func (h *Handler) Register(api huma.API, services *app.Services) {
	member := huma.Middlewares{httpx.RequireMember(api, services)}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
			Tags: []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
		}
	}
	huma.Register(api, op("notify-list-notifications", http.MethodGet, "/notifications", "List my notifications, newest first, unread or all", 0), h.list)
	huma.Register(api, op("notify-unread-count", http.MethodGet, "/unread-count", "The number of unread notifications in this workspace", 0), h.unreadCount)
	huma.Register(api, op("notify-mark-read", http.MethodPost, "/notifications/{id}/read", "Mark one notification read; repeating it changes nothing", http.StatusNoContent), h.markRead)
	huma.Register(api, op("notify-read-all", http.MethodPost, "/read-all", "Mark every notification in this workspace read", http.StatusNoContent), h.readAll)
	huma.Register(api, op("notify-get-preferences", http.MethodGet, "/preferences", "My notification settings in this workspace; defaults when never saved", 0), h.getPreferences)
	huma.Register(api, op("notify-set-preferences", http.MethodPut, "/preferences", "Replace my notification settings in this workspace", 0), h.setPreferences)
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
		h.log.ErrorContext(ctx, "notify: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}

type listInput struct {
	Unread bool   `query:"unread" default:"false" doc:"Only unread notifications"`
	Before string `query:"before" maxLength:"36" doc:"Cursor: the ID of the last notification of the previous page"`
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"50" doc:"Page size"`
}

type notificationsOutput struct {
	Body struct {
		Items   []wire.Notification `json:"items" nullable:"false"`
		HasMore bool                `json:"has_more" doc:"There are older notifications behind the cursor"`
	}
}

func (h *Handler) list(ctx context.Context, in *listInput) (*notificationsOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	page, err := h.uc.List(ctx, p, usecase.ListQuery{UnreadOnly: in.Unread, BeforeID: in.Before, Limit: in.Limit})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &notificationsOutput{}
	out.Body.Items = wire.ToNotifications(page.Items)
	out.Body.HasMore = page.HasMore
	return out, nil
}

type unreadCountOutput struct {
	Body struct {
		Count int64 `json:"count"`
	}
}

func (h *Handler) unreadCount(ctx context.Context, _ *struct{}) (*unreadCountOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	n, err := h.uc.UnreadCount(ctx, p)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &unreadCountOutput{}
	out.Body.Count = n
	return out, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (h *Handler) markRead(ctx context.Context, in *idInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.MarkRead(ctx, p, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

func (h *Handler) readAll(ctx context.Context, _ *struct{}) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.ReadAll(ctx, p); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

type preferencesOutput struct {
	Body wire.Preferences
}

func (h *Handler) getPreferences(ctx context.Context, _ *struct{}) (*preferencesOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	pref, err := h.uc.GetPreferences(ctx, p)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &preferencesOutput{Body: wire.ToPreferences(pref)}, nil
}

type setPreferencesInput struct {
	Body wire.Preferences
}

func (h *Handler) setPreferences(ctx context.Context, in *setPreferencesInput) (*preferencesOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	pref, err := h.uc.SetPreferences(ctx, p, in.Body.ToDomain())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &preferencesOutput{Body: wire.ToPreferences(pref)}, nil
}
