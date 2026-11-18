package service_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

type allowing struct{ port.Authorizer }

func (allowing) Evaluate(context.Context, port.Query) (port.AccessDecision, error) {
	return port.AccessDecision{Mode: port.ModeAllow}, nil
}

func auditActions(w *world, subject string) []string {
	var out []string
	w.Read(tenant, func(tx port.Tx) {
		rows, _ := tx.ListAudit(subject)
		for _, r := range rows {
			out = append(out, r.Action)
		}
	})
	return out
}

func TestExpiryAndCancelWriteTheirAuditRows(t *testing.T) {
	w, x := askWorld(t)
	expired := w.call(x, "s__search", `{"q":"1"}`)
	w.Clock.Advance(601 * time.Second)
	w.sweep()
	want(t, slices.Contains(auditActions(w, string(expired.Approval.ID)), "connector.approval.expired"), true, "A6 audit row")
	cancelled := w.call(x, "s__search", `{"q":"2"}`)
	st.Ok(w.Svc.ApprovalCancelByAgent(w.Ctx, x, cancelled.Approval.ID))
	want(t, slices.Contains(auditActions(w, string(cancelled.Approval.ID)), "connector.approval.cancelled"), true, "A7 audit row")
}

func TestCallDeniedFollowsSection12(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) { d.Authorizer = allowing{d.Authorizer} }}})
	w.acceptAll()
	w.customWorkspace()
	w.bind("x")
	x := w.agent("x", "s1")
	before := len(w.Events())
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeUnknownTool, "the in-transaction evaluation denies")
	want(t, w.Types(before), []string{"connector.call.denied"}, "the deny inside the transaction is event 22")
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.row(port.ScopeAgent, "x", "*", port.ModeAllow)
	before = len(w.Events())
	want(t, w.call(x, "s__search", `{"q":1}`).Code, port.CodeInvalidArgument, "C5 refuses the arguments")
	want(t, slices.Contains(w.Types(before), "connector.call.denied"), false, "C5 is not event 22")
}
