package table_test

import (
	"testing"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func TestAllowedFollowsRolesAndScopes(t *testing.T) {
	e := st.New(t)
	w1 := authz.Workspace("t1", "w1")
	wsAdmin := e.Person("t1", "wa", [2]string{string(table.WorkspaceAdmin), w1})
	tenantAdmin := e.Person("t1", "ta", [2]string{string(table.TenantAdmin), authz.Tenant("t1")})
	member := e.Person("t1", "m")
	op := e.Operator()
	for _, c := range []struct {
		name     string
		p        port.Principal
		action   string
		resource string
		want     bool
	}{
		{"workspace admin installs in its workspace", wsAdmin, authz.ConnectorInstall, w1, true},
		{"workspace admin does not install in another", wsAdmin, authz.ConnectorInstall, authz.Workspace("t1", "w2"), false},
		{"workspace admin writes no catalog rule", wsAdmin, authz.CatalogWrite, authz.Tenant("t1"), false},
		{"tenant admin writes catalog rules", tenantAdmin, authz.CatalogWrite, authz.Tenant("t1"), true},
		{"tenant admin covers every workspace", tenantAdmin, authz.ConnectorInstall, authz.Workspace("t1", "w9"), true},
		{"tenant admin has no say in another tenant", tenantAdmin, authz.ConnectorInstall, authz.Workspace("t2", "w1"), false},
		{"tenant admin registers no tenant", tenantAdmin, authz.TenantCreate, authz.Installation, false},
		{"a person writes their own rows", member, authz.GrantUser, authz.Person("t1", "m"), true},
		{"a person does not write another's", member, authz.GrantUser, authz.Person("t1", "o"), false},
		{"any person may be asked to decide", member, authz.ApprovalDecide, w1, true},
		{"the operator registers tenants", op, authz.TenantCreate, authz.Installation, true},
		{"the operator does not install", op, authz.ConnectorInstall, w1, false},
		{"an agent is never allowed", e.AgentClaims(port.Claims{Tenant: "t1", Agent: "x"}), authz.ConnectorRead, authz.Tenant("t1"), false},
	} {
		got, err := e.Auth.Allowed(e.Ctx, port.Permission{Principal: c.p, Action: c.action, Resource: c.resource})
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v want %v", c.name, got, err, c.want)
		}
	}
}

func TestGroupsAreTheJoinedOnes(t *testing.T) {
	e := st.New(t)
	e.Join("t1", "m", "platform")
	g, _ := e.Auth.Groups(e.Ctx, "t1", "m")
	if len(g) != 1 || g[0] != "platform" {
		t.Fatalf("groups: %v", g)
	}
	if other, _ := e.Auth.Groups(e.Ctx, "t2", "m"); len(other) != 0 {
		t.Fatal("groups do not cross tenants")
	}
}

func TestAnInstallationRoleRowCoversEveryTenantAndAnotherTenantsRowDoesNot(t *testing.T) {
	e := st.New(t)
	e.Write("", func(tx port.Tx) error {
		return tx.PutRole(port.RoleAssignment{Person: "boss", Role: string(table.Operator), Scope: authz.Installation})
	})
	e.Write("t2", func(tx port.Tx) error {
		return tx.PutRole(port.RoleAssignment{Tenant: "t2", Person: "boss", Role: string(table.TenantAdmin), Scope: authz.Tenant("t2")})
	})
	boss := port.Principal{Kind: port.PrincipalPerson, Person: "boss", Claims: port.Claims{Tenant: "t1", Subject: "boss"}}
	for action, want := range map[string]bool{authz.TenantCreate: true, authz.CatalogWrite: false} {
		if got, err := e.Auth.Allowed(e.Ctx, port.Permission{Principal: boss, Action: action, Resource: authz.Tenant("t2")}); err != nil || got != want {
			t.Errorf("%s: %v %v, want %v", action, got, err, want)
		}
	}
}
