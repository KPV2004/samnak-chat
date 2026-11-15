package chat_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// The openapi command starts modules with no database, no Tx, no Realtime and
// possibly no bus; every operation must still be registered under /chat.
func TestOperationsAppearInOpenAPIWithoutDatabase(t *testing.T) {
	mod := chat.New()
	reg := app.NewRegistry(mod)
	mods, err := reg.Enable([]string{"chat"})
	if err != nil {
		t.Fatal(err)
	}
	api := httpx.NewAPI(chi.NewRouter(), "test")
	deps := &app.Deps{Logger: slog.New(slog.DiscardHandler), Services: app.NewServices(), Notifications: app.NoopNotificationRegistry{}}
	if err := reg.Start(mods, deps, api); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"/chat/channels":               {"GET", "POST"},
		"/chat/dms":                    {"POST"},
		"/chat/channels/{id}":          {"GET", "PATCH"},
		"/chat/channels/{id}/join":     {"POST"},
		"/chat/channels/{id}/leave":    {"POST"},
		"/chat/channels/{id}/members":  {"GET", "POST"},
		"/chat/channels/{id}/messages": {"GET", "POST"},
		"/chat/channels/{id}/read":     {"POST"},
		"/chat/messages/{id}":          {"PATCH", "DELETE"},
	}
	paths := api.OpenAPI().Paths
	for path, methods := range want {
		item := paths[path]
		if item == nil {
			t.Errorf("path %s missing from OpenAPI", path)
			continue
		}
		ops := map[string]bool{"POST": item.Post != nil, "GET": item.Get != nil, "PATCH": item.Patch != nil, "DELETE": item.Delete != nil}
		for _, m := range methods {
			if !ops[m] {
				t.Errorf("%s %s missing", m, path)
			}
		}
	}
}

// chat provides the CardRegistry contract; until M2-05 every call fails with
// an error that wraps app.ErrInvalid.
func TestCardRegistryStub(t *testing.T) {
	svc := app.NewServices()
	chat.New().Provide(svc)
	cards, ok := app.Lookup[app.CardRegistry](svc)
	if !ok {
		t.Fatal("chat does not provide app.CardRegistry")
	}
	cards.Register(app.CardKind{Kind: "demo.card", Module: "demo"}) // must not panic
	ctx := t.Context()
	if _, err := cards.Post(ctx, app.Card{Kind: "demo.card"}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Post = %v, want ErrInvalid", err)
	}
	if err := cards.Update(ctx, "w", "m", nil); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Update = %v, want ErrInvalid", err)
	}
	if _, err := cards.DirectChannel(ctx, "w", nil); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("DirectChannel = %v, want ErrInvalid", err)
	}
}
