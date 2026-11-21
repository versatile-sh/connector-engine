package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func TestTheQueriesRowLevelSecurityFilteredInPostgresStayScoped(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "store.db"))
	a, b := port.Scope{Tenant: "ta"}, port.Scope{Tenant: "tb"}
	t0 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if err := s.Write(ctx, a, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "x", Tenant: "ta", Time: t0})
		_, err2 := tx.MarkInbox("m", t0)
		return errors.Join(err, err2, tx.PutGrant(port.Grant{ID: "g1", Tenant: "ta", ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "c1", Selector: "*", Mode: port.ModeAllow, MaxUses: 3, CreatedAt: t0}),
			tx.PutPause(port.Pause{ID: "p1", Tenant: "ta", Session: "s", Agent: "x", Connector: "c1", Paused: true, ExpiresAt: t0.Add(time.Hour)}),
			tx.Audit(port.AuditRow{At: t0, Tenant: "ta", Actor: "u", Action: "x", Subject: "s"}))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, b, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("tenant tb reserved a grant of ta: %v", err)
	}
	if err := s.Write(ctx, b, func(tx port.Tx) error { return tx.DeletePause("p1") }); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("tenant tb deleted a pause of ta: %v", err)
	}
	if err := s.Read(ctx, b, func(tx port.Tx) error {
		events, err1 := tx.EventsAfter(port.SourceOf("ta"), 0, 0)
		audit, err2 := tx.ListAudit("")
		seen, err3 := tx.SeenInbox("m")
		if len(events)+len(audit) != 0 || seen {
			t.Fatalf("tenant tb reads %d events, %d audit rows and the inbox of ta", len(events), len(audit))
		}
		return errors.Join(err1, err2, err3)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(ctx, a, func(tx port.Tx) error {
		g, err := tx.GetGrant("g1")
		_, paused, err2 := tx.GetPause("s", "x", "c1")
		if g.Uses != 0 || !paused {
			t.Fatalf("tenant ta's rows changed: uses %d, paused %v", g.Uses, paused)
		}
		return errors.Join(err, err2)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestATenantNamedLikeTheInstallationCannotAdvanceItsStream(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "store.db"))
	if err := s.Write(ctx, port.Scope{Installation: true}, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "x", Time: time.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := s.Write(ctx, port.Scope{Tenant: "installation"}, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "x", Tenant: "installation", Time: time.Now()})
		return err
	})
	if !port.IsCode(err, port.CodePermissionDenied) {
		t.Fatalf("a tenant wrote into the installation stream: %v", err)
	}
}
