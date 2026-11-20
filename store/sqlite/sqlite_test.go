package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/store/postgres"
	"github.com/versatile-sh/connector-engine/store/sqlite"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

var (
	ctx = context.Background()
	pg  *storetest.Postgres
	t0  = time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
)

func TestMain(m *testing.M) {
	var err error
	if pg, err = storetest.StartPostgres(); err != nil {
		fmt.Fprintln(os.Stderr, "postgres migration test skipped:", err)
	}
	code := m.Run()
	if pg != nil {
		pg.Stop()
	}
	os.Exit(code)
}

func open(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) (port.Store, port.Outbox) {
		s := open(t, filepath.Join(t.TempDir(), "store.db"))
		return s, s
	})
}

func TestOpenSetsDurablePragmasOnPathsWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Application Support", "connector engine.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	s := open(t, path)
	s.Close()
	s = open(t, path)
	if err := s.Write(ctx, port.Scope{Tenant: "ta"}, func(tx port.Tx) error {
		_, err := tx.PutConnector(port.Connector{ID: "c1", Tenant: "ta"}, port.AnyRevision)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the database is not at the path with spaces: %v", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	var sync, busy int
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA synchronous`).Scan(&sync); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || sync != 2 || busy != 5000 {
		t.Fatalf("journal_mode=%s synchronous=%d busy_timeout=%d", mode, sync, busy)
	}
}

const postgresSnapshot = `{"store": "postgres", "tables": {
  "records": [
    {"kind": "tenant", "k": "local", "tenant_id": "local", "uniq": null, "rev": 1, "doc": {"id": "local", "state": "active", "created_at": "2026-11-18T03:04:05+00:00"}},
    {"kind": "connector", "k": "c1", "tenant_id": "local", "uniq": null, "rev": 3, "doc": {"id": "c1", "tenant": "local", "workspace": "default", "alias": "notes", "enabled": true}},
    {"kind": "grant", "k": "g1", "tenant_id": "local", "uniq": "abc", "rev": 1, "doc": {"id": "g1", "tenant": "local", "scope_kind": "agent", "scope_id": "x", "connector": "c1", "expires_at": "2026-11-18T04:04:05.5+00:00"}},
    {"kind": "approval", "k": "a1", "tenant_id": "local", "uniq": "local\u001fs1\u001fh", "rev": 1, "doc": {"id": "a1", "tenant": "local", "session": "s1", "call_hash": "h", "state": "pending", "created_at": "2026-11-18T03:04:05+00:00", "expires_at": "2026-11-18T03:05:05.123456+00:00"}}
  ],
  "streams": [{"stream": "connector-engine/local", "tenant_id": "local", "last": 2}],
  "outbox": [
    {"pos": 7, "id": "e1", "tenant_id": "local", "stream": "connector-engine/local", "sequence": 1, "doc": {"id": "e1", "type": "connector.connector.installed"}, "delivered_at": "2026-11-18T03:04:06.25+00:00"},
    {"pos": 8, "id": "e2", "tenant_id": "local", "stream": "connector-engine/local", "sequence": 2, "doc": {"id": "e2", "type": "connector.connector.updated"}, "delivered_at": null}
  ],
  "audit": [{"id": 4, "tenant_id": "local", "subject": "c1", "doc": {"at": "2026-11-18T03:04:05+00:00", "tenant": "local", "action": "install", "subject": "c1"}}],
  "inbox": [{"tenant_id": "local", "id": "m1", "at": "2026-11-18T03:04:05.999+00:00"}]
}}`

func TestRestoresAPostgresSnapshot(t *testing.T) {
	var snap port.Snapshot
	if err := json.Unmarshal([]byte(postgresSnapshot), &snap); err != nil {
		t.Fatal(err)
	}
	s := open(t, filepath.Join(t.TempDir(), "store.db"))
	if err := s.Restore(ctx, snap); err != nil {
		t.Fatal(err)
	}
	local := port.Scope{Tenant: "local"}
	if err := s.Read(ctx, local, func(tx port.Tx) error {
		c, err := tx.GetConnector("c1")
		if err != nil || c.Revision != 3 || c.Alias != "notes" {
			t.Fatalf("connector %+v, %v", c, err)
		}
		seen, err := tx.SeenInbox("m1")
		audit, err2 := tx.ListAudit("c1")
		if !seen || len(audit) != 1 || err != nil || err2 != nil {
			t.Fatalf("seen=%v audit=%d %v %v", seen, len(audit), err, err2)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) error {
		due, err := tx.DueWork(t0.Add(2*time.Hour), 0)
		want := []port.WorkItem{
			{Tenant: "local", Kind: port.WorkApprovalExpiry, ID: "a1", Due: time.Date(2026, 11, 18, 3, 5, 5, 123456000, time.UTC)},
			{Tenant: "local", Kind: port.WorkGrantExpiry, ID: "g1", Due: time.Date(2026, 11, 18, 4, 4, 5, 500000000, time.UTC)},
		}
		if fmt.Sprint(due) != fmt.Sprint(want) {
			t.Fatalf("due %v, want %v", due, want)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending(ctx, 0)
	if err != nil || len(pending) != 1 || pending[0].ID != "e2" {
		t.Fatalf("pending %+v, %v", pending, err)
	}
	if err := s.Write(ctx, local, func(tx port.Tx) error {
		e, err := tx.Emit(port.Event{Type: "x", Tenant: "local", Time: t0})
		if e.Sequence != 3 {
			t.Fatalf("sequence %d after restore", e.Sequence)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMigratesALivePostgresStore(t *testing.T) {
	if pg == nil {
		t.Skip("postgres container unavailable")
	}
	dsn, err := pg.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	src, err := postgres.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	local := port.Scope{Tenant: "local"}
	seed := func(tx port.Tx) error {
		if _, err := tx.PutConnector(port.Connector{ID: "c1", Tenant: "local", Workspace: "default", Alias: "notes"}, port.AnyRevision); err != nil {
			return err
		}
		if err := tx.PutGrant(port.Grant{ID: "g1", Tenant: "local", ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "c1", Selector: "*", Mode: port.ModeAllow, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}); err != nil {
			return err
		}
		if _, err := tx.MarkInbox("m1", t0); err != nil {
			return err
		}
		if _, err := tx.Emit(port.Event{Type: "connector.connector.installed", Tenant: "local", Time: t0}); err != nil {
			return err
		}
		return tx.Audit(port.AuditRow{At: t0, Tenant: "local", Actor: "a", Action: "install", Subject: "c1"})
	}
	if err := src.Write(ctx, port.Scope{Installation: true}, func(tx port.Tx) error {
		return tx.PutTenant(port.Tenant{ID: "local", State: port.TenantActive, CreatedAt: t0})
	}); err != nil {
		t.Fatal(err)
	}
	if err := src.Write(ctx, local, seed); err != nil {
		t.Fatal(err)
	}
	snap, err := src.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dst := open(t, filepath.Join(t.TempDir(), "store.db"))
	if err := dst.Restore(ctx, snap); err != nil {
		t.Fatal(err)
	}
	view := func(s port.Store) string {
		var out string
		if err := s.Read(ctx, local, func(tx port.Tx) error {
			cs, err1 := tx.ListConnectors(port.ConnectorFilter{})
			gs, err2 := tx.ListGrants(port.GrantFilter{})
			seen, err3 := tx.SeenInbox("m1")
			audit, err4 := tx.ListAudit("c1")
			events, err5 := tx.EventsAfter(port.SourceOf("local"), 0, 0)
			raw, _ := json.Marshal([]any{cs, gs, seen, audit, events})
			out = string(raw)
			for _, err := range []error{err1, err2, err3, err4, err5} {
				if err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if a, b := view(src), view(dst); a != b {
		t.Fatalf("postgres %s\nsqlite   %s", a, b)
	}
	if err := dst.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) error {
		due, err := tx.DueWork(t0.Add(2*time.Hour), 0)
		if len(due) != 1 || due[0].ID != "g1" || !due[0].Due.Equal(t0.Add(time.Hour)) {
			t.Fatalf("due %+v", due)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
