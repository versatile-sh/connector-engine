package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/store/postgres"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

var pg *storetest.Postgres

func TestMain(m *testing.M) {
	var err error
	if pg, err = storetest.StartPostgres(); err != nil {
		fmt.Fprintln(os.Stderr, "postgres tests skipped:", err)
	}
	code := m.Run()
	if pg != nil {
		pg.Stop()
	}
	os.Exit(code)
}

func newStore(t *testing.T) (*postgres.Store, string) {
	t.Helper()
	if pg == nil {
		t.Skip("postgres container unavailable")
	}
	dsn, err := pg.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := postgres.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	for range 2 {
		if err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return s, dsn
}

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) (port.Store, port.Outbox) {
		s, _ := newStore(t)
		return s, s
	})
}

func TestNewRefusesRolesThatBypassRLS(t *testing.T) {
	if pg == nil {
		t.Skip("postgres container unavailable")
	}
	_, err := postgres.New(context.Background(), pg.SuperuserDSN())
	if !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("got %v", err)
	}
}

func TestRowLevelSecurityBlocksCraftedQueries(t *testing.T) {
	s, dsn := newStore(t)
	ctx := context.Background()
	for _, tenant := range []port.TenantID{"ta", "tb"} {
		err := s.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
			if _, err := tx.PutConnector(port.Connector{ID: port.ConnectorID("c-" + tenant), Tenant: tenant, Workspace: "w"}, port.AnyRevision); err != nil {
				return err
			}
			if err := tx.Audit(port.AuditRow{At: time.Now(), Tenant: tenant, Actor: "a", Action: "x"}); err != nil {
				return err
			}
			_, err := tx.Emit(port.Event{Type: "t", Tenant: tenant})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	begin := func(tenant string) pgx.Tx {
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close(ctx) })
		q, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tenant != "" {
			if _, err := q.Exec(ctx, `SELECT set_config('cn.tenant', $1, true)`, tenant); err != nil {
				t.Fatal(err)
			}
		}
		return q
	}
	count := func(q pgx.Tx, sql string) int {
		var n int
		if err := q.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"records", "audit", "outbox", "streams"} {
		q := begin("ta")
		if n := count(q, "SELECT count(*) FROM "+table); n != 1 {
			t.Errorf("%s: tenant ta sees %d rows", table, n)
		}
		if n := count(q, "SELECT count(*) FROM "+table+" WHERE true OR tenant_id = 'tb'"); n != 1 {
			t.Errorf("%s: crafted predicate sees %d rows", table, n)
		}
		tag, err := q.Exec(ctx, "DELETE FROM "+table+" WHERE tenant_id = 'tb'")
		if err != nil || tag.RowsAffected() != 0 {
			t.Errorf("%s: delete of tenant tb rows: %v %d", table, err, tag.RowsAffected())
		}
		if n := count(begin(""), "SELECT count(*) FROM "+table); n != 0 {
			t.Errorf("%s: a session without a tenant sees %d rows", table, n)
		}
	}
	q := begin("ta")
	if _, err := q.Exec(ctx, `UPDATE records SET tenant_id = 'tb' WHERE k = 'c-ta'`); err == nil {
		t.Error("moving a row to another tenant succeeded")
	}
	q = begin("ta")
	if _, err := q.Exec(ctx, `INSERT INTO records (kind, k, tenant_id, rev, doc) VALUES ('connector', 'x', 'tb', 1, '{}')`); err == nil {
		t.Error("insert into another tenant succeeded")
	}
}

func TestReadsAcrossTenantsReturnNothing(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if err := s.Write(ctx, port.Scope{Tenant: "ta"}, func(tx port.Tx) error {
		_, err := tx.PutConnector(port.Connector{ID: "c1", Tenant: "ta"}, port.AnyRevision)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := s.Read(ctx, port.Scope{Tenant: "tb"}, func(tx port.Tx) error {
		if _, err := tx.GetConnector("c1"); !port.IsCode(err, port.CodeNotFound) {
			t.Errorf("get across tenants: %v", err)
		}
		if cs, err := tx.ListConnectors(port.ConnectorFilter{}); err != nil || len(cs) != 0 {
			t.Errorf("list across tenants: %v %v", cs, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
