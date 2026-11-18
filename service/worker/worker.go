// Package worker drives the timers of §8.3, the sweep of §10.3 C12, the periodic re-list of §9 and the fan-outs of §4.4 and §5.5.
package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
)

// Config holds approval.sweepInterval, the size of one batch of due work and how many approved records run at once.
type Config struct {
	Interval  time.Duration
	Batch     int
	Executors int
}

// Stats counts what one sweep did.
type Stats struct {
	Items   int
	Relists int
	FanOuts int
}

const maxBackoff = 5 * time.Minute

type retry struct {
	fails int
	next  time.Time
}

// Worker finds due work through the one cross-tenant view and does each item in a transaction of its tenant; every item is re-checked by the service before it acts.
type Worker struct {
	svc     *service.Service
	store   port.Store
	clock   port.Clock
	cfg     Config
	slots   chan struct{}
	mu      sync.Mutex
	retries map[string]retry
	running map[string]bool
}

// New returns a Worker over a Service, the store it reads due work from and a clock.
func New(svc *service.Service, store port.Store, clock port.Clock, cfg Config) *Worker {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Batch == 0 {
		cfg.Batch = 100
	}
	if cfg.Executors == 0 {
		cfg.Executors = 8
	}
	return &Worker{svc: svc, store: store, clock: clock, cfg: cfg, slots: make(chan struct{}, cfg.Executors), retries: map[string]retry{}, running: map[string]bool{}}
}

// Run sweeps every Interval until ctx ends without waiting for the approved records it started; a failed item backs off.
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		_, _ = w.sweep(ctx, &wg, nil)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Sweep runs one pass: due work, the re-list of due connectors and credentials, and the pending fan-outs; it waits for the approved records it started.
func (w *Worker) Sweep(ctx context.Context) (Stats, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var async []error
	st, err := w.sweep(ctx, &wg, func(e error) {
		mu.Lock()
		async = append(async, e)
		mu.Unlock()
	})
	wg.Wait()
	return st, errors.Join(append(async, err)...)
}

func (w *Worker) sweep(ctx context.Context, wg *sync.WaitGroup, report func(error)) (Stats, error) {
	var st Stats
	var items []port.WorkItem
	var tenants []port.Tenant
	var fanouts []port.FanOut
	now := w.clock.Now()
	err := w.store.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) (err error) {
		if items, err = tx.DueWork(now, w.cfg.Batch+w.waiting(now)); err != nil {
			return err
		}
		if tenants, err = tx.ListTenants(); err != nil {
			return err
		}
		fanouts, err = tx.ListFanOuts()
		return err
	})
	if err != nil {
		return st, err
	}
	var errs []error
	for _, it := range items {
		if st.Items == w.cfg.Batch {
			break
		}
		key := string(it.Kind) + "|" + it.ID
		if !w.ready(key, now) {
			continue
		}
		st.Items++
		if it.Kind == port.WorkApprovedRecord {
			w.execute(ctx, wg, key, it, report)
			continue
		}
		err := w.dispatch(ctx, it)
		w.settle(key, err)
		errs = append(errs, err)
	}
	for _, t := range tenants {
		if t.State != port.TenantActive {
			continue
		}
		due, err := w.svc.RelistDue(ctx, t.ID)
		st.Relists += len(due)
		errs = append(errs, err, w.svc.RefreshDueCredentials(ctx, t.ID))
	}
	for _, f := range fanouts {
		if f.Done {
			continue
		}
		st.FanOuts++
		errs = append(errs, w.fanOut(ctx, f))
	}
	return st, errors.Join(errs...)
}

func (w *Worker) waiting(now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, r := range w.retries {
		if now.Sub(r.next) > 2*maxBackoff {
			delete(w.retries, k)
		}
	}
	return len(w.retries) + len(w.running)
}

func (w *Worker) ready(key string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.running[key] && !now.Before(w.retries[key].next)
}

func (w *Worker) settle(key string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err == nil {
		delete(w.retries, key)
		return
	}
	r := w.retries[key]
	r.fails++
	r.next = w.clock.Now().Add(min(w.cfg.Interval<<min(r.fails-1, 16), maxBackoff))
	w.retries[key] = r
}

func (w *Worker) execute(ctx context.Context, wg *sync.WaitGroup, key string, it port.WorkItem, report func(error)) {
	select {
	case w.slots <- struct{}{}:
	default:
		return
	}
	w.mu.Lock()
	w.running[key] = true
	w.mu.Unlock()
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.dispatch(ctx, it)
		w.mu.Lock()
		delete(w.running, key)
		w.mu.Unlock()
		w.settle(key, err)
		<-w.slots
		if report != nil && err != nil {
			report(err)
		}
	}()
}

func (w *Worker) dispatch(ctx context.Context, it port.WorkItem) error {
	switch it.Kind {
	case port.WorkApprovalExpiry:
		return w.svc.ExpireApproval(ctx, it.Tenant, port.ApprovalID(it.ID))
	case port.WorkApprovedRecord:
		return w.svc.ExecuteApproved(ctx, it.Tenant, port.ApprovalID(it.ID))
	case port.WorkExecuteDeadline:
		return w.svc.MarkExecutionUnknown(ctx, it.Tenant, port.ApprovalID(it.ID))
	case port.WorkResultRetention:
		return w.svc.RetainApproval(ctx, it.Tenant, port.ApprovalID(it.ID))
	case port.WorkCallDeadline:
		return w.svc.SweepCall(ctx, it.Tenant, port.CallID(it.ID))
	case port.WorkGrantExpiry:
		return w.svc.ExpireGrant(ctx, it.Tenant, port.GrantID(it.ID))
	case port.WorkPauseExpiry:
		return w.svc.ExpirePauses(ctx, it.Tenant)
	}
	return nil
}

func (w *Worker) fanOut(ctx context.Context, f port.FanOut) error {
	if f.Kind == service.RemoveTenantKind {
		return w.svc.RemoveTenantData(ctx, f)
	}
	var errs []error
	for _, t := range f.Pending {
		errs = append(errs, w.svc.FanOutTenant(ctx, f, t))
	}
	return errors.Join(errs...)
}
