package httpadapter

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// registerInvites adds the invite endpoints. Register calls it.
func (h *Handler) registerInvites(api huma.API, user, member huma.Middlewares) {
	huma.Register(api, huma.Operation{
		OperationID: "workspace-create-invite", Method: http.MethodPost, Path: "/invites", DefaultStatus: http.StatusCreated,
		Summary:     "Create an invite; the token is returned only in this response (owner or admin)",
		Description: "An invite with an email can only be accepted by the user with that email. It expires after 7 days and cannot grant owner.",
		Tags:        []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.createInvite)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list-invites", Method: http.MethodGet, Path: "/invites",
		Summary: "List pending invites, without tokens (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.listInvites)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-revoke-invite", Method: http.MethodDelete, Path: "/invites/{id}", DefaultStatus: http.StatusNoContent,
		Summary: "Revoke a pending invite (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.revokeInvite)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-accept-invite", Method: http.MethodPost, Path: "/invites/accept",
		Summary:     "Join a workspace with an invite token",
		Description: "404 when the token is unknown, expired, revoked or used; 403 when the invite is for another email; 409 when already a member.",
		Tags:        []string{tag}, Security: httpx.BearerAuth, Middlewares: user,
	}, h.acceptInvite)
}

// Invite is a workspace invite. Token is set only in the create response.
type Invite struct {
	ID        string    `json:"id"`
	Email     *string   `json:"email" nullable:"true" doc:"Null for a link invite"`
	Role      app.Role  `json:"role" enum:"admin,member,guest"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	Token     string    `json:"token,omitempty" doc:"Shown once, when the invite is created"`
}

func toInvite(i domain.Invite, token string) Invite {
	out := Invite{ID: i.ID, Role: i.Role, ExpiresAt: i.ExpiresAt.UTC(), CreatedAt: i.CreatedAt.UTC(), Token: token}
	if i.Email != "" {
		out.Email = &i.Email
	}
	return out
}

type createInviteInput struct {
	Body struct {
		Email string   `json:"email,omitempty" maxLength:"254" doc:"Leave out for a link invite anyone can use once"`
		Role  app.Role `json:"role" enum:"admin,member,guest"`
	}
}

type inviteOutput struct{ Body Invite }

func (h *Handler) createInvite(ctx context.Context, in *createInviteInput) (*inviteOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	inv, err := h.uc.CreateInvite(ctx, p, in.Body.Email, in.Body.Role)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &inviteOutput{Body: toInvite(inv.Invite, inv.Token)}, nil
}

type listInvitesOutput struct {
	Body struct {
		Items []Invite `json:"items" nullable:"false"`
	}
}

func (h *Handler) listInvites(ctx context.Context, _ *struct{}) (*listInvitesOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	invs, err := h.uc.ListInvites(ctx, p)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listInvitesOutput{}
	out.Body.Items = make([]Invite, len(invs))
	for i, inv := range invs {
		out.Body.Items[i] = toInvite(inv, "")
	}
	return out, nil
}

type revokeInviteInput struct {
	ID string `path:"id"`
}

func (h *Handler) revokeInvite(ctx context.Context, in *revokeInviteInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.RevokeInvite(ctx, p, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

type acceptInviteInput struct {
	Body struct {
		Token string `json:"token" maxLength:"256"`
	}
}

func (h *Handler) acceptInvite(ctx context.Context, in *acceptInviteInput) (*workspaceOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	var w usecase.WorkspaceView
	if w, err = h.uc.AcceptInvite(ctx, p.UserID, in.Body.Token); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &workspaceOutput{Body: toWorkspace(w)}, nil
}
