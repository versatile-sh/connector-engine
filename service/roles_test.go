package service_test

import (
	"testing"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func TestTenantAdminDelegatesRolesWithinItsTenantOnly(t *testing.T) {
	e := st.New(t)
	admin := e.TenantAdmin("t1")
	role := port.RoleAssignment{Tenant: "t1", Person: "bia", Role: string(table.Approver), Scope: authz.Workspace("t1", "w1")}
	st.Ok(e.Svc.PutRole(e.Ctx, admin, role))
	st.Ok(e.Svc.PutRole(e.Ctx, admin, role))
	got := st.Ok(e.Svc.ListRoles(e.Ctx, admin, "t1", port.RoleFilter{Person: "bia"}))
	want(t, len(got), 1, "one row after two puts")
	bia := e.Person("t1", "bia")
	allowed, err := e.Auth.Allowed(e.Ctx, port.Permission{Principal: bia, Action: authz.PauseClear, Resource: authz.Workspace("t1", "w1")})
	want(t, allowed && err == nil, true, "the new role is read by the authorizer")

	for name, bad := range map[string]port.RoleAssignment{
		"another tenant":     {Tenant: "t2", Person: "bia", Role: string(table.Approver), Scope: authz.Tenant("t2")},
		"scope outside":      {Tenant: "t1", Person: "bia", Role: string(table.Approver), Scope: authz.Tenant("t2")},
		"unknown role":       {Tenant: "t1", Person: "bia", Role: "god", Scope: authz.Tenant("t1")},
		"operator in tenant": {Tenant: "t1", Person: "bia", Role: string(table.Operator), Scope: authz.Installation},
	} {
		if _, err := e.Svc.PutRole(e.Ctx, admin, bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	_, err = e.Svc.PutRole(e.Ctx, bia, role)
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "an approver writes no roles")
	_, err = e.Svc.PutRole(e.Ctx, e.AgentClaims(port.Claims{Tenant: "t1", Agent: "x"}), role)
	want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "an agent writes no roles")
	want(t, e.Svc.DeleteRole(e.Ctx, admin, role), nil, "delete")
	want(t, len(st.Ok(e.Svc.ListRoles(e.Ctx, admin, "t1", port.RoleFilter{Person: "bia"}))), 0, "gone")
}

func TestOperatorNamesOperatorsAndGroupsFollowTheTenant(t *testing.T) {
	e := st.New(t)
	op := port.RoleAssignment{Issuer: "corp", Person: "boss", Role: string(table.Operator), Scope: authz.Installation}
	_, err := e.Svc.PutRole(e.Ctx, e.Operator(), port.RoleAssignment{Person: "boss", Role: string(table.Operator), Scope: authz.Installation})
	want(t, port.IsCode(err, port.CodeInvalidArgument), true, "an installation role names its issuer")
	st.Ok(e.Svc.PutRole(e.Ctx, e.Operator(), op))
	_, err = e.Svc.PutRole(e.Ctx, e.TenantAdmin("t1"), op)
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a tenant admin writes no installation role")
	_, err = e.Svc.PutRole(e.Ctx, e.Operator(), port.RoleAssignment{Tenant: "t1", Person: "x", Role: string(table.Approver), Scope: authz.Tenant("t1")})
	want(t, port.IsCode(err, port.CodeNotFound), true, "the operator writes no tenant role")

	admin := e.TenantAdmin("t1")
	st.Ok(e.Svc.PutGroupMember(e.Ctx, admin, port.GroupMember{Person: "bia", Group: "ops"}))
	groups := st.Ok(e.Auth.Groups(e.Ctx, "t1", "bia"))
	want(t, len(groups) == 1 && groups[0] == "ops", true, "membership read back")
	want(t, len(st.Ok(e.Svc.ListGroupMembers(e.Ctx, admin, port.MemberFilter{Group: "ops"}))), 1, "listed")
	want(t, e.Svc.DeleteGroupMember(e.Ctx, admin, port.GroupMember{Person: "bia", Group: "ops"}), nil, "removed")
}

func TestIDsOutsideTheCharacterSetAreRefused(t *testing.T) {
	e := st.New(t)
	admin := e.TenantAdmin("t1")
	refused := func(what string, err error) {
		t.Helper()
		want(t, port.IsCode(err, port.CodeInvalidArgument), true, what)
	}
	for _, id := range []port.TenantID{"installation", "a/b", "t 1"} {
		_, err := e.Svc.CreateTenant(e.Ctx, e.Operator(), id)
		refused("tenant "+string(id), err)
	}
	_, err := e.Svc.PutRole(e.Ctx, admin, port.RoleAssignment{Tenant: "t1", Person: "bia/p/x", Role: string(table.Approver), Scope: authz.Tenant("t1")})
	refused("role person", err)
	_, err = e.Svc.PutGroupMember(e.Ctx, admin, port.GroupMember{Person: "bia", Group: "ops/x"})
	refused("group", err)
	_, err = e.Svc.InstallConnector(e.Ctx, admin, service.InstallInput{Workspace: "w1/x", Alias: "notes"})
	refused("workspace", err)
	_, err = e.Svc.PutBinding(e.Ctx, admin, port.Binding{Connector: "c", Agent: "a/b"})
	refused("agent", err)
}

func TestATenantThatWasNeverCreatedIsNotActive(t *testing.T) {
	e := st.New(t)
	want(t, port.IsCode(e.Svc.TenantActive(e.Ctx, "ghost"), port.CodeTenantRemoved), true, "unknown tenant")
	ghost := port.Principal{Kind: port.PrincipalPerson, Person: "admin", Claims: port.Claims{Tenant: "ghost", Subject: "admin"}}
	_, err := e.Svc.ListRoles(e.Ctx, ghost, "ghost", port.RoleFilter{})
	want(t, port.IsCode(err, port.CodeTenantRemoved), true, "a person of an unknown tenant")
	e.Tenant("t1")
	want(t, e.Svc.TenantActive(e.Ctx, "t1"), nil, "registered tenant")
}

func TestRemovePersonDropsTheirRolesAndGroups(t *testing.T) {
	e := st.New(t)
	admin := e.TenantAdmin("t1")
	bia := e.Person("t1", "bia", [2]string{string(table.Approver), authz.Workspace("t1", "w1")}, [2]string{string(table.Auditor), authz.Workspace("t1", "w2")})
	e.Join("t1", "bia", "ops")
	sys := e.Operator()
	sys.Claims.Tenant = "t1"
	want(t, e.Svc.RemovePerson(e.Ctx, sys, bia.Person, "w1"), nil, "remove from w1")
	want(t, len(st.Ok(e.Svc.ListRoles(e.Ctx, admin, "t1", port.RoleFilter{Person: "bia"}))), 1, "the w2 role stays")
	want(t, len(st.Ok(e.Auth.Groups(e.Ctx, "t1", "bia"))), 1, "groups stay on a workspace removal")
	want(t, e.Svc.RemovePerson(e.Ctx, sys, bia.Person, ""), nil, "remove everywhere")
	want(t, len(st.Ok(e.Svc.ListRoles(e.Ctx, admin, "t1", port.RoleFilter{Person: "bia"}))), 0, "no role left")
	want(t, len(st.Ok(e.Auth.Groups(e.Ctx, "t1", "bia"))), 0, "no group left")
}
