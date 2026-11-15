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

		"/chat/uploads":                         {"POST"},
		"/chat/attachments/{id}":                {"GET"},
		"/chat/messages/{id}/reactions":         {"POST"},
		"/chat/messages/{id}/reactions/{emoji}": {"DELETE"},
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

// chat provides the CardRegistry contract. Without a database the checks that
// come before any storage still apply: a kind must be registered under its
// module's name.
func TestCardRegistryChecksKinds(t *testing.T) {
	svc := app.NewServices()
	chat.New().Provide(svc)
	cards, ok := app.Lookup[app.CardRegistry](svc)
	if !ok {
		t.Fatal("chat does not provide app.CardRegistry")
	}
	ctx := t.Context()
	post := func(kind string) error {
		_, err := cards.Post(ctx, app.Card{Kind: kind, FallbackText: "x"})
		return err
	}
	if err := post("demo.card"); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Post of an unregistered kind = %v, want ErrInvalid", err)
	}
	for _, bad := range []app.CardKind{
		{Kind: "other.card", Module: "demo"},
		{Kind: "democard", Module: "demo"},
		{Kind: "demo.", Module: "demo"},
		{Kind: "demo.card", Module: ""},
		{Kind: "text", Module: "text"},
	} {
		cards.Register(bad) // ignored, and must not panic
		if err := post(bad.Kind); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("Post after Register(%+v) = %v, want ErrInvalid", bad, err)
		}
	}
	if err := cards.Update(ctx, "w", "m", nil); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("Update with a bad workspace = %v, want ErrInvalid", err)
	}
}
