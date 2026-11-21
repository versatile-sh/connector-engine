package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/core/restore"
	"github.com/versatile-sh/connector-engine/port"
)

const snapshotStore = "postgres"

type snapshotTable struct{ name, order, serial string }

var snapshotTables = []snapshotTable{
	{"records", "kind, k", ""}, {"streams", "stream", ""}, {"outbox", "pos", "pos"}, {"audit", "id", "id"}, {"inbox", "tenant_id, id", ""},
}

func (s *Store) inAll(ctx context.Context, opts pgx.TxOptions, fn func(t *tx, tenants []string) error) error {
	q, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = q.Rollback(ctx) }()
	t := &tx{ctx: ctx, q: q, scope: port.Scope{Installation: true}}
	if _, err := t.exec(`SELECT set_config('cn.tenant', '', true), set_config('cn.installation', 'on', true), set_config('cn.relay', 'off', true)`); err != nil {
		return err
	}
	tenants, err := many[string](t, `SELECT k FROM records WHERE kind = 'tenant' ORDER BY k`)
	if err != nil {
		return err
	}
	if err := fn(t, append([]string{""}, tenants...)); err != nil {
		return err
	}
	return fail(q.Commit(ctx))
}

func (t *tx) as(tenant string) error {
	_, err := t.exec(`SELECT set_config('cn.tenant', $1, true)`, tenant)
	return err
}

// Snapshot copies every row of every tenant and of the installation in one repeatable-read transaction.
func (s *Store) Snapshot(ctx context.Context) (port.Snapshot, error) {
	snap := port.Snapshot{Store: snapshotStore, Schema: schema(), Tables: map[string][]json.RawMessage{}}
	err := s.inAll(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(t *tx, tenants []string) error {
		for _, tenant := range tenants {
			if err := t.as(tenant); err != nil {
				return err
			}
			for _, tb := range snapshotTables {
				rows, err := many[json.RawMessage](t, `SELECT to_jsonb(x) FROM `+tb.name+` x WHERE tenant_id = $1 ORDER BY `+tb.order, tenant)
				if err != nil {
					return err
				}
				snap.Tables[tb.name] = append(snap.Tables[tb.name], rows...)
			}
		}
		return nil
	})
	return snap, err
}

// Restore replaces every row with a snapshot that CheckRows and restore.Apply accept, in one transaction that holds the tables exclusively; due work is rebuilt by the records trigger.
func (s *Store) Restore(ctx context.Context, snap port.Snapshot, by string) (port.Restored, error) {
	if snap.Store != snapshotStore {
		return port.Restored{}, port.Errorf(port.CodeInvalidArgument, "a %q snapshot does not restore into a %s store", snap.Store, snapshotStore)
	}
	if err := CheckRows(snap, schema()); err != nil {
		return port.Restored{}, err
	}
	byTenant := map[string]map[string][]json.RawMessage{}
	serials := map[string]int64{}
	for _, tb := range snapshotTables {
		for _, raw := range snap.Tables[tb.name] {
			var r struct {
				Tenant string `json:"tenant_id"`
				Pos    int64  `json:"pos"`
				ID     any    `json:"id"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				return port.Restored{}, port.Errorf(port.CodeInvalidArgument, "snapshot %s: a row does not decode", tb.name)
			}
			if byTenant[r.Tenant] == nil {
				byTenant[r.Tenant] = map[string][]json.RawMessage{}
			}
			byTenant[r.Tenant][tb.name] = append(byTenant[r.Tenant][tb.name], raw)
			if id, ok := r.ID.(float64); ok && tb.serial == "id" {
				serials[tb.name] = max(serials[tb.name], int64(id))
			}
			if tb.serial == "pos" {
				serials[tb.name] = max(serials[tb.name], r.Pos)
			}
		}
	}
	var out port.Restored
	err := s.inAll(ctx, pgx.TxOptions{}, func(t *tx, tenants []string) error {
		if _, err := t.exec(`LOCK TABLE records, streams, outbox, audit, inbox, due_work IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		for tenant := range byTenant {
			tenants = append(tenants, tenant)
		}
		for _, tenant := range slices.Compact(slices.Sorted(slices.Values(tenants))) {
			if err := t.as(tenant); err != nil {
				return err
			}
			for _, tb := range snapshotTables {
				if _, err := t.exec(`DELETE FROM `+tb.name+` WHERE tenant_id = $1`, tenant); err != nil {
					return err
				}
			}
		}
		for tenant, tables := range byTenant {
			if err := t.as(tenant); err != nil {
				return err
			}
			for _, tb := range snapshotTables {
				if rows := tables[tb.name]; len(rows) > 0 {
					raw, err := json.Marshal(rows)
					if err != nil {
						return err
					}
					if _, err := t.exec(`INSERT INTO `+tb.name+` SELECT * FROM jsonb_populate_recordset(NULL::`+tb.name+`, $1::jsonb)`, string(raw)); err != nil {
						return err
					}
				}
			}
		}
		for _, tb := range snapshotTables {
			if tb.serial != "" {
				if _, err := t.exec(`SELECT setval(pg_get_serial_sequence($1, $2), greatest($3::bigint, nextval(pg_get_serial_sequence($1, $2))))`, tb.name, tb.serial, serials[tb.name]); err != nil {
					return err
				}
			}
		}
		var owners []port.TenantID
		for tenant := range byTenant {
			owners = append(owners, port.TenantID(tenant))
		}
		slices.Sort(owners)
		in := func(tenant port.TenantID) (port.Tx, error) { return t, t.as(string(tenant)) }
		var err error
		out, err = restore.Apply(owners, in, by, time.Now().UTC())
		return err
	})
	if err != nil {
		return port.Restored{}, err
	}
	return out, nil
}

func schema() int {
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	return len(files)
}

var recordDocs = map[string]func([]byte) error{
	kindTenant: decodes[port.Tenant], kindConnector: decodes[port.Connector], kindFunction: decodes[port.Function],
	kindBinding: decodes[port.Binding], kindGrant: decodes[port.Grant],
}

func decodes[V any](doc []byte) error {
	var v V
	return json.Unmarshal(doc, &v)
}

// CheckRows refuses, naming the first offending row, a snapshot in this store's row shape of a schema newer than schema, with an unknown table, or with a record that does not decode or repeats a key.
func CheckRows(snap port.Snapshot, schema int) error {
	if snap.Schema > schema {
		return port.Errorf(port.CodeInvalidArgument, "snapshot: schema %d is newer than this store's %d; restore it with the release that took it", snap.Schema, schema)
	}
	for name := range snap.Tables {
		if !slices.ContainsFunc(snapshotTables, func(tb snapshotTable) bool { return tb.name == name }) {
			return port.Errorf(port.CodeInvalidArgument, "snapshot: table %s is not a table of this store", name)
		}
	}
	keys, uniqs := map[[2]string]bool{}, map[[2]string]string{}
	for _, raw := range snap.Tables["records"] {
		var r struct {
			Kind string          `json:"kind"`
			K    string          `json:"k"`
			Uniq *string         `json:"uniq"`
			Doc  json.RawMessage `json:"doc"`
		}
		if json.Unmarshal(raw, &r) != nil || r.Kind == "" || r.K == "" || !bytes.HasPrefix(bytes.TrimSpace(r.Doc), []byte("{")) {
			return port.Errorf(port.CodeInvalidArgument, "snapshot records: a row does not decode")
		}
		if decode := recordDocs[r.Kind]; decode != nil && decode(r.Doc) != nil {
			return port.Errorf(port.CodeInvalidArgument, "snapshot records: %s %q does not decode", r.Kind, r.K)
		}
		if keys[[2]string{r.Kind, r.K}] {
			return port.Errorf(port.CodeInvalidArgument, "snapshot records: %s %q is there twice", r.Kind, r.K)
		}
		keys[[2]string{r.Kind, r.K}] = true
		if r.Uniq == nil {
			continue
		}
		if other, ok := uniqs[[2]string{r.Kind, *r.Uniq}]; ok {
			return port.Errorf(port.CodeInvalidArgument, "snapshot records: %s %q and %q have one key", r.Kind, other, r.K)
		}
		uniqs[[2]string{r.Kind, *r.Uniq}] = r.K
	}
	return nil
}

const canonicalGrantsMigration = "migrations/0003_canonical_grants.sql"

func migrateGrants(ctx context.Context, q pgx.Tx) error {
	t := &tx{ctx: ctx, q: q, scope: port.Scope{Installation: true}}
	if _, err := t.exec(`ALTER TABLE records NO FORCE ROW LEVEL SECURITY`); err != nil {
		return err
	}
	tenants, err := many[port.TenantID](t, `SELECT DISTINCT tenant_id FROM records WHERE kind = 'grant' ORDER BY tenant_id`)
	if err != nil {
		return err
	}
	if _, err := t.exec(`ALTER TABLE records FORCE ROW LEVEL SECURITY`); err != nil {
		return err
	}
	return t.canonicalGrants(tenants)
}

func (t *tx) canonicalGrants(tenants []port.TenantID) error {
	in := func(tenant port.TenantID) (port.Tx, error) { return t, t.as(string(tenant)) }
	if err := grant.RewriteTenants(tenants, in, time.Now().UTC()); err != nil {
		return err
	}
	return t.as("")
}

var _ port.Snapshotter = (*Store)(nil)
