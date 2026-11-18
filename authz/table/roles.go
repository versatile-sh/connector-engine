package table

import (
	"slices"
	"sync"

	"github.com/agents-control/connector-engine/port"
)

// Role is a delegated role of §4.2.
type Role string

// Roles of §4.2.
const (
	Operator        Role = "operator"
	TenantAdmin     Role = "tenant_admin"
	WorkspaceAdmin  Role = "workspace_admin"
	AgentConfigurer Role = "agent_configurer"
	Reviewer        Role = "reviewer"
	Approver        Role = "approver"
	Auditor         Role = "auditor"
)

// Assignment gives a person a role over a resource scope of package authz.
type Assignment struct {
	Tenant port.TenantID
	Person port.PersonID
	Role   Role
	Scope  string
}

// Roles holds the role assignments and group memberships the table authorizer reads.
type Roles struct {
	mu     sync.RWMutex
	grants []Assignment
	groups map[port.TenantID]map[port.PersonID][]port.GroupID
}

// NewRoles returns an empty role table.
func NewRoles() *Roles { return &Roles{groups: map[port.TenantID]map[port.PersonID][]port.GroupID{}} }

// Assign adds an assignment; a duplicate is ignored.
func (r *Roles) Assign(a Assignment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.Contains(r.grants, a) {
		r.grants = append(r.grants, a)
	}
}

// Revoke removes an assignment.
func (r *Roles) Revoke(a Assignment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants = slices.DeleteFunc(r.grants, func(x Assignment) bool { return x == a })
}

// Join makes a person a member of a group.
func (r *Roles) Join(tenant port.TenantID, person port.PersonID, group port.GroupID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.groups[tenant] == nil {
		r.groups[tenant] = map[port.PersonID][]port.GroupID{}
	}
	if !slices.Contains(r.groups[tenant][person], group) {
		r.groups[tenant][person] = append(r.groups[tenant][person], group)
	}
}

func (r *Roles) of(tenant port.TenantID, person port.PersonID) []Assignment {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Assignment
	for _, a := range r.grants {
		if (a.Tenant == tenant || a.Tenant == port.Installation) && a.Person == person {
			out = append(out, a)
		}
	}
	return out
}

func (r *Roles) groupsOf(tenant port.TenantID, person port.PersonID) []port.GroupID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.groups[tenant][person])
}
