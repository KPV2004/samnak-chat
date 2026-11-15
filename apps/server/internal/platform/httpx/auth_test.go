package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type fakeAuth map[string]app.Identity

func (f fakeAuth) Authenticate(_ context.Context, tok string) (app.Identity, error) {
	if id, ok := f[tok]; ok {
		return id, nil
	}
	return app.Identity{}, fmt.Errorf("token: %w", app.ErrUnauthorized)
}

type fakeMembers map[string]app.Role // key: workspaceID/userID

func (f fakeMembers) Role(_ context.Context, ws, user string) (app.Role, error) {
	if r, ok := f[ws+"/"+user]; ok {
		return r, nil
	}
	return "", fmt.Errorf("member: %w", app.ErrForbidden)
}

type whoOutput struct {
	Body app.Principal
}

// newAuthAPI serves GET /user (RequireUser) and GET /member (RequireMember)
// that echo the Principal from the context.
func newAuthAPI(s *app.Services) http.Handler {
	r := chi.NewRouter()
	api := NewAPI(r, "test")
	echo := func(ctx context.Context, _ *struct{}) (*whoOutput, error) {
		p, ok := app.PrincipalFrom(ctx)
		if !ok {
			return nil, huma.Error500InternalServerError("no principal")
		}
		return &whoOutput{Body: p}, nil
	}
	huma.Register(api, huma.Operation{OperationID: "user", Method: http.MethodGet, Path: "/user",
		Security: BearerAuth, Middlewares: huma.Middlewares{RequireUser(api, s)}}, echo)
	huma.Register(api, huma.Operation{OperationID: "member", Method: http.MethodGet, Path: "/member",
		Security: BearerAuth, Middlewares: huma.Middlewares{RequireMember(api, s)}}, echo)
	return r
}

func TestAuthMiddleware(t *testing.T) {
	s := app.NewServices()
	app.Provide[app.Authenticator](s, fakeAuth{"good": {UserID: "u1", DeviceID: "d1"}})
	app.Provide[app.MembershipResolver](s, fakeMembers{"w1/u1": app.RoleAdmin})
	h := newAuthAPI(s)

	cases := []struct {
		name, path, auth, ws string
		wantCode             int
		wantBody             string
	}{
		{"user ok", "/user", "Bearer good", "", 200, `"UserID":"u1"`},
		{"no token", "/user", "", "", 401, `"code":"unauthorized"`},
		{"not bearer", "/user", "Basic good", "", 401, `"code":"unauthorized"`},
		{"bad token", "/user", "Bearer nope", "", 401, `"code":"unauthorized"`},
		{"member ok", "/member", "Bearer good", "w1", 200, `"Role":"admin"`},
		{"member ws in principal", "/member", "Bearer good", "w1", 200, `"WorkspaceID":"w1"`},
		{"member no header", "/member", "Bearer good", "", 400, `"code":"invalid"`},
		{"member other ws", "/member", "Bearer good", "w2", 403, `"code":"forbidden"`},
		{"member bad token", "/member", "Bearer nope", "w1", 401, `"code":"unauthorized"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			if tc.ws != "" {
				req.Header.Set(WorkspaceHeader, tc.ws)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %s, want %d containing %s", rec.Code, rec.Body, tc.wantCode, tc.wantBody)
			}
		})
	}
}

func TestAuthMiddlewareWithoutProviders(t *testing.T) {
	h := newAuthAPI(app.NewServices())
	for path, want := range map[string]int{"/user": 401, "/member": 401} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set(WorkspaceHeader, "w1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestAuthMiddlewareHidesUnexpectedErrors(t *testing.T) {
	s := app.NewServices()
	app.Provide[app.Authenticator](s, errAuth{})
	h := newAuthAPI(s)
	req := httptest.NewRequest(http.MethodGet, "/user", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "db exploded") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

type errAuth struct{}

func (errAuth) Authenticate(context.Context, string) (app.Identity, error) {
	return app.Identity{}, fmt.Errorf("db exploded")
}
