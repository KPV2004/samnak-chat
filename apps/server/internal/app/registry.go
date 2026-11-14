package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Registry checks dependencies, orders modules and starts them.
// See docs/architecture.md section 5.2.
type Registry struct {
	all map[string]Module
}

func NewRegistry(modules ...Module) *Registry {
	r := &Registry{all: map[string]Module{}}
	for _, m := range modules {
		r.all[m.Name()] = m
	}
	return r
}

// Enable returns the named modules ordered so that every module comes after
// the modules it requires. Modules that do not depend on each other are
// ordered alphabetically, so the result is the same on every run.
//
// It fails when a name is unknown, when a required module is not in names, or
// when Requires() form a cycle. Core modules are not added here; the caller
// decides which names to pass (see CoreModules).
func (r *Registry) Enable(names []string) ([]Module, error) {
	enabled := map[string]Module{}
	for _, name := range names {
		m, ok := r.all[name]
		if !ok {
			return nil, fmt.Errorf("unknown module %q", name)
		}
		enabled[name] = m
	}
	sorted := make([]string, 0, len(enabled))
	for name := range enabled {
		sorted = append(sorted, name)
	}
	slices.Sort(sorted)

	for _, name := range sorted {
		for _, req := range sortedRequires(enabled[name]) {
			if _, ok := enabled[req]; !ok {
				return nil, fmt.Errorf("module %q requires %q, which is not enabled", name, req)
			}
		}
	}
	if cycle := findCycle(sorted, enabled); cycle != nil {
		return nil, fmt.Errorf("module dependency cycle: %s", strings.Join(cycle, " -> "))
	}

	// With no cycle, a repeated "pick the first module whose requirements are
	// all placed" yields the alphabetical-tiebreak topological order.
	ordered := make([]Module, 0, len(sorted))
	placed := map[string]bool{}
	for len(ordered) < len(sorted) {
		for _, name := range sorted {
			if placed[name] || !allPlaced(enabled[name], placed) {
				continue
			}
			placed[name] = true
			ordered = append(ordered, enabled[name])
			break
		}
	}
	return ordered, nil
}

// Start runs the startup sequence on mods, which must come from Enable.
// Provide runs on every module before any Init, so Init can look up services
// of modules ordered after it. Each module's routes are registered on a Huma
// group prefixed "/{name}", so a module that is not in mods has no routes and
// answers 404.
func (r *Registry) Start(mods []Module, d *Deps, api huma.API) error {
	if d == nil || d.Services == nil {
		return errors.New("start modules: Deps and Deps.Services are required")
	}
	for _, m := range mods {
		m.Provide(d.Services)
	}
	for _, m := range mods {
		if err := m.Init(d); err != nil {
			return fmt.Errorf("init module %q: %w", m.Name(), err)
		}
	}
	for _, m := range mods {
		m.Routes(huma.NewGroup(api, "/"+m.Name()))
	}
	for _, m := range mods {
		m.Subscribe(d.Bus)
	}
	return nil
}

func sortedRequires(m Module) []string {
	reqs := slices.Clone(m.Requires())
	slices.Sort(reqs)
	return reqs
}

func allPlaced(m Module, placed map[string]bool) bool {
	for _, req := range m.Requires() {
		if !placed[req] {
			return false
		}
	}
	return true
}

// findCycle returns the first dependency cycle as a path that starts and ends
// on the same module (for example a, b, a), or nil when there is none.
// Every Requires() entry must be in enabled.
func findCycle(sorted []string, enabled map[string]Module) []string {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var path []string

	var visit func(name string) []string
	visit = func(name string) []string {
		switch state[name] {
		case done:
			return nil
		case visiting:
			i := slices.Index(path, name)
			return append(slices.Clone(path[i:]), name)
		}
		state[name] = visiting
		path = append(path, name)
		for _, req := range sortedRequires(enabled[name]) {
			if cycle := visit(req); cycle != nil {
				return cycle
			}
		}
		path = path[:len(path)-1]
		state[name] = done
		return nil
	}

	for _, name := range sorted {
		if cycle := visit(name); cycle != nil {
			return cycle
		}
	}
	return nil
}
