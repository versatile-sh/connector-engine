package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

type lockStep struct {
	port.Store
	armed     atomic.Bool
	deadlocks atomic.Int32
	at        map[string]bool
	mu        sync.Mutex
	arrived   int
	both      chan struct{}
}

func (l *lockStep) arm(at ...string) {
	l.mu.Lock()
	l.arrived, l.both, l.at = 0, make(chan struct{}), map[string]bool{}
	for _, m := range at {
		l.at[m] = true
	}
	l.mu.Unlock()
	l.armed.Store(true)
}

func (l *lockStep) pausesAt(method string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.at[method]
}

func (l *lockStep) arrive() {
	l.mu.Lock()
	l.arrived++
	both := l.both
	if l.arrived == 2 {
		close(both)
	}
	l.mu.Unlock()
	select {
	case <-both:
	case <-time.After(300 * time.Millisecond):
	}
}

func (l *lockStep) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	err := l.Store.Write(ctx, scope, func(tx port.Tx) error { return fn(&stepTx{Tx: tx, l: l}) })
	if err != nil && strings.Contains(err.Error(), "deadlock") {
		l.deadlocks.Add(1)
	}
	return err
}

type stepTx struct {
	port.Tx
	l     *lockStep
	wrote bool
}

func (t *stepTx) step(method string, err error) error {
	if err == nil && !t.wrote && t.l.armed.Load() && t.l.pausesAt(method) {
		t.wrote = true
		t.l.arrive()
	}
	return err
}

func (t *stepTx) UpdateCall(c port.Call, from port.CallState) error {
	return t.step("UpdateCall", t.Tx.UpdateCall(c, from))
}

func (t *stepTx) UpdateApproval(a port.Approval, from port.ApprovalState) error {
	return t.step("UpdateApproval", t.Tx.UpdateApproval(a, from))
}

func (t *stepTx) ReserveGrants(ids []port.GrantID) error {
	return t.step("ReserveGrants", t.Tx.ReserveGrants(ids))
}

func (t *stepTx) DeleteGrant(id port.GrantID) error {
	return t.step("DeleteGrant", t.Tx.DeleteGrant(id))
}

func (t *stepTx) Emit(e port.Event) (port.Event, error) {
	e, err := t.Tx.Emit(e)
	return e, t.step("Emit", err)
}

func lockWorld(t *testing.T, tune ...func(*service.Config, *service.Deps)) (*lockStep, *world) {
	step := &lockStep{}
	tune = append(tune, func(_ *service.Config, d *service.Deps) {
		step.Store = d.Store
		d.Store = step
	})
	w := newWorld(t, opts{reviewed: true, store: postgresStore(t), tune: tune})
	w.acceptAll()
	return step, w
}

func (l *lockStep) together(t *testing.T, at []string, first, second func() error) {
	t.Helper()
	l.arm(at...)
	done := make(chan error, 1)
	go func() { done <- first() }()
	errs := []error{second(), <-done}
	l.armed.Store(false)
	if n := l.deadlocks.Load(); n > 0 {
		t.Fatalf("%d transactions were aborted on a deadlock: %v", n, errs)
	}
}

func (w *world) admitted(x port.Principal, name string) func() error {
	return func() error {
		_, err := w.Svc.Call(w.Ctx, x, service.CallRequest{Name: name, Arguments: json.RawMessage(`{"q":"a"}`)})
		return err
	}
}

var admitting = []string{"ReserveGrants", "Emit"}

func writeAllowed(c *service.Config, _ *service.Deps) { c.LevelModes.Write = port.ModeAllow }

func TestSettingALevelWhileACallIsAdmittedNeverDeadlocks(t *testing.T) {
	step, w := lockWorld(t, writeAllowed)
	st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelWrite))
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAllow)
	x := w.agent("x", "s1")
	want(t, w.call(x, "s__create_issue", `{"q":"a"}`).Status, service.OutcomeResult, "warm call")
	step.together(t, admitting, func() error {
		_, err := w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelRead)
		return err
	}, w.admitted(x, "s__create_issue"))
}

func TestUnbindWhileACallIsAdmittedNeverDeadlocks(t *testing.T) {
	step, w := lockWorld(t)
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, w.call(x, "s__search", `{"q":"a"}`).Status, service.OutcomeResult, "warm call")
	step.together(t, admitting, func() error { return w.Svc.DeleteBinding(w.Ctx, w.admin, w.conn.ID, "x") }, w.admitted(x, "s__search"))
}

func TestRemovingAConnectorWhileACallIsAdmittedNeverDeadlocks(t *testing.T) {
	step, w := lockWorld(t)
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, w.call(x, "s__search", `{"q":"a"}`).Status, service.OutcomeResult, "warm call")
	step.together(t, admitting, func() error { return w.Svc.RemoveConnector(w.Ctx, w.admin, w.conn.ID) }, w.admitted(x, "s__search"))
}

func TestExpiringASessionRowWhileAnApprovalReplacesItNeverDeadlocks(t *testing.T) {
	step, w := lockWorld(t)
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAskOnce)
	x := w.agent("x", "s1")
	first := w.call(x, "s__search", `{"q":"a"}`)
	st.Ok(w.Svc.DecideApproval(w.Ctx, w.alice, first.Approval.ID, port.DecisionApproveForSession, "", "test"))
	rows := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeSession, ScopeID: "s1"}))
	want(t, len(rows), 1, "session rows")
	w.Clock.Advance(9 * time.Hour)
	second := w.call(x, "s__search", `{"q":"b"}`)
	want(t, second.Status, service.OutcomePending, "held after the session row expired")
	step.together(t, []string{"DeleteGrant", "Emit"}, func() error { return w.Svc.ExpireGrant(w.Ctx, tenant, rows[0].ID) }, func() error {
		_, err := w.Svc.DecideApproval(w.Ctx, w.alice, second.Approval.ID, port.DecisionApproveForSession, "", "test")
		return err
	})
}

func TestExpiringAnApprovalWhileItsBindingIsRemovedNeverDeadlocks(t *testing.T) {
	step, w := lockWorld(t)
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: "x", AskRequester: true, ApprovalTimeout: time.Minute}))
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	x := w.agent("x", "s1")
	held := w.call(x, "s__search", `{"q":"a"}`)
	want(t, held.Status, service.OutcomePending, "held")
	w.Clock.Advance(2 * time.Minute)
	step.together(t, []string{"UpdateApproval", "Emit"}, func() error { return w.Svc.ExpireApproval(w.Ctx, tenant, held.Approval.ID) },
		func() error { return w.Svc.DeleteBinding(w.Ctx, w.admin, w.conn.ID, "x") })
}

func TestMarkingAnExecutionUnknownWhileItSettlesNeverDeadlocks(t *testing.T) {
	step := &lockStep{}
	w := newWorld(t, opts{reviewed: true, store: postgresStore(t), tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) {
		step.Store = d.Store
		d.Store = step
	}}})
	w.acceptAll()
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	x := w.agent("x", "s1")
	var id port.ApprovalID
	marked := make(chan error, 1)
	w.Runtime.Handle("search", func(args json.RawMessage) (port.Result, error) {
		w.Clock.Advance(5 * time.Minute)
		step.arm("UpdateCall", "UpdateApproval")
		go func() { marked <- w.Svc.MarkExecutionUnknown(context.Background(), tenant, id) }()
		return port.Result{Content: json.RawMessage(`[{"type":"text","text":"ok"}]`), Structured: args}, nil
	})
	for i := range 5 {
		held := w.call(x, "s__search", fmt.Sprintf(`{"q":"%d"}`, i))
		w.decide(held.Approval.ID, port.DecisionApprove)
		id = held.Approval.ID
		settled := w.Svc.ExecuteApproved(w.Ctx, tenant, id)
		unknown := <-marked
		step.armed.Store(false)
		for _, err := range []error{settled, unknown} {
			if err != nil && strings.Contains(err.Error(), "deadlock") {
				t.Fatalf("run %d: the two writes of one execution deadlocked: %v", i, err)
			}
		}
	}
}
