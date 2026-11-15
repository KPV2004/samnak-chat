package httpx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// WorkspaceHeader names the workspace a request acts in (SRS 14.1).
const WorkspaceHeader = "X-Workspace-ID"

// BearerAuth is the OpenAPI security requirement for authenticated operations.
var BearerAuth = []map[string][]string{{"bearer": {}}}

// RequireUser is Huma middleware for endpoints that need a logged-in user but
// no workspace, such as GET /auth/me. It puts an app.Principal with an empty
// WorkspaceID into the context. It answers 401 when no Authenticator is
// provided or the token is rejected.
func RequireUser(api huma.API, s *app.Services) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id, ok := authenticate(api, s, ctx)
		if !ok {
			return
		}
		next(huma.WithContext(ctx, app.WithPrincipal(ctx.Context(), app.Principal{UserID: id.UserID, DeviceID: id.DeviceID})))
	}
}

// RequireMember is Huma middleware for workspace-scoped endpoints. It
// authenticates the user, reads X-Workspace-ID, checks membership and puts the
// full app.Principal into the context. Missing header → 400, not a member →
// 403. Use cases read the workspace only from that Principal.
func RequireMember(api huma.API, s *app.Services) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id, ok := authenticate(api, s, ctx)
		if !ok {
			return
		}
		wsID := strings.TrimSpace(ctx.Header(WorkspaceHeader))
		if wsID == "" {
			_ = huma.WriteErr(api, ctx, http.StatusBadRequest, WorkspaceHeader+" header is required")
			return
		}
		members, ok := app.Lookup[app.MembershipResolver](s)
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "workspaces are not available")
			return
		}
		role, err := members.Role(ctx.Context(), wsID, id.UserID)
		if err != nil {
			writeAuthErr(api, ctx, err)
			return
		}
		p := app.Principal{UserID: id.UserID, DeviceID: id.DeviceID, WorkspaceID: wsID, Role: role}
		next(huma.WithContext(ctx, app.WithPrincipal(ctx.Context(), p)))
	}
}

func authenticate(api huma.API, s *app.Services, ctx huma.Context) (app.Identity, bool) {
	token, found := strings.CutPrefix(ctx.Header("Authorization"), "Bearer ")
	if !found || strings.TrimSpace(token) == "" {
		_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing bearer token")
		return app.Identity{}, false
	}
	auth, ok := app.Lookup[app.Authenticator](s)
	if !ok {
		_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication is not available")
		return app.Identity{}, false
	}
	id, err := auth.Authenticate(ctx.Context(), strings.TrimSpace(token))
	if err != nil {
		writeAuthErr(api, ctx, err)
		return app.Identity{}, false
	}
	return id, true
}

// writeAuthErr maps contract errors to their status and hides anything else.
func writeAuthErr(api huma.API, ctx huma.Context, err error) {
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token")
	case errors.Is(err, app.ErrForbidden):
		_ = huma.WriteErr(api, ctx, http.StatusForbidden, "not a member of this workspace")
	default:
		_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "internal error")
	}
}
