# Modules

One folder per feature module, laid out as in `docs/architecture.md` section 5.3.
Core: `auth`, `workspace`, `chat`, `notify`. Optional: `task`, `approval`, `helpdesk`.

A module may import only `api.go` and `events.go` from another module.
