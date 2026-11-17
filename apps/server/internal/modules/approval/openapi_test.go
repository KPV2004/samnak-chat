package approval_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// The openapi command starts modules with no database, no Tx and no Bus; every
// operation must still be registered, under the /approval prefix.
func TestOperationsAppearInOpenAPIWithoutDatabase(t *testing.T) {
	reg := app.NewRegistry(approval.New())
	mods, err := reg.Enable([]string{"approval"})
	if err != nil {
		t.Fatal(err)
	}
	api := httpx.NewAPI(chi.NewRouter(), "test")
	deps := &app.Deps{Logger: slog.New(slog.DiscardHandler), Services: app.NewServices(), Notifications: app.NoopNotificationRegistry{}}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"/approval/requests":              {"POST", "GET"},
		"/approval/requests/{id}":         {"GET"},
		"/approval/requests/{id}/approve": {"POST"},
		"/approval/requests/{id}/reject":  {"POST"},
		"/approval/requests/{id}/cancel":  {"POST"},
		"/approval/uploads":               {"POST"},
		"/approval/attachments/{id}":      {"GET"},
	}
	paths := api.OpenAPI().Paths
	for path, methods := range want {
		item := paths[path]
		if item == nil {
			t.Errorf("path %s missing from OpenAPI", path)
			continue
		}
		ops := map[string]bool{"POST": item.Post != nil, "GET": item.Get != nil}
		for _, m := range methods {
			if !ops[m] {
				t.Errorf("%s %s missing", m, path)
			}
		}
	}
	// Huma names a schema after its Go type, and names are shared by every
	// module's operations, so each of ours carries the module prefix.
	// ApiError and ApiErrorBody come from httpx and are shared on purpose.
	for name := range api.OpenAPI().Components.Schemas.Map() {
		if name == "ApiError" || name == "ApiErrorBody" || strings.HasPrefix(name, "Approval") {
			continue
		}
		t.Errorf("schema %q may collide with another module's: its Go type needs the approval prefix", name)
	}
}
