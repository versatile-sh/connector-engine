package service_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func want(t *testing.T, got, exp any, what string) {
	t.Helper()
	if !equal(got, exp) {
		t.Fatalf("%s: got %v, want %v", what, got, exp)
	}
}

func equal(a, b any) bool {
	as, aok := a.([]string)
	bs, bok := b.([]string)
	if aok && bok {
		return slices.Equal(as, bs)
	}
	return a == b
}

func TestCaseOneFunctionForOneAgent(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
	x := w.agent("x", "s1")

	want(t, w.names(x), []string{"s__create_issue"}, "list")
	want(t, w.call(x, "s__create_issue", `{"q":"a"}`).Status, service.OutcomeResult, "allowed function")
	out := w.call(x, "s__search", `{"q":"a"}`)
	want(t, out.Status, service.OutcomeUnknownTool, "function with no agent row")
	want(t, out.Reason, port.ReasonNoAgentGrant, "reason")

	w.Runtime.SetDefinitions(listing, w.redefine("create_issue", "Creates an issue, now with labels")...)
	res := st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	want(t, len(res.Changes), 1, "changes of a re-list")
	want(t, len(w.names(x)), 0, "list after the description changed")
	want(t, w.call(x, "s__create_issue", `{}`).Reason, port.ReasonQuarantined, "quarantined")

	acc := st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "create_issue", res.Changes[0].NewHash, false))
	want(t, len(acc.StaleGrant), 1, "stale fn: rows listed by acceptFunction")
	want(t, w.call(x, "s__create_issue", `{}`).Reason, port.ReasonGrantStale, "stale row answers deny")
	want(t, len(w.names(x)), 0, "list while the row is stale")

	st.Ok(w.Svc.ConfirmGrant(w.Ctx, w.admin, acc.StaleGrant[0].ID))
	want(t, w.names(x), []string{"s__create_issue"}, "list after confirmGrant")
	want(t, w.call(x, "s__create_issue", `{}`).Status, service.OutcomePending, "the changed hash lost its reviewed class, so the destructive cap holds it")
}

func TestCaseEachUseAsksFirst(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	x := w.agent("x", "s1")
	want(t, len(w.names(x)), 4, "every function of a reviewed listing is listed")

	first := w.call(x, "s__search", `{"q":"a"}`)
	want(t, first.Status, service.OutcomePending, "first call is held")
	id := first.Approval.ID
	again := w.call(x, "s__search", `{"q":"a"}`)
	want(t, again.Status, service.OutcomePending, "a repeat attaches")
	want(t, again.Approval.ID, id, "attached to the same record")
	want(t, w.Runtime.CallsTo("search"), 0, "nothing forwarded before approval")

	w.decide(id, port.DecisionApprove)
	want(t, w.Runtime.CallsTo("search"), 0, "decide does not forward by itself")
	w.sweep()
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "executed exactly once by the worker")

	delivered := w.call(x, "s__search", `{"q":"a"}`)
	want(t, delivered.Status, service.OutcomeResult, "the first repeat after the terminal state gets the stored answer")
	want(t, w.Runtime.CallsTo("search"), 1, "answering from the record forwards nothing")
	if err := w.Svc.MarkDelivered(w.Ctx, x, id); err != nil {
		t.Fatal(err)
	}

	second := w.call(x, "s__search", `{"q":"a"}`)
	want(t, second.Status, service.OutcomePending, "after delivery a repeat is a new call")
	if second.Approval.ID == id {
		t.Fatal("a new approval was expected")
	}
}

func TestCaseReadOnlyAgent(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelWrite))
	w.bind("y")
	st.Ok(w.Svc.SetAgentLevel(w.Ctx, w.admin, w.conn.ID, "y", port.LevelRead))
	y := w.agent("y", "s1")
	want(t, w.names(y), []string{"s__search"}, "read-only list: export is unclassified so destructive")
	want(t, w.call(y, "s__create_issue", `{}`).Status, service.OutcomeUnknownTool, "write is refused for the read agent")
}

func TestCaseAskOncePerSession(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.bind("z")
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAskOnce)
	st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "z", Connector: w.conn.ID, Selector: "fn:create_issue", Mode: port.ModeAskOnce, DefinitionHash: w.hashes["create_issue"]}))
	x := w.agent("x", "s1")

	held := w.call(x, "s__create_issue", `{"q":"1"}`)
	want(t, held.Status, service.OutcomePending, "first call in the session is held")
	w.decide(held.Approval.ID, port.DecisionApproveForSession)
	w.sweep()
	want(t, w.Runtime.CallsTo("create_issue"), 1, "the held call ran")
	if err := w.Svc.MarkDelivered(w.Ctx, x, held.Approval.ID); err != nil {
		t.Fatal(err)
	}

	want(t, w.call(x, "s__create_issue", `{"q":"2"}`).Status, service.OutcomeResult, "second call in s1 is forwarded without a prompt")
	want(t, w.call(w.agent("z", "s1"), "s__create_issue", `{"q":"3"}`).Status, service.OutcomePending, "another agent in the same session is held")
	want(t, w.call(w.agent("x", "s2"), "s__create_issue", `{"q":"4"}`).Status, service.OutcomePending, "a new session is held again")
}

func TestCaseEnableAll(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, len(w.names(x)), 4, "all four listed")
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeResult, "read forwarded")
	want(t, w.call(x, "s__create_issue", `{}`).Status, service.OutcomeResult, "write forwarded")
	want(t, w.call(x, "s__delete_repo", `{}`).Status, service.OutcomePending, "destructive held by the cap")
	want(t, w.call(x, "s__export", `{}`).Status, service.OutcomePending, "unclassified held by the cap")
}

func TestCatalogCeilingCapsEnableAll(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, ceiling: port.CeilingRead})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, w.names(x), []string{"s__search"}, "tenant ceiling read leaves the reads")
	out := w.call(x, "s__create_issue", `{}`)
	want(t, out.Reason, port.ReasonOverCatalogCeiling, "a workspace cannot widen the ceiling")
}

func TestHiddenListingBlocksConnector(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	held := w.call(x, "s__delete_repo", `{}`)
	want(t, held.Status, service.OutcomePending, "held before the rule goes")
	before := len(w.Events())

	rules := st.Ok(w.Svc.ListCatalogRules(w.Ctx, w.admin))
	for _, r := range rules {
		if r.SubjectKind == port.SubjectTenant {
			if err := w.Svc.DeleteCatalogRule(w.Ctx, w.admin, r.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	types := w.Types(before)
	want(t, slices.Contains(types, "connector.connector.blocked_changed"), true, "event 36 in the admin's transaction")
	want(t, slices.Contains(types, "connector.approval.cancelled"), true, "pending approval cancelled in the same transaction")
	want(t, w.call(x, "s__search", `{}`).Reason, port.ReasonNotInCatalog, "call refused")
	want(t, len(w.names(x)), 0, "list empty")
	_, err := w.Svc.GetListing(w.Ctx, w.admin, workspace, port.VersionRef{Listing: ref, Version: "1.0.0"})
	want(t, port.IsCode(err, port.CodeNotFound), true, "getListing not found")
	views := st.Ok(w.Svc.ListConnectors(w.Ctx, w.admin, workspace))
	want(t, views[0].Blocked, port.ReasonNotInCatalog, "admins see the connector as blocked")
}

func TestTenantRowDenyOneFunction(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	w.row(port.ScopeTenant, string(tenant), "fn:delete_repo", port.ModeDeny)
	x := w.agent("x", "s1")
	want(t, w.names(x), []string{"s__create_issue", "s__export", "s__search"}, "only delete_repo is gone")
	want(t, w.call(x, "s__delete_repo", `{}`).Reason, port.ReasonDeniedByRow, "denied by the tenant row")
}

func TestAnApprovedHeldCallReturnsTheProjectedResult(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	def, _ := st.Definition("search", "Searches", `"outputSchema":{"type":"object","properties":{"name":{"type":"string"},"note":{"type":"string"}}}`)
	_, _, list := definitions("Searches", "Creates an issue")
	for i, d := range list {
		if d.Name == "search" {
			list[i] = def
		}
	}
	w.Runtime.SetDefinitions(listing, list...)
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
		return port.Result{Content: json.RawMessage(`[]`), Structured: json.RawMessage(`{"name":"Ana","note":"vip"}`)}, nil
	})
	res := st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", res.Changes[0].NewHash, false))
	st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "search", true))
	x := w.agent("x", "s1")
	req := service.CallRequest{Name: "s__search", Arguments: json.RawMessage(`{"q":"a"}`), Fields: []string{"/name"}}

	held := st.Ok(w.Svc.Call(w.Ctx, x, req))
	want(t, held.Status, service.OutcomePending, "held")
	w.decide(held.Approval.ID, port.DecisionApprove)
	w.sweep()
	w.sweep()
	got := st.Ok(w.Svc.Call(w.Ctx, x, service.CallRequest{Name: "s__search", Arguments: json.RawMessage(`{"q":"a"}`)}))
	want(t, got.Status, service.OutcomeResult, "delivered from the record")
	want(t, string(got.Result.Structured), `{"name":"Ana"}`, "the stored paths trim the approved call's result")
}
