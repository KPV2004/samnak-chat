package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// newV1 returns a root router and the API mounted at /api/v1.
func newV1(t *testing.T) (chi.Router, huma.API) {
	t.Helper()
	root := httpx.NewRouter(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	var api huma.API
	root.Route("/api/v1", func(v1 chi.Router) {
		api = httpx.NewAPI(v1, "9.9.9")
	})
	return root, api
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestMetaContract(t *testing.T) {
	root, api := newV1(t)
	httpx.RegisterMeta(api, "samnak-chat", "0.0.0-dev", []string{"auth", "chat"})

	rec := get(root, "/api/v1/meta")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "samnak-chat" || got.Version != "0.0.0-dev" || strings.Join(got.Modules, ",") != "auth,chat" {
		t.Fatalf("meta = %+v", got)
	}
}

func TestMetaEmptyModulesIsArray(t *testing.T) {
	root, api := newV1(t)
	httpx.RegisterMeta(api, "samnak-chat", "0.0.0-dev", nil)

	if body := get(root, "/api/v1/meta").Body.String(); !strings.Contains(body, `"modules":[]`) {
		t.Fatalf("body = %s", body)
	}
}

func TestAPIErrorsUseErrorContract(t *testing.T) {
	root, api := newV1(t)
	huma.Get(api, "/missing", func(context.Context, *struct{}) (*struct{}, error) {
		return nil, huma.Error404NotFound("channel not found")
	})
	huma.Get(api, "/boom", func(context.Context, *struct{}) (*struct{}, error) {
		return nil, huma.Error500InternalServerError("boom")
	})
	huma.Get(api, "/num/{n}", func(context.Context, *struct {
		N int `path:"n"`
	}) (*struct{}, error) {
		return nil, nil
	})

	cases := []struct {
		path   string
		status int
		code   string
	}{
		{"/api/v1/missing", 404, "not_found"},
		{"/api/v1/boom", 500, "internal"},
		{"/api/v1/num/abc", 400, "invalid"}, // validation failures are 400, not Huma's 422
	}
	for _, c := range cases {
		rec := get(root, c.path)
		if rec.Code != c.status {
			t.Fatalf("%s: status = %d, want %d: %s", c.path, rec.Code, c.status, rec.Body)
		}
		var b errBody
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("%s: %v: %s", c.path, err, rec.Body)
		}
		if b.Error.Code != c.code || b.Error.Message == "" {
			t.Fatalf("%s: body = %s", c.path, rec.Body)
		}
	}
}

func TestOpenAPIDescribesMeta(t *testing.T) {
	_, api := newV1(t)
	httpx.RegisterMeta(api, "samnak-chat", "0.0.0-dev", nil)

	spec, err := api.OpenAPI().YAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/meta:", "operationId: get-meta", "url: /api/v1", "version: 9.9.9"} {
		if !strings.Contains(string(spec), want) {
			t.Errorf("openapi lacks %q:\n%s", want, spec)
		}
	}
}
