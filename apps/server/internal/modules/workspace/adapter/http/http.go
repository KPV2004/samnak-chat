// Package httpadapter maps the workspace use cases to Huma operations.
package httpadapter

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "workspace"

// Handler serves the workspace endpoints.
type Handler struct {
	uc  *usecase.Service
	log *slog.Logger
}

func New(uc *usecase.Service, log *slog.Logger) *Handler { return &Handler{uc: uc, log: log} }

// Register adds the endpoints to api, a group prefixed with "/workspace".
// Paths are relative to that prefix.
func (h *Handler) Register(api huma.API, services *app.Services) {
	user := huma.Middlewares{httpx.RequireUser(api, services)}
	member := huma.Middlewares{httpx.RequireMember(api, services)}

	huma.Register(api, huma.Operation{
		OperationID: "workspace-create", Method: http.MethodPost, Path: "", DefaultStatus: http.StatusCreated,
		Summary: "Create a workspace; the caller becomes its owner",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: user,
	}, h.create)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list", Method: http.MethodGet, Path: "",
		Summary: "List my workspaces with my role",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: user,
	}, h.list)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list-members", Method: http.MethodGet, Path: "/members",
		Summary: "List the members of the workspace in X-Workspace-ID",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.listMembers)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-update-member", Method: http.MethodPatch, Path: "/members/{userId}",
		Summary: "Change a member's role or status (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.updateMember)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-remove-member", Method: http.MethodDelete, Path: "/members/{userId}", DefaultStatus: http.StatusNoContent,
		Summary: "Remove a member (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.removeMember)
	h.registerInvites(api, user, member)
	h.registerGroups(api, member)
}

// Workspace is a workspace with the caller's role.
type Workspace struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Slug string   `json:"slug"`
	Role app.Role `json:"role" enum:"owner,admin,manager,member,guest"`
}

// Member is a workspace member.
type Member struct {
	UserID      string        `json:"user_id"`
	DisplayName string        `json:"display_name" doc:"Empty when the auth module is unavailable"`
	Email       string        `json:"email" doc:"Empty when the auth module is unavailable"`
	Role        app.Role      `json:"role" enum:"owner,admin,manager,member,guest"`
	Status      domain.Status `json:"status" enum:"active,deactivated"`
	JoinedAt    time.Time     `json:"joined_at"`
}

func toWorkspace(v usecase.WorkspaceView) Workspace {
	return Workspace{ID: v.ID, Name: v.Name, Slug: v.Slug, Role: v.Role}
}

func toMember(v usecase.MemberView) Member {
	return Member{UserID: v.UserID, DisplayName: v.DisplayName, Email: v.Email, Role: v.Role, Status: v.Status, JoinedAt: v.JoinedAt.UTC()}
}

type workspaceOutput struct{ Body Workspace }

type createInput struct {
	Body struct {
		Name string `json:"name" doc:"Display name, 1 to 100 characters"`
		Slug string `json:"slug" doc:"Unique, 3 to 40 characters of a-z, 0-9 and hyphen" example:"acme-team"`
	}
}

func (h *Handler) create(ctx context.Context, in *createInput) (*workspaceOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	w, err := h.uc.CreateWorkspace(ctx, p.UserID, in.Body.Name, in.Body.Slug)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &workspaceOutput{Body: toWorkspace(w)}, nil
}

type listOutput struct {
	Body struct {
		Items []Workspace `json:"items" nullable:"false"`
	}
}

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	ws, err := h.uc.ListWorkspaces(ctx, p.UserID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listOutput{}
	out.Body.Items = make([]Workspace, len(ws))
	for i, w := range ws {
		out.Body.Items[i] = toWorkspace(w)
	}
	return out, nil
}

type listMembersInput struct {
	Cursor string `query:"cursor" doc:"Opaque cursor from the previous page's next_cursor"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"100"`
}

type listMembersOutput struct {
	Body struct {
		Items      []Member `json:"items" nullable:"false"`
		NextCursor string   `json:"next_cursor" doc:"Empty on the last page"`
	}
}

func (h *Handler) listMembers(ctx context.Context, in *listMembersInput) (*listMembersOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	page, err := h.uc.ListMembers(ctx, p, in.Cursor, in.Limit)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listMembersOutput{}
	out.Body.NextCursor = page.NextCursor
	out.Body.Items = make([]Member, len(page.Items))
	for i, m := range page.Items {
		out.Body.Items[i] = toMember(m)
	}
	return out, nil
}

type updateMemberInput struct {
	UserID string `path:"userId"`
	Body   struct {
		Role   *app.Role      `json:"role,omitempty" enum:"owner,admin,manager,member,guest" doc:"Only an owner may grant or change owner"`
		Status *domain.Status `json:"status,omitempty" enum:"active,deactivated"`
	}
}

type memberOutput struct{ Body Member }

func (h *Handler) updateMember(ctx context.Context, in *updateMemberInput) (*memberOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	m, err := h.uc.UpdateMember(ctx, p, in.UserID, in.Body.Role, in.Body.Status)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &memberOutput{Body: toMember(m)}, nil
}

type removeMemberInput struct {
	UserID string `path:"userId"`
}

func (h *Handler) removeMember(ctx context.Context, in *removeMemberInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.RemoveMember(ctx, p, in.UserID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

// principal reads the caller put there by httpx.RequireUser or RequireMember.
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
	default:
		h.log.ErrorContext(ctx, "workspace: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}
