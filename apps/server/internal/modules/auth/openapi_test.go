package auth_test

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// `server openapi` starts modules with no database and no secret.
func TestRoutesAppearInOpenAPIWithoutADatabase(t *testing.T) {
	api := httpx.NewAPI(chi.NewRouter(), "test")
	reg := app.NewRegistry(auth.New())
	mods, err := reg.Enable([]string{"auth"})
	if err != nil {
		t.Fatal(err)
	}
	deps := &app.Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Services: app.NewServices()}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatalf("Start without a database: %v", err)
	}

	// path -> method -> needs a bearer token
	want := map[string]map[string]bool{
		"/auth/register":     {"post": false},
		"/auth/login":        {"post": false},
		"/auth/refresh":      {"post": false},
		"/auth/logout":       {"post": true},
		"/auth/me":           {"get": true, "patch": true},
		"/auth/devices":      {"get": true},
		"/auth/devices/{id}": {"delete": true},
	}
	spec := api.OpenAPI()
	for path, methods := range want {
		item := spec.Paths[path]
		if item == nil {
			t.Errorf("path %s is missing from the OpenAPI document", path)
			continue
		}
		ops := map[string]opInfo{
			"post": infoOf(item.Post), "get": infoOf(item.Get), "patch": infoOf(item.Patch), "delete": infoOf(item.Delete),
		}
		for method, needsAuth := range methods {
			op := ops[method]
			if op.id == "" {
				t.Errorf("%s %s is missing", method, path)
				continue
			}
			if !strings.HasPrefix(op.id, "auth-") {
				t.Errorf("%s %s: operation ID %q is not prefixed with the module name", method, path, op.id)
			}
			if op.secured != needsAuth {
				t.Errorf("%s %s: bearer security = %v, want %v", method, path, op.secured, needsAuth)
			}
		}
	}
}

type opInfo struct {
	id      string
	secured bool
}

func infoOf(op *huma.Operation) opInfo {
	if op == nil {
		return opInfo{}
	}
	return opInfo{id: op.OperationID, secured: len(op.Security) > 0}
}

func TestInitFailsWithADatabaseButNoSecret(t *testing.T) {
	pool := testPool(t)
	t.Setenv("JWT_SECRET", "")
	deps := &app.Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Services: app.NewServices(),
		Platform: map[string]any{"db": pool},
	}
	err := auth.New().Init(deps)
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Init err = %v, want one naming JWT_SECRET", err)
	}
}
