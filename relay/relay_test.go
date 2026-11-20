package relay_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/relay"
	sinkmem "github.com/agents-control/connector-engine/sink/memory"
	"github.com/agents-control/connector-engine/store/memory"
)

var (
	ctx = context.Background()
	t0  = time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
)

func emit(t *testing.T, s *memory.Store, tenant port.TenantID, types ...string) {
	t.Helper()
	scope := port.Scope{Tenant: tenant, Installation: tenant == port.Installation}
	err := s.Write(ctx, scope, func(tx port.Tx) error {
		for _, typ := range types {
			if _, err := tx.Emit(port.Event{Type: typ, Tenant: tenant, Time: t0}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func types(events []port.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

func pendingTypes(t *testing.T, s *memory.Store) []string {
	t.Helper()
	p, err := s.Pending(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	return types(p)
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFlushDeliversInOrderInBatchesAndMarksDelivered(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1", "a2", "a3")
	emit(t, s, port.Installation, "i1")
	emit(t, s, "tb", "b1")
	n, err := relay.Relay{Outbox: s, Sink: sink, Batch: 2}.Flush(ctx)
	if err != nil || n != 5 {
		t.Fatalf("flush: %d %v", n, err)
	}
	equal(t, types(sink.Events()), []string{"a1", "a2", "a3", "i1", "b1"})
	equal(t, pendingTypes(t, s), nil)
	if n, err := (relay.Relay{Outbox: s, Sink: sink}).Flush(ctx); err != nil || n != 0 {
		t.Fatalf("second flush: %d %v", n, err)
	}
}

func TestFailedDeliveryLeavesTheEventsPending(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1", "a2")
	sink.FailWith(func([]port.Event) error { return port.Errorf(port.CodeUnavailable, "down") })
	r := relay.Relay{Outbox: s, Sink: sink}
	if _, err := r.Flush(ctx); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("got %v", err)
	}
	equal(t, pendingTypes(t, s), []string{"a1", "a2"})
	sink.FailWith(nil)
	if n, err := r.Flush(ctx); err != nil || n != 2 {
		t.Fatalf("recovered flush: %d %v", n, err)
	}
	equal(t, pendingTypes(t, s), nil)
}

func TestPermanentRejectionGivesTheEventUpAndGoesOn(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1", "bad", "a3")
	sink.FailWith(func(events []port.Event) error {
		for _, e := range events {
			if e.Type == "bad" {
				return &port.Rejection{Event: e.ID, Detail: "status 400"}
			}
		}
		return nil
	})
	n, err := relay.Relay{Outbox: s, Sink: sink, Store: s, Now: func() time.Time { return t0 }}.Flush(ctx)
	if err != nil || n != 3 {
		t.Fatalf("flush: %d %v", n, err)
	}
	equal(t, types(sink.Events()), []string{"a3"})
	equal(t, pendingTypes(t, s), nil)
	err = s.Read(ctx, port.Scope{Tenant: "ta"}, func(tx port.Tx) error {
		rows, err := tx.ListAudit("")
		if len(rows) != 1 || rows[0].Action != "outbox.rejected" || rows[0].Detail["type"] != "bad" || rows[0].Tenant != "ta" {
			t.Fatalf("audit: %+v", rows)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRejectionOfAnUnknownEventIsAFailure(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1")
	sink.FailWith(func([]port.Event) error { return &port.Rejection{Event: "nope"} })
	if _, err := (relay.Relay{Outbox: s, Sink: sink}).Flush(ctx); err == nil {
		t.Fatal("a rejection naming no event of the batch settled it")
	}
	equal(t, pendingTypes(t, s), []string{"a1"})
}

func TestRunBacksOffExponentiallyAndRecovers(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1")
	failures := 3
	sink.FailWith(func([]port.Event) error {
		if failures > 0 {
			failures--
			return errors.New("down")
		}
		return nil
	})
	var waits []time.Duration
	runCtx, stop := context.WithCancel(ctx)
	r := relay.Relay{Outbox: s, Sink: sink, Every: time.Second, MaxBackoff: 3 * time.Second, Sleep: func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(sink.Events()) == 1 {
			stop()
			return runCtx.Err()
		}
		return nil
	}}
	r.Run(runCtx)
	want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, time.Second}
	if len(waits) != len(want) {
		t.Fatalf("waits %v, want %v", waits, want)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits %v, want %v", waits, want)
		}
	}
	equal(t, types(sink.Events()), []string{"a1"})
}

func TestRunSurvivesASinkThatPanics(t *testing.T) {
	s, sink := memory.New(), sinkmem.New()
	emit(t, s, "ta", "a1")
	panics := 1
	sink.FailWith(func([]port.Event) error {
		if panics > 0 {
			panics--
			panic("sink bug")
		}
		return nil
	})
	runCtx, stop := context.WithCancel(ctx)
	r := relay.Relay{Outbox: s, Sink: sink, Sleep: func(context.Context, time.Duration) error {
		if len(sink.Events()) == 1 {
			stop()
			return runCtx.Err()
		}
		return nil
	}}
	r.Run(runCtx)
	equal(t, types(sink.Events()), []string{"a1"})
}
