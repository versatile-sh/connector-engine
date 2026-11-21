package service_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

type failingSettle struct {
	port.Store
	code port.Code
	left atomic.Int32
	log  bytes.Buffer
}

func (f *failingSettle) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return f.Store.Write(ctx, scope, func(tx port.Tx) error {
		ft := &settleTx{Tx: tx}
		if err := fn(ft); err != nil {
			return err
		}
		if ft.settled && f.left.Add(-1) >= 0 {
			return port.Errorf(f.code, "the commit failed")
		}
		return nil
	})
}

type settleTx struct {
	port.Tx
	settled bool
}

func (t *settleTx) UpdateCall(c port.Call, from port.CallState) error {
	if from == port.CallStarted && (c.State == port.CallFinished || c.State == port.CallFailed) {
		t.settled = true
	}
	return t.Tx.UpdateCall(c, from)
}

func settleWorld(t *testing.T, code port.Code) (*world, *failingSettle) {
	f := &failingSettle{code: code}
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) {
		f.Store = d.Store
		d.Store = f
		d.Log = slog.New(slog.NewTextHandler(&f.log, nil))
	}}})
	return w, f
}

func TestASettleConflictNeverForwardsTwice(t *testing.T) {
	w, f := settleWorld(t, port.CodeConflict)
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	f.left.Store(1)
	out := w.call(x, "s__search", `{"q":"1"}`)
	want(t, w.Runtime.CallsTo("search"), 1, "a call whose result could not be recorded is not sent again")
	want(t, out.Code, service.CodeOutcomeUnknown, "the agent learns the call may have run")
}

func TestASettleCommitFailureAnswersOutcomeUnknown(t *testing.T) {
	w, f := settleWorld(t, port.CodeUnavailable)
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	f.left.Store(1)
	before := len(w.Events())
	out := w.call(x, "s__search", `{"q":"1"}`)
	want(t, out.Code, service.CodeOutcomeUnknown, "a call the upstream received is not reported as an error that did not run")
	want(t, w.Runtime.CallsTo("search"), 1, "forwarded once")
	w.Clock.Advance(71 * time.Second)
	w.sweep()
	want(t, w.Types(before), []string{"connector.call.started", "connector.call.outcome_unknown"}, "the sweep records the unknown outcome")
}

func TestAFailureToRecordAForwardedCallIsLogged(t *testing.T) {
	w, f := settleWorld(t, port.CodeUnavailable)
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	f.left.Store(1)
	before := len(w.Events())
	w.call(x, "s__search", `{"q":"kept-private"}`)
	line := f.log.String()
	call := w.Events()[before].Subject
	want(t, strings.Contains(line, "call="+call) && strings.Contains(line, "the commit failed"), true, "the log names the call and the failure: "+line)
	want(t, strings.Contains(line, "kept-private"), false, "the log carries no argument")
}

func TestAnApprovedCallWhoseResultCouldNotBeRecordedRunsOnce(t *testing.T) {
	w, f := settleWorld(t, port.CodeUnavailable)
	w.acceptAll()
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	x := w.agent("x", "s1")
	held := w.call(x, "s__search", `{"q":"1"}`)
	w.decide(held.Approval.ID, port.DecisionApprove)
	f.left.Store(1)
	_, _ = w.Worker.Sweep(w.Ctx)
	w.Clock.Advance(10 * time.Second)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "forwarded once")
	w.Clock.Advance(71 * time.Second)
	w.sweep()
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "never forwarded again")
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Code, service.CodeOutcomeUnknown, "the answer says the call may have run")
	want(t, w.call(x, "s__search", `{"q":"1"}`).Code, service.CodeOutcomeUnknown, "a repeat answers from the record")
	want(t, w.Runtime.CallsTo("search"), 1, "the repeat is not forwarded")
}
