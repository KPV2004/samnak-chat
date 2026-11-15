package workspace

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

// Policy implements Authorizer and ActionRegistry with a table of allowed
// roles per action.
type Policy struct {
	mu      sync.RWMutex
	allowed map[Action]map[app.Role]bool
}

var (
	_ Authorizer     = (*Policy)(nil)
	_ ActionRegistry = (*Policy)(nil)
)

// NewAuthorizer returns a Policy that already knows the workspace actions.
func NewAuthorizer() *Policy {
	p := &Policy{allowed: map[Action]map[app.Role]bool{}}
	all := []app.Role{app.RoleOwner, app.RoleAdmin, app.RoleManager, app.RoleMember, app.RoleGuest}
	for _, reg := range []struct {
		action Action
		roles  []app.Role
	}{
		{ActionMemberList, all},
		{ActionMemberManage, []app.Role{app.RoleOwner, app.RoleAdmin}},
		{ActionInviteManage, []app.Role{app.RoleOwner, app.RoleAdmin}},
	} {
		if err := p.Register(reg.action, reg.roles...); err != nil {
			panic(err) // the table above is constant
		}
	}
	return p
}

func (p *Policy) Can(pr app.Principal, action Action) bool {
	if pr.WorkspaceID == "" || pr.Role == "" {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.allowed[action][pr.Role]
}

func (p *Policy) Register(action Action, roles ...app.Role) error {
	if action == "" {
		return fmt.Errorf("workspace: register: empty action")
	}
	set := make(map[app.Role]bool, len(roles))
	for _, r := range roles {
		if !domain.ValidRole(r) {
			return fmt.Errorf("workspace: register %q: unknown role %q", action, r)
		}
		set[r] = true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if have, ok := p.allowed[action]; ok {
		if !maps.Equal(have, set) {
			return fmt.Errorf("workspace: register %q: already registered for roles %v", action, slices.Sorted(maps.Keys(have)))
		}
		return nil
	}
	p.allowed[action] = set
	return nil
}
