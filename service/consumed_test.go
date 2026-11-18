package service_test

import (
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func TestEndSessionDropsRowsAndCancelsApprovals(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	held := w.call(x, "s__delete_repo", `{}`)
	w.decide(held.Approval.ID, port.DecisionApproveForSession)
	w.sweep()
	other := w.call(x, "s__export", `{}`)
	want(t, other.Status, service.OutcomePending, "export is held")
	sys := w.Operator()
	sys.Claims.Tenant = tenant
	if err := w.Svc.EndSession(w.Ctx, sys, "s1"); err != nil {
		t.Fatal(err)
	}
	want(t, len(st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeSession}))), 0, "session rows deleted")
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, other.Approval.ID)).Code, port.ReasonSessionEnded, "pending approval cancelled")
	want(t, w.call(x, "s__search", `{}`).Reason, port.ReasonSessionEnded, "the session is refused at step 1")
	want(t, w.call(w.agent("x", "s2"), "s__search", `{}`).Status, service.OutcomeResult, "another session is not")
}

func TestRemovePersonRefusesTheirRunsAndOwnedAgents(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: "owned", AgentOwner: "alice", AskRequester: true}))
	st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "owned", Connector: w.conn.ID, Selector: "class:read", Mode: port.ModeAllow, ExpiresAt: st.Epoch.Add(time.Hour)}))
	x := w.agent("x", "s1")
	held := w.call(x, "s__delete_repo", `{}`)
	want(t, held.Status, service.OutcomePending, "held for alice")

	sys := w.Operator()
	sys.Claims.Tenant = tenant
	if err := w.Svc.RemovePerson(w.Ctx, sys, "alice", ""); err != nil {
		t.Fatal(err)
	}
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Code, port.ReasonPersonRemoved, "the requester's approval is cancelled")
	want(t, w.call(x, "s__search", `{}`).Reason, port.ReasonPersonRemoved, "no call runs for a person who left")
	owned := w.AgentClaims(port.Claims{Tenant: tenant, Workspace: workspace, Agent: "owned", AgentOwner: "alice", Session: "s1"})
	want(t, w.call(owned, "s__search", `{}`).Reason, port.ReasonPersonRemoved, "an agent the person owns is refused")

	if err := w.Svc.RestorePerson(w.Ctx, sys, "alice", ""); err != nil {
		t.Fatal(err)
	}
	want(t, w.call(x, "s__search", `{"q":"back"}`).Status, service.OutcomeResult, "restorePerson deletes the tombstone")
	want(t, w.call(owned, "s__search", `{}`).Reason, port.ReasonPersonRemoved, "the owner_removed flag stays until putBinding")
	st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: "owned", AskRequester: true}))
	want(t, w.call(owned, "s__search", `{}`).Status, service.OutcomeResult, "putBinding names another owner")
}

func TestRemoveAgentRevokesItsBindingsAndRows(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	held := w.call(x, "s__export", `{}`)
	sys := w.Operator()
	sys.Claims.Tenant = tenant
	if err := w.Svc.RemoveAgent(w.Ctx, sys, "x"); err != nil {
		t.Fatal(err)
	}
	want(t, w.call(x, "s__search", `{}`).Reason, port.ReasonUnknownFunction, "no binding, nothing resolves")
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Code, port.ReasonNoBinding, "pending approval cancelled")
	rows := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeAgent, ScopeID: "x"}))
	want(t, len(rows), 0, "agent rows revoked")
}

func TestListAgentFunctionsMatchesWhatTheAgentSees(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	got := st.Ok(w.Svc.ListAgentFunctions(w.Ctx, w.admin, port.Subject{Agent: "x", Session: "s1", OnBehalfOf: "alice", Workspace: workspace}))
	var names []string
	for _, f := range got.Functions {
		names = append(names, f.Name)
	}
	want(t, names, w.names(x), "same list")
	_, err := w.Svc.ListAgentFunctions(w.Ctx, w.alice, port.Subject{Agent: "x"})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "agent configurers only")
}
