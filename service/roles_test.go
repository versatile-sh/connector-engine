package service_test

import (
	"testing"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
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
