package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
)

type writerProbe struct {
	port.Authorizer
	store port.Store
	armed atomic.Bool
}

func (w *writerProbe) write(ctx context.Context) error {
	if !w.armed.Load() {
		return nil
	}
	return w.store.Write(ctx, port.Scope{Tenant: tenant}, func(port.Tx) error { return nil })
}

func (w *writerProbe) Allowed(ctx context.Context, p port.Permission) (bool, error) {
	if err := w.write(ctx); err != nil {
		return false, err
	}
	return w.Authorizer.Allowed(ctx, p)
}

func (w *writerProbe) Groups(ctx context.Context, t port.TenantID, p port.PersonID) ([]port.GroupID, error) {
	if err := w.write(ctx); err != nil {
		return nil, err
	}
	return w.Authorizer.Groups(ctx, t, p)
}

func TestTheAuthorizerIsNeverAskedInsideAStoreRead(t *testing.T) {
	probe := &writerProbe{}
	w := newWorld(t, opts{tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) {
		probe.Authorizer, probe.store = d.Authorizer, d.Store
		d.Authorizer = probe
	}}})
	probe.armed.Store(true)
	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"ListConnectors", func() error { _, err := w.Svc.ListConnectors(w.Ctx, w.alice, ""); return err }},
		{"GetConnector", func() error { _, err := w.Svc.GetConnector(w.Ctx, w.alice, w.conn.ID); return err }},
		{"ListListings", func() error {
			_, err := w.Svc.ListListings(w.Ctx, w.alice, workspace, service.ListingFilter{})
			return err
		}},
		{"GetListing", func() error {
			_, err := w.Svc.GetListing(w.Ctx, w.alice, workspace, port.VersionRef{Listing: ref, Version: "1.0.0"})
			return err
		}},
		{"InstallConnector", func() error {
			_, err := w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "s2", Listing: ref})
			return err
		}},
		{"UpdateConnector", func() error { _, err := w.Svc.UpdateConnector(w.Ctx, w.admin, w.conn.ID, "1.0.0"); return err }},
	} {
		name, run := c.name, c.run
		done := make(chan error, 1)
		go func() { done <- run() }()
		select {
		case err := <-done:
			if err != nil && !port.IsCode(err, port.CodeNotFound) {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s deadlocked: a write waited while the service asked the authorizer inside a store read", name)
		}
	}
}
