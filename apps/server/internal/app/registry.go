package app

// Registry checks dependencies, orders modules and starts them.
// See docs/architecture.md section 5.2. Implementation: task M0-03.
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
