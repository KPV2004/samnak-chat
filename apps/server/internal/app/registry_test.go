package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// fakeModule records lifecycle calls into a shared log.
type fakeModule struct {
	name     string
	requires []string
	log      *[]string
	provide  func(s *Services)
	init     func(d *Deps) error
}

func (f *fakeModule) Name() string       { return f.name }
func (f *fakeModule) Requires() []string { return f.requires }

func (f *fakeModule) Provide(s *Services) {
	f.record("provide")
	if f.provide != nil {
		f.provide(s)
	}
}

func (f *fakeModule) Init(d *Deps) error {
	f.record("init")
	if f.init != nil {
		return f.init(d)
	}
	return nil
}

type pingOutput struct{ Body []byte }

func (f *fakeModule) Routes(api huma.API) {
	f.record("routes")
	huma.Register(api, huma.Operation{
		OperationID: f.name + "-ping",
		Method:      http.MethodGet,
		Path:        "/ping",
	}, func(context.Context, *struct{}) (*pingOutput, error) {
		return &pingOutput{Body: []byte("pong from " + f.name)}, nil
	})
}

// newTestAPI returns a Huma API and the router that serves it.
func newTestAPI() (huma.API, chi.Router) {
	r := chi.NewRouter()
	return humachi.New(r, huma.DefaultConfig("test", "0")), r
}

func testAPI() huma.API { api, _ := newTestAPI(); return api }

func (f *fakeModule) Subscribe(EventBus) { f.record("subscribe") }

func (f *fakeModule) record(step string) {
	if f.log != nil {
		*f.log = append(*f.log, step+":"+f.name)
	}
}

func fake(name string, requires ...string) *fakeModule {
	return &fakeModule{name: name, requires: requires}
}

func names(mods []Module) []string {
	out := make([]string, len(mods))
	for i, m := range mods {
		out[i] = m.Name()
	}
	return out
}

func TestEnableOrdersByDependency(t *testing.T) {
	reg := NewRegistry(fake("approval", "task"), fake("task", "chat"), fake("chat"))

	mods, err := reg.Enable([]string{"approval", "task", "chat"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	want := []string{"chat", "task", "approval"}
	if got := names(mods); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestEnableTiesBrokenAlphabetically(t *testing.T) {
	reg := NewRegistry(fake("zeta"), fake("alpha"), fake("mid"), fake("beta", "zeta"))

	// The input order must not matter.
	for _, in := range [][]string{
		{"zeta", "alpha", "mid", "beta"},
		{"beta", "mid", "alpha", "zeta"},
	} {
		mods, err := reg.Enable(in)
		if err != nil {
			t.Fatalf("Enable(%v): %v", in, err)
		}
		want := []string{"alpha", "mid", "zeta", "beta"}
		if got := names(mods); !reflect.DeepEqual(got, want) {
			t.Fatalf("Enable(%v) order = %v, want %v", in, got, want)
		}
	}
}

func TestEnableIgnoresDuplicateNames(t *testing.T) {
	reg := NewRegistry(fake("a"), fake("b"))
	mods, err := reg.Enable([]string{"b", "a", "b"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if got, want := names(mods), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestEnableUnknownModule(t *testing.T) {
	reg := NewRegistry(fake("chat"))
	_, err := reg.Enable([]string{"chat", "ghost"})
	if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
		t.Fatalf("err = %v, want error naming %q", err, "ghost")
	}
}

func TestEnableMissingDependency(t *testing.T) {
	reg := NewRegistry(fake("approval", "task"), fake("task"))
	_, err := reg.Enable([]string{"approval"})
	want := `module "approval" requires "task", which is not enabled`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestEnableMissingDependencyNotRegistered(t *testing.T) {
	reg := NewRegistry(fake("approval", "task"))
	_, err := reg.Enable([]string{"approval"})
	want := `module "approval" requires "task", which is not enabled`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestEnableCycle(t *testing.T) {
	reg := NewRegistry(fake("a", "b"), fake("b", "c"), fake("c", "a"), fake("ok"))
	_, err := reg.Enable([]string{"a", "b", "c", "ok"})
	if err == nil {
		t.Fatal("want cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "a -> b -> c -> a") {
		t.Fatalf("err = %q, want it to list the cycle a -> b -> c -> a", err)
	}
}

func TestEnableSelfCycle(t *testing.T) {
	reg := NewRegistry(fake("a", "a"))
	_, err := reg.Enable([]string{"a"})
	if err == nil || !strings.Contains(err.Error(), "a -> a") {
		t.Fatalf("err = %v, want cycle a -> a", err)
	}
}

func TestStartRunsPhasesInOrder(t *testing.T) {
	var log []string
	a, b := fake("a"), fake("b", "a")
	a.log, b.log = &log, &log
	reg := NewRegistry(b, a)

	mods, err := reg.Enable([]string{"a", "b"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	d := &Deps{Services: NewServices()}
	if err := reg.Start(mods, d, testAPI()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	want := []string{
		"provide:a", "provide:b",
		"init:a", "init:b",
		"routes:a", "routes:b",
		"subscribe:a", "subscribe:b",
	}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("calls = %v, want %v", log, want)
	}
}

type greeter interface{ Greet() string }

type greeterImpl struct{}

func (greeterImpl) Greet() string { return "hello" }

func TestStartProvideBeforeInit(t *testing.T) {
	// "a" is ordered before "b" but looks up a service that "b" provides.
	a, b := fake("a"), fake("b")
	b.provide = func(s *Services) { Provide[greeter](s, greeterImpl{}) }
	var got string
	a.init = func(d *Deps) error {
		g, ok := Lookup[greeter](d.Services)
		if !ok {
			return errors.New("greeter not provided yet")
		}
		got = g.Greet()
		return nil
	}

	reg := NewRegistry(a, b)
	mods, err := reg.Enable([]string{"a", "b"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if names(mods)[0] != "a" {
		t.Fatalf("test setup: a must be ordered first, got %v", names(mods))
	}
	if err := reg.Start(mods, &Deps{Services: NewServices()}, testAPI()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got != "hello" {
		t.Fatalf("greeting = %q, want hello", got)
	}
}

func TestStartInitErrorWrappedWithModuleName(t *testing.T) {
	boom := errors.New("boom")
	var log []string
	a, b, c := fake("a"), fake("b"), fake("c")
	a.log, b.log, c.log = &log, &log, &log
	b.init = func(*Deps) error { return boom }
	c.init = func(*Deps) error { return errors.New("second failure") }

	reg := NewRegistry(a, b, c)
	mods, err := reg.Enable([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	err = reg.Start(mods, &Deps{Services: NewServices()}, testAPI())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the Init error", err)
	}
	if !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("err = %q, want module name %q", err, "b")
	}
	for _, step := range log {
		if strings.HasPrefix(step, "init:c") || strings.HasPrefix(step, "routes:") || strings.HasPrefix(step, "subscribe:") {
			t.Fatalf("step %q ran after the first Init error", step)
		}
	}
}

func TestStartMountsRoutesUnderModuleName(t *testing.T) {
	// "off" is registered but not enabled, so its routes must not exist.
	reg := NewRegistry(fake("chat"), fake("task"), fake("off"))
	mods, err := reg.Enable([]string{"chat", "task"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	api, r := newTestAPI()
	if err := reg.Start(mods, &Deps{Services: NewServices()}, api); err != nil {
		t.Fatalf("Start: %v", err)
	}

	cases := []struct {
		path     string
		wantCode int
		wantBody string
	}{
		{"/chat/ping", http.StatusOK, "pong from chat"},
		{"/task/ping", http.StatusOK, "pong from task"},
		{"/off/ping", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.wantCode {
			t.Errorf("GET %s = %d, want %d", tc.path, rec.Code, tc.wantCode)
		}
		if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
			t.Errorf("GET %s body = %q, want %q", tc.path, rec.Body.String(), tc.wantBody)
		}
	}
}

func TestStartRequiresDeps(t *testing.T) {
	reg := NewRegistry(fake("a"))
	mods, _ := reg.Enable([]string{"a"})
	if err := reg.Start(mods, nil, testAPI()); err == nil {
		t.Fatal("want error for nil Deps")
	}
	if err := reg.Start(mods, &Deps{}, testAPI()); err == nil {
		t.Fatal("want error for nil Deps.Services")
	}
}

func TestStartPublishesModuleOperationsInOpenAPI(t *testing.T) {
	reg := NewRegistry(fake("chat"), fake("off"))
	mods, err := reg.Enable([]string{"chat"})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	api := testAPI()
	if err := reg.Start(mods, &Deps{Services: NewServices()}, api); err != nil {
		t.Fatalf("Start: %v", err)
	}
	paths := api.OpenAPI().Paths
	if _, ok := paths["/chat/ping"]; !ok {
		t.Errorf("OpenAPI paths = %v, want /chat/ping", keys(paths))
	}
	if _, ok := paths["/off/ping"]; ok {
		t.Error("OpenAPI lists /off/ping for a disabled module")
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
