package archtest

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

const root = "github.com/KPV2004/samnak-chat/apps/server/internal"

// violation returns a non-empty reason when from may not import to.
func violation(from, to string) string {
	app, platform, modules := root+"/app", root+"/platform", root+"/modules/"
	inTree := func(p, tree string) bool { return p == tree || strings.HasPrefix(p, tree+"/") }

	switch {
	case inTree(from, app) && (inTree(to, platform) || strings.HasPrefix(to, modules)):
		return "internal/app must not import platform or modules"
	case inTree(from, platform) && strings.HasPrefix(to, modules):
		return "internal/platform must not import modules"
	}
	if strings.HasPrefix(to, modules) {
		toMod, toRest, _ := strings.Cut(strings.TrimPrefix(to, modules), "/")
		layer, _, _ := strings.Cut(toRest, "/")
		if layer == "domain" || layer == "usecase" || layer == "adapter" {
			fromMod := ""
			if strings.HasPrefix(from, modules) {
				fromMod, _, _ = strings.Cut(strings.TrimPrefix(from, modules), "/")
			}
			if fromMod != toMod {
				return "module " + toMod + " internals may only be imported by module " + toMod
			}
		}
	}
	return ""
}

func TestViolationRules(t *testing.T) {
	m := root + "/modules/"
	cases := []struct {
		from, to string
		bad      bool
	}{
		{root + "/app", root + "/platform/database", true},
		{root + "/app", m + "chat", true},
		{root + "/platform/httpx", m + "chat", true},
		{root + "/platform/httpx", root + "/app", false},
		{m + "task", m + "chat/domain", true},
		{m + "task", m + "chat/usecase/x", true},
		{m + "task", m + "chat/adapter", true},
		{m + "task", m + "chat", false},
		{m + "chat/usecase", m + "chat/domain", false},
		{m + "chat", m + "chat/adapter/pg", false},
		{root + "/../cmd/server", m + "chat/domain", true},
		{root + "/../cmd/server", m + "chat", false},
	}
	for _, c := range cases {
		if got := violation(c.from, c.to) != ""; got != c.bad {
			t.Errorf("%s -> %s: violation=%v, want %v", c.from, c.to, got, c.bad)
		}
	}
}

func TestModuleBoundaries(t *testing.T) {
	out, err := exec.Command("go", "list", "-json", "../../...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		for _, imp := range p.Imports {
			if why := violation(p.ImportPath, imp); why != "" {
				t.Errorf("%s imports %s: %s", p.ImportPath, imp, why)
			}
		}
	}
}
