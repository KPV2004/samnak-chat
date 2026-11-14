package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

type fakeModule struct {
	name     string
	requires []string
	initErr  error
}

func (f *fakeModule) Name() string           { return f.name }
func (f *fakeModule) Requires() []string     { return f.requires }
func (f *fakeModule) Provide(*app.Services)  {}
func (f *fakeModule) Init(*app.Deps) error   { return f.initErr }
func (f *fakeModule) Subscribe(app.EventBus) {}
func (f *fakeModule) Routes(api huma.API) {
	huma.Get(api, "/ping", func(context.Context, *struct{}) (*struct{ Body string }, error) {
		return &struct{ Body string }{Body: "pong"}, nil
	})
}

func testLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)) }

func build(t *testing.T, names []string, available ...app.Module) (http.Handler, error) {
	t.Helper()
	return newHandler(testLogger(), names, available, infra{})
}

func getPath(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func metaModules(t *testing.T, h http.Handler) []string {
	t.Helper()
	rec := getPath(h, "/api/v1/meta")
	if rec.Code != 200 {
		t.Fatalf("meta status = %d: %s", rec.Code, rec.Body)
	}
	var m struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "samnak-chat" || m.Version == "" || m.Modules == nil {
		t.Fatalf("meta = %+v", m)
	}
	return m.Modules
}

func TestMetaListsModulesInStartupOrder(t *testing.T) {
	h, err := build(t, []string{"task", "auth"},
		&fakeModule{name: "task", requires: []string{"auth"}},
		&fakeModule{name: "auth"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(metaModules(t, h), ","); got != "auth,task" {
		t.Fatalf("modules = %s", got)
	}
	if rec := getPath(h, "/api/v1/task/ping"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("module route = %d %q", rec.Code, rec.Body)
	}
}

func TestDisabledModuleReturns404(t *testing.T) {
	h, err := build(t, []string{"auth"}, &fakeModule{name: "auth"}, &fakeModule{name: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := getPath(h, "/api/v1/task/ping"); rec.Code != 404 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNoModulesMetaIsEmptyList(t *testing.T) {
	h, err := build(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := metaModules(t, h); len(got) != 0 {
		t.Fatalf("modules = %v", got)
	}
}

func TestMissingDependencyFailsStartupNamingModule(t *testing.T) {
	_, err := build(t, []string{"task"}, &fakeModule{name: "task", requires: []string{"auth"}})
	if err == nil || !strings.Contains(err.Error(), `"task"`) || !strings.Contains(err.Error(), `"auth"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownModuleFailsStartupNamingIt(t *testing.T) {
	_, err := build(t, []string{"nope"})
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestInitFailureFailsStartupNamingModule(t *testing.T) {
	_, err := build(t, []string{"task"}, &fakeModule{name: "task", initErr: io.ErrUnexpectedEOF})
	if err == nil || !strings.Contains(err.Error(), `"task"`) {
		t.Fatalf("err = %v", err)
	}
}

// Core modules have no implementation until M1; the server must still boot,
// but a name that is not a core module and not registered stays an error.
func TestUnregisteredCoreModulesAreSkipped(t *testing.T) {
	h, err := build(t, append(append([]string{}, app.CoreModules...), "nope"))
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("err = %v, h = %v", err, h)
	}
	h, err = build(t, app.CoreModules)
	if err != nil {
		t.Fatal(err)
	}
	if got := metaModules(t, h); len(got) != 0 {
		t.Fatalf("modules = %v", got)
	}
}

func TestReadyzReportsFailingCheck(t *testing.T) {
	failing := io.ErrClosedPipe
	h, err := newHandler(testLogger(), nil, nil, infra{Checks: []httpx.Check{{Name: "db", Fn: func(context.Context) error { return failing }}}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := getPath(h, "/readyz"); rec.Code != 503 || !strings.Contains(rec.Body.String(), "db") {
		t.Fatalf("readyz = %d %s", rec.Code, rec.Body)
	}
	if rec := getPath(h, "/healthz"); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestOpenAPIYAMLPrintsSpec(t *testing.T) {
	var buf bytes.Buffer
	if err := writeOpenAPI(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"openapi: 3.", "/meta:", "get-meta"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("spec lacks %q", want)
		}
	}
}

func TestServeShutsDownGracefully(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, ln, mux, testLogger(), 5*time.Second) }()

	body := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			body <- err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	}()

	<-started
	cancel() // like SIGTERM: stop accepting, let the in-flight request finish
	select {
	case err := <-served:
		t.Fatalf("serve returned before the request finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if got := <-body; got != "done" {
		t.Fatalf("in-flight response = %q", got)
	}
	if err := <-served; err != nil {
		t.Fatalf("serve = %v", err)
	}
}

func TestServeShutdownTimeoutIsAnError(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, ln, mux, testLogger(), 50*time.Millisecond) }()
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/hang")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()
	if err := <-served; err == nil {
		t.Fatal("want a shutdown timeout error")
	}
}
