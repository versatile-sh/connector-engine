// Package relay moves outbox rows to an EventSink at least once, in order, with backoff and a give-up on permanent rejection.
package relay

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/internal/safe"
	"github.com/versatile-sh/connector-engine/port"
)

// Relay reads Outbox.Pending, delivers through Sink and marks what was delivered.
type Relay struct {
	Outbox port.Outbox
	Sink   port.EventSink
	// Store, when set, receives an audit row for each event given up.
	Store port.Store
	// Batch is the most events per delivery; 100 by default.
	Batch int
	// Every is the poll interval once the outbox is empty; one second by default.
	Every time.Duration
	// MaxBackoff caps the doubling wait after a failure; one minute by default.
	MaxBackoff time.Duration
	Log        *slog.Logger
	Now        func() time.Time
	Sleep      func(ctx context.Context, d time.Duration) error
}

// Flush delivers pending events until the outbox is empty or a delivery fails, and returns how many it settled.
func (r Relay) Flush(ctx context.Context) (int, error) {
	batch := r.batch()
	settled := 0
	for {
		events, err := r.Outbox.Pending(ctx, batch)
		if err != nil || len(events) == 0 {
			return settled, err
		}
		done, err := r.deliver(ctx, events)
		if len(done) > 0 {
			ids := make([]string, len(done))
			for i, e := range done {
				ids[i] = e.ID
			}
			if merr := r.Outbox.MarkDelivered(ctx, ids); merr != nil {
				return settled, merr
			}
			settled += len(done)
		}
		if err != nil {
			return settled, err
		}
	}
}

func (r Relay) deliver(ctx context.Context, events []port.Event) ([]port.Event, error) {
	err := r.Sink.Deliver(ctx, events)
	if err == nil {
		return events, nil
	}
	var rej *port.Rejection
	if !errors.As(err, &rej) {
		return nil, err
	}
	n := slices.IndexFunc(events, func(e port.Event) bool { return e.ID == rej.Event }) + 1
	if n == 0 {
		return nil, err
	}
	r.giveUp(ctx, events[n-1], rej.Detail)
	return events[:n], nil
}

func (r Relay) giveUp(ctx context.Context, e port.Event, detail string) {
	r.log().Warn("outbox event rejected for good", "event", e.ID, "type", e.Type, "detail", detail)
	if r.Store == nil {
		return
	}
	scope := port.Scope{Tenant: e.Tenant, Installation: e.Tenant == port.Installation}
	err := r.Store.Write(ctx, scope, func(tx port.Tx) error {
		return tx.Audit(port.AuditRow{At: r.now(), Tenant: e.Tenant, Actor: "engine", Action: "outbox.rejected", Subject: e.ID,
			Detail: map[string]any{"type": e.Type, "detail": detail}})
	})
	if err != nil {
		r.log().Warn("audit of a rejected event failed", "event", e.ID, "error", err)
	}
}

// Run flushes until ctx ends: it polls every Every while the outbox is empty and doubles its wait after each failure or recovered panic.
func (r Relay) Run(ctx context.Context) {
	every, maxBackoff := r.every(), r.maxBackoff()
	wait := every
	for ctx.Err() == nil {
		if err := safe.Run(func() error { _, err := r.Flush(ctx); return err }); err != nil && ctx.Err() == nil {
			r.log().Warn("outbox relay failed", "error", err, "retry_in", wait)
			if r.sleep(ctx, wait) != nil {
				return
			}
			wait = min(wait*2, maxBackoff)
			continue
		}
		wait = every
		if r.sleep(ctx, every) != nil {
			return
		}
	}
}

func (r Relay) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r Relay) batch() int                { return cmp.Or(r.Batch, 100) }
func (r Relay) every() time.Duration      { return cmp.Or(r.Every, time.Second) }
func (r Relay) maxBackoff() time.Duration { return cmp.Or(r.MaxBackoff, time.Minute) }

func (r Relay) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r Relay) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}
