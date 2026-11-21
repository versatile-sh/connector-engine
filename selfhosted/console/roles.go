package console

import (
	"context"
	"net/http"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	engineTenantAdmin     = "tenant_admin"
	engineAgentConfigurer = "agent_configurer"
)

func (c *Console) engineRole(u User) port.RoleAssignment {
	tenant := c.Engine.Tenant
	row := port.RoleAssignment{Tenant: port.TenantID(tenant), Person: port.PersonID(u.Person), Role: engineTenantAdmin, Scope: "t/" + tenant}
	if !u.Role.AtLeast(RoleAdmin) {
		row.Role, row.Scope = engineAgentConfigurer, "t/"+tenant+"/w/"+c.Engine.Workspace
	}
	return row
}

func (c *Console) managers() port.GroupID {
	return port.GroupID(firstOf(c.ManagersGroup, defaultManagersGroup))
}

// ReconcileRoles puts, as the service person, the users' engine roles and managers group members, and deletes those of users and gone persons that should not be there.
func (c *Console) ReconcileRoles(ctx context.Context, gone ...string) error {
	c.reconciling.Lock()
	defer c.reconciling.Unlock()
	return c.reconcileRoles(ctx, c.State.Users(), gone)
}

func (c *Console) reconcileRoles(ctx context.Context, users []User, gone []string) error {
	ctx = WithActor(ctx, c.Engine.service())
	tenant := port.TenantID(c.Engine.Tenant)
	managed := map[port.PersonID]bool{}
	for _, p := range gone {
		managed[port.PersonID(p)] = true
	}
	wantRows := map[port.RoleAssignment]bool{}
	wantMembers := map[port.PersonID]bool{}
	for _, u := range users {
		managed[port.PersonID(u.Person)] = true
		wantRows[c.engineRole(u)] = true
		wantMembers[port.PersonID(u.Person)] = u.Role.AtLeast(RoleAdmin)
	}
	legacy := port.RoleAssignment{Tenant: tenant, Person: defaultPerson, Role: engineTenantAdmin, Scope: "t/" + string(tenant)}
	rows, err := list[port.RoleAssignment](ctx, c.Engine, "/v1/roles"+q("tenant", string(tenant)))
	if err != nil {
		return err
	}
	delete(managed, port.PersonID(c.Engine.service()))
	for _, r := range rows {
		if r.Issuer != "" {
			continue
		}
		r.Tenant = tenant
		stale := managed[r.Person] && !wantRows[r] || r == legacy && !managed[r.Person]
		if !stale {
			delete(wantRows, r)
			continue
		}
		if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/roles"+q("tenant", string(tenant), "person", string(r.Person), "role", r.Role, "scope", r.Scope), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	members, err := list[port.GroupMember](ctx, c.Engine, "/v1/groups/members"+q("group", string(c.managers())))
	if err != nil {
		return err
	}
	for _, m := range members {
		if want, ok := wantMembers[m.Person]; ok && want || !managed[m.Person] {
			wantMembers[m.Person] = false
			continue
		}
		if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/groups/members"+q("tenant", string(tenant), "person", string(m.Person), "group", string(m.Group)), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	for r := range wantRows {
		if err := c.Engine.Call(ctx, http.MethodPut, "/v1/roles", r, nil); err != nil {
			return err
		}
	}
	for p, want := range wantMembers {
		if !want {
			continue
		}
		if err := c.Engine.Call(ctx, http.MethodPut, "/v1/groups/members", port.GroupMember{Tenant: tenant, Person: p, Group: c.managers()}, nil); err != nil {
			return err
		}
	}
	return nil
}

// SetUserRole changes the role of person failing closed: a raise writes state then engine, a lowering engine then state, and the last Owner stays.
func (c *Console) SetUserRole(ctx context.Context, person string, role Role) error {
	if _, err := ParseRole(string(role)); err != nil {
		return err
	}
	c.reconciling.Lock()
	defer c.reconciling.Unlock()
	users := c.State.Users()
	i := slices.IndexFunc(users, func(u User) bool { return u.Person == person })
	switch {
	case i < 0:
		return notFound("no user is %s", person)
	case users[i].Role == role:
		return nil
	case role.AtLeast(users[i].Role):
		if err := c.State.SetRole(actorOf(ctx), person, role); err != nil {
			return err
		}
		return c.reconcileRoles(ctx, c.State.Users(), nil)
	}
	if users[i].Role == RoleOwner && !slices.ContainsFunc(users, func(u User) bool { return u.Role == RoleOwner && u.Person != person }) {
		return ErrLastOwner
	}
	users[i].Role = role
	if err := c.reconcileRoles(ctx, users, nil); err != nil {
		return err
	}
	return c.State.SetRole(actorOf(ctx), person, role)
}
