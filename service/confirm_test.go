package service_test

import (
	"context"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

type interleaved struct {
	port.Store
	before func()
}

func (s *interleaved) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	if f := s.before; f != nil {
		s.before = nil
		f()
	}
	return s.Store.Write(ctx, scope, fn)
}

func TestConfirmingAGrantKeepsAConcurrentChange(t *testing.T) {
	is := &interleaved{}
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) {
		is.Store, d.Store = d.Store, is
	}}})
	row := func() port.Grant {
		return st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(workspace), Connector: w.conn.ID,
			Selector: "fn:search", Mode: port.ModeAllow, MaxUses: 3, DefinitionHash: "stale"})).Grant
	}
	stored := func(id port.GrantID) []port.Grant {
		var out []port.Grant
		for _, g := range st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{})) {
			if g.ID == id {
				out = append(out, g)
			}
		}
		return out
	}

	g := row()
	is.before = func() {
		if err := is.Store.Write(w.Ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{g.ID}) }); err != nil {
			t.Fatal(err)
		}
	}
	st.Ok(w.Svc.ConfirmGrant(w.Ctx, w.admin, g.ID))
	got := stored(g.ID)
	want(t, len(got), 1, "the confirmed row is kept")
	want(t, got[0].Uses, 1, "a use consumed while the row was being confirmed is kept")
	if err := w.Svc.DeleteGrant(w.Ctx, w.admin, g.ID); err != nil {
		t.Fatal(err)
	}

	g = row()
	is.before = func() {
		if err := w.Svc.DeleteGrant(w.Ctx, w.admin, g.ID); err != nil {
			t.Fatal(err)
		}
	}
	_, err := w.Svc.ConfirmGrant(w.Ctx, w.admin, g.ID)
	want(t, port.IsCode(err, port.CodeNotFound), true, "a row revoked while it was being confirmed is not confirmed")
	want(t, len(stored(g.ID)), 0, "a revoked row is not written back")
}
