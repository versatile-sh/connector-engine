package service_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

const duplicated = `{"q":"a","q":"b"}`

func TestDuplicateKeyIsRefusedAndNeverForwarded(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	for _, args := range []string{duplicated, `{"o":{"k":1,"k":2}}`, `{"l":[{"k":1,"k":2}]}`} {
		out := w.call(x, "s__search", args)
		want(t, out.Status, service.OutcomeError, "refused: "+args)
		want(t, out.Code, port.CodeInvalidArgument, "code: "+args)
	}
	want(t, w.Runtime.CallsTo("search"), 0, "nothing reached the upstream")
	want(t, w.call(x, "s__search", `{"q":"a"}`).Status, service.OutcomeResult, "the same call without a duplicate runs")
	want(t, w.Runtime.CallsTo("search"), 1, "one forwarded call")
}

func TestDuplicateKeyOnADeniedFunctionAnswersAsDenied(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
	x := w.agent("x", "s1")
	plain := w.call(x, "s__search", `{"q":"a"}`)
	want(t, plain.Status, service.OutcomeUnknownTool, "denied")
	dup := w.call(x, "s__search", duplicated)
	want(t, [2]any{dup.Status, dup.Reason}, [2]any{plain.Status, plain.Reason}, "a duplicate key changes nothing on a denied function")
	missing := w.call(x, "s__nothing", `{"q":"a"}`)
	dup = w.call(x, "s__nothing", duplicated)
	want(t, [2]any{dup.Status, dup.Reason}, [2]any{missing.Status, missing.Reason}, "a duplicate key changes nothing on an unknown function")
	want(t, w.Runtime.CallsTo("search"), 0, "nothing reached the upstream")
}

func TestDuplicateKeyInAStoredApprovalIsNotForwarded(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{"q":"1"}`)
	approved := w.decide(held.Approval.ID, port.DecisionApprove)
	sealed := st.Ok(w.Secrets.Seal(w.Ctx, tenant, []byte(duplicated)))
	approved.Arguments = &sealed
	if err := w.Store.Write(w.Ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		return tx.UpdateApproval(approved, port.ApprovalApproved)
	}); err != nil {
		t.Fatal(err)
	}
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 0, "a duplicate key in the stored arguments is not run")
	got := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
	want(t, got.Code, port.CodeInvalidArgument, "reason")
}
