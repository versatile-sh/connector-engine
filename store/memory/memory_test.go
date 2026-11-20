package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/store/memory"
	"github.com/agents-control/connector-engine/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(*testing.T) (port.Store, port.Outbox) {
		s := memory.New()
		return s, s
	})
}

func TestACallFromInsideACallbackPanicsInsteadOfDeadlocking(t *testing.T) {
	s := memory.New()
	ctx, scope := context.Background(), port.Scope{Tenant: "t1"}
	nested := map[string]func() error{
		"read":      func() error { return s.Read(ctx, scope, func(port.Tx) error { return nil }) },
		"write":     func() error { return s.Write(ctx, scope, func(port.Tx) error { return nil }) },
		"pending":   func() error { _, err := s.Pending(ctx, 0); return err },
		"delivered": func() error { return s.MarkDelivered(ctx, nil) },
	}
	for outer, run := range map[string]func(func(port.Tx) error) error{
		"read":  func(fn func(port.Tx) error) error { return s.Read(ctx, scope, fn) },
		"write": func(fn func(port.Tx) error) error { return s.Write(ctx, scope, fn) },
	} {
		for inner, call := range nested {
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				_ = run(func(port.Tx) error { return call() })
			}()
			select {
			case p := <-done:
				if p == nil {
					t.Errorf("%s inside %s did not panic", inner, outer)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s inside %s deadlocked", inner, outer)
			}
		}
	}
	if err := s.Write(ctx, scope, func(port.Tx) error { return nil }); err != nil {
		t.Fatalf("the store is unusable after a refused nested call: %v", err)
	}
}

func TestCallsFromOtherGoroutinesWaitTheirTurn(t *testing.T) {
	s := memory.New()
	ctx, scope := context.Background(), port.Scope{Tenant: "t1"}
	err := s.Read(ctx, scope, func(port.Tx) error {
		done := make(chan error, 1)
		go func() { done <- s.Read(ctx, scope, func(port.Tx) error { return nil }) }()
		return <-done
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadGrantsAndBindingsAreCopies(t *testing.T) {
	s := memory.New()
	ctx, scope := context.Background(), port.Scope{Tenant: "t1"}
	ref := port.ListingRef{Name: "acme.dev/s"}
	err := s.Write(ctx, scope, func(tx port.Tx) error {
		return errors.Join(tx.PutGrant(port.Grant{ID: "g", Tenant: "t1", ScopeKind: port.ScopeAgent, ScopeID: "x", Listing: &ref, Selector: "*", Mode: port.ModeAsk}),
			tx.PutBinding(port.Binding{Tenant: "t1", Connector: "c", Agent: "x", Approvers: []string{"ana"}}))
	})
	if err != nil {
		t.Fatal(err)
	}
	read := func() (port.Grant, port.Binding) {
		var g port.Grant
		var b port.Binding
		if err := s.Read(ctx, scope, func(tx port.Tx) (err error) {
			gs, err1 := tx.ListGrants(port.GrantFilter{})
			b, err = tx.GetBinding("c", "x")
			g = gs[0]
			return errors.Join(err, err1)
		}); err != nil {
			t.Fatal(err)
		}
		return g, b
	}
	g, b := read()
	g.Listing.Name, b.Approvers[0] = "changed", "changed"
	if g, b := read(); g.Listing.Name != "acme.dev/s" || b.Approvers[0] != "ana" {
		t.Fatalf("a caller changed the stored rows: %v %v", g.Listing, b.Approvers)
	}
}
