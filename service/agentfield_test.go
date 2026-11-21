package service_test

import (
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

func TestUnbindKeepsUserRows(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	w.bind("y")
	w.row(port.ScopeAgent, "y", "*", port.ModeAllow)
	w.Write(tenant, func(tx port.Tx) error {
		return tx.PutGrant(port.Grant{ID: "u1", Tenant: tenant, ScopeKind: port.ScopeUser, ScopeID: "alice", Agent: "x", Connector: w.conn.ID, Selector: "*", Mode: port.ModeDeny, Origin: port.OriginSettings})
	})
	y := port.Subject{Agent: "y", Session: "s1", OnBehalfOf: "alice"}
	want(t, st.Ok(w.Svc.ExplainAccess(w.Ctx, w.admin, y, w.conn.ID, "search")).Mode, port.ModeDeny, "before unbind")
	if err := w.Svc.DeleteBinding(w.Ctx, w.admin, w.conn.ID, "x"); err != nil {
		t.Fatal(err)
	}
	want(t, st.Ok(w.Svc.ExplainAccess(w.Ctx, w.admin, y, w.conn.ID, "search")).Mode, port.ModeDeny, "after another agent is unbound")
}

func TestCreateGrantRefusesAUserRowNamingAnAgent(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	_, err := w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeUser, ScopeID: "alice", Agent: "x", Connector: w.conn.ID, Selector: "*", Mode: port.ModeDeny})
	want(t, port.IsCode(err, port.CodeRowRefused), true, "refused")
}

func TestAnAgentRowIsStoredInOneForm(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.bind("x")
	exp := st.Epoch.Add(24 * time.Hour)
	first := st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Agent: "x", Connector: w.conn.ID, Selector: "*", Mode: port.ModeAllow, ExpiresAt: exp}))
	want(t, first.Grant.Agent, port.AgentID(""), "stored agent")
	second := st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: w.conn.ID, Selector: "*", Mode: port.ModeDeny}))
	want(t, second.Replaced != nil && second.Replaced.ID == first.Grant.ID, true, "the second row replaces the first")
	rows := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeAgent, ScopeID: "x"}))
	want(t, len(rows), 1, "rows")
}
