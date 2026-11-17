package workspace_test

import (
	"log/slog"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// The openapi command starts modules with no database and no Tx; every
// operation must still be registered, under the /workspace prefix.
func TestOperationsAppearInOpenAPIWithoutDatabase(t *testing.T) {
	mod := workspace.New()
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"workspace"})
	if err != nil {
		t.Fatal(err)
	}
	api := httpx.NewAPI(chi.NewRouter(), "test")
	deps := &app.Deps{Logger: slog.New(slog.DiscardHandler), Services: app.NewServices(), Notifications: app.NoopNotificationRegistry{}}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"/workspace":                     {"POST", "GET"},
		"/workspace/members":             {"GET"},
		"/workspace/members/{userId}":    {"PATCH", "DELETE"},
		"/workspace/invites":             {"POST", "GET"},
		"/workspace/invites/{id}":        {"DELETE"},
		"/workspace/invites/accept":      {"POST"},
		"/workspace/groups":              {"GET", "POST"},
		"/workspace/groups/{id}":         {"PATCH", "DELETE"},
		"/workspace/groups/{id}/members": {"GET", "PUT"},
	}
	paths := api.OpenAPI().Paths
	for path, methods := range want {
		item := paths[path]
		if item == nil {
			t.Errorf("path %s missing from OpenAPI", path)
			continue
		}
		ops := map[string]bool{"POST": item.Post != nil, "GET": item.Get != nil, "PATCH": item.Patch != nil, "PUT": item.Put != nil, "DELETE": item.Delete != nil}
		for _, m := range methods {
			if !ops[m] {
				t.Errorf("%s %s missing", m, path)
			}
		}
	}
}
