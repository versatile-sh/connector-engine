// Package worker drives the timers of §8.3, the sweep of §10.3 C12, the periodic re-list of §9 and the fan-outs of §4.4 and §5.5.
package worker

import (
	"context"
	"errors"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
)

// Config holds approval.sweepInterval and the size of one batch of due work.
type Config struct {
	Interval time.Duration
	Batch    int
}

// Stats counts what one sweep did.
type Stats struct {
	Items   int
	Relists int
	FanOuts int
}

// Worker finds due work through the one cross-tenant view and does each item in a transaction of its tenant; every item is re-checked by the service before it acts.
type Worker struct {
	svc   *service.Service
	store port.Store
	clock port.Clock
	cfg   Config
}

// New returns a Worker over a Service, the store it reads due work from and a clock.
func New(svc *service.Service, store port.Store, clock port.Clock, cfg Config) *Worker {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Batch == 0 {
		cfg.Batch = 100
	}
	return &Worker{svc: svc, store: store, clock: clock, cfg: cfg}
}

// Run sweeps every Interval until ctx ends; a failed sweep is retried at the next tick.
func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		_, _ = w.Sweep(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Sweep runs one pass: due approvals, calls, grants and pauses, the re-list of due connectors and credentials, and the pending fan-outs.
func (w *Worker) Sweep(ctx context.Context) (Stats, error) {
	var st Stats
	var items []port.WorkItem
	var tenants []port.Tenant
	var fanouts []port.FanOut
	err := w.store.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) (err error) {
		if items, err = tx.DueWork(w.clock.Now(), w.cfg.Batch); err != nil {
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
		st.Items++
		errs = append(errs, w.dispatch(ctx, it))
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
