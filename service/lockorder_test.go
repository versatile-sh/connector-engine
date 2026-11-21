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
)

type lockStep struct {
	port.Store
	armed   atomic.Bool
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func (l *lockStep) arm() {
	l.mu.Lock()
	l.arrived, l.both = 0, make(chan struct{})
	l.mu.Unlock()
	l.armed.Store(true)
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
	return l.Store.Write(ctx, scope, func(tx port.Tx) error { return fn(&stepTx{Tx: tx, l: l}) })
}

type stepTx struct {
	port.Tx
	l     *lockStep
	wrote bool
}

func (t *stepTx) step(err error) error {
	if err == nil && !t.wrote && t.l.armed.Load() {
		t.wrote = true
		t.l.arrive()
	}
	return err
}

func (t *stepTx) UpdateCall(c port.Call, from port.CallState) error {
	return t.step(t.Tx.UpdateCall(c, from))
}

func (t *stepTx) UpdateApproval(a port.Approval, from port.ApprovalState) error {
	return t.step(t.Tx.UpdateApproval(a, from))
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
		step.arm()
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
