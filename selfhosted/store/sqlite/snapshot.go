package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

const snapshotStore = "sqlite"

type recordRow struct {
	Kind   string          `json:"kind"`
	K      string          `json:"k"`
	Tenant string          `json:"tenant_id"`
	Uniq   *string         `json:"uniq"`
	Rev    int64           `json:"rev"`
	Doc    json.RawMessage `json:"doc"`
}

type streamRow struct {
	Stream string `json:"stream"`
	Tenant string `json:"tenant_id"`
	Last   int64  `json:"last"`
}

type outboxRow struct {
	Pos         int64           `json:"pos"`
	ID          string          `json:"id"`
	Tenant      string          `json:"tenant_id"`
	Stream      string          `json:"stream"`
	Sequence    int64           `json:"sequence"`
	Doc         json.RawMessage `json:"doc"`
	DeliveredAt *time.Time      `json:"delivered_at"`
}

type auditRow struct {
	ID      int64           `json:"id"`
	Tenant  string          `json:"tenant_id"`
	Subject string          `json:"subject"`
	Doc     json.RawMessage `json:"doc"`
}

type inboxRow struct {
	Tenant  string    `json:"tenant_id"`
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Payload string    `json:"payload"`
}

type rows struct {
	records []recordRow
	streams []streamRow
	outbox  []outboxRow
	audit   []auditRow
	inbox   []inboxRow
}

// Snapshot copies every row of every tenant and of the installation in one read transaction, in the row shape of the Postgres store.
func (s *Store) Snapshot(ctx context.Context) (port.Snapshot, error) {
	var r rows
	err := s.run(ctx, port.Scope{Installation: true}, s.reader, func(t *tx) (err error) {
		if r.records, err = scan(t, `SELECT kind, k, tenant_id, uniq, rev, doc FROM records ORDER BY kind, k`, func(q *sql.Rows) (v recordRow, err error) {
			var doc string
			err = q.Scan(&v.Kind, &v.K, &v.Tenant, &v.Uniq, &v.Rev, &doc)
			v.Doc = json.RawMessage(doc)
			return v, err
		}); err != nil {
			return err
		}
		if r.streams, err = scan(t, `SELECT stream, tenant_id, last FROM streams ORDER BY stream`, func(q *sql.Rows) (v streamRow, err error) {
			return v, q.Scan(&v.Stream, &v.Tenant, &v.Last)
		}); err != nil {
			return err
		}
		if r.outbox, err = scan(t, `SELECT pos, id, tenant_id, stream, sequence, doc, delivered_at FROM outbox ORDER BY pos`, func(q *sql.Rows) (v outboxRow, err error) {
			var doc string
			var at sql.NullString
			if err = q.Scan(&v.Pos, &v.ID, &v.Tenant, &v.Stream, &v.Sequence, &doc, &at); err == nil && at.Valid {
				var d time.Time
				d, err = time.Parse(time.RFC3339Nano, at.String)
				v.DeliveredAt = &d
			}
			v.Doc = json.RawMessage(doc)
			return v, err
		}); err != nil {
			return err
		}
		if r.audit, err = scan(t, `SELECT id, tenant_id, subject, doc FROM audit ORDER BY id`, func(q *sql.Rows) (v auditRow, err error) {
			var doc string
			err = q.Scan(&v.ID, &v.Tenant, &v.Subject, &doc)
			v.Doc = json.RawMessage(doc)
			return v, err
		}); err != nil {
			return err
		}
		r.inbox, err = scan(t, `SELECT tenant_id, id, at, coalesce(payload, '') FROM inbox ORDER BY tenant_id, id`, func(q *sql.Rows) (v inboxRow, err error) {
			var at string
			if err = q.Scan(&v.Tenant, &v.ID, &at, &v.Payload); err == nil {
				v.At, err = time.Parse(time.RFC3339Nano, at)
			}
			return v, err
		})
		return err
	})
	if err != nil {
		return port.Snapshot{}, err
	}
	snap := port.Snapshot{Store: snapshotStore, Tables: map[string][]json.RawMessage{}}
	for name, table := range map[string]any{"records": r.records, "streams": r.streams, "outbox": r.outbox, "audit": r.audit, "inbox": r.inbox} {
		if snap.Tables[name], err = encodeRows(table); err != nil {
			return port.Snapshot{}, port.Errorf(port.CodeInternal, "snapshot %s: %v", name, err)
		}
	}
	return snap, nil
}

func encodeRows(table any) ([]json.RawMessage, error) {
	raw, err := json.Marshal(table)
	if err != nil {
		return nil, err
	}
	out := []json.RawMessage{}
	return out, json.Unmarshal(raw, &out)
}

func decodeRows[T any](snap port.Snapshot, name string, check func(T) bool) ([]T, error) {
	out := make([]T, 0, len(snap.Tables[name]))
	for _, raw := range snap.Tables[name] {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil || !check(v) {
			return nil, port.Errorf(port.CodeInvalidArgument, "snapshot %s: a row does not decode: %v", name, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func object(doc json.RawMessage) bool {
	return json.Valid(doc) && bytes.HasPrefix(bytes.TrimSpace(doc), []byte("{"))
}

func decodeSnapshot(snap port.Snapshot) (r rows, err error) {
	if r.records, err = decodeRows(snap, "records", func(v recordRow) bool { return v.Kind != "" && v.K != "" && object(v.Doc) }); err != nil {
		return r, err
	}
	if r.streams, err = decodeRows(snap, "streams", func(v streamRow) bool { return v.Stream != "" }); err != nil {
		return r, err
	}
	if r.outbox, err = decodeRows(snap, "outbox", func(v outboxRow) bool { return v.Pos > 0 && v.ID != "" && object(v.Doc) }); err != nil {
		return r, err
	}
	if r.audit, err = decodeRows(snap, "audit", func(v auditRow) bool { return v.ID > 0 && object(v.Doc) }); err != nil {
		return r, err
	}
	r.inbox, err = decodeRows(snap, "inbox", func(v inboxRow) bool { return v.ID != "" })
	return r, err
}

// Restore replaces every row with a snapshot of this store or of the Postgres store in one transaction and rebuilds the due work.
func (s *Store) Restore(ctx context.Context, snap port.Snapshot) error {
	if snap.Store != snapshotStore && snap.Store != "postgres" {
		return port.Errorf(port.CodeInvalidArgument, "a %q snapshot does not restore into a %s store", snap.Store, snapshotStore)
	}
	r, err := decodeSnapshot(snap)
	if err != nil {
		return err
	}
	dues := make([]any, len(r.records))
	for i, rec := range r.records {
		if dues[i], err = decodeDue(rec.Kind, rec.Doc); err != nil {
			return port.Errorf(port.CodeInvalidArgument, "snapshot records: %s %s does not decode: %v", rec.Kind, shown(rec.K), err)
		}
	}
	return s.run(ctx, port.Scope{Installation: true}, s.writer, func(t *tx) error {
		for _, table := range []string{"records", "streams", "outbox", "audit", "inbox", "due_work"} {
			if _, err := t.exec(`DELETE FROM ` + table); err != nil {
				return err
			}
		}
		for i, v := range r.records {
			if _, err := t.exec(`INSERT INTO records (kind, k, tenant_id, uniq, rev, doc) VALUES (?3, ?4, ?5, ?6, ?7, ?8)`, v.Kind, v.K, v.Tenant, v.Uniq, v.Rev, string(v.Doc)); err != nil {
				return err
			}
			if err := t.track(v.Kind, v.K, port.TenantID(v.Tenant), dues[i]); err != nil {
				return err
			}
		}
		for _, v := range r.streams {
			if _, err := t.exec(`INSERT INTO streams (stream, tenant_id, last) VALUES (?3, ?4, ?5)`, v.Stream, v.Tenant, v.Last); err != nil {
				return err
			}
		}
		for _, v := range r.outbox {
			var at *string
			if v.DeliveredAt != nil {
				d := stamp(*v.DeliveredAt)
				at = &d
			}
			if _, err := t.exec(`INSERT INTO outbox (pos, id, tenant_id, stream, sequence, doc, delivered_at) VALUES (?3, ?4, ?5, ?6, ?7, ?8, ?9)`, v.Pos, v.ID, v.Tenant, v.Stream, v.Sequence, string(v.Doc), at); err != nil {
				return err
			}
		}
		for _, v := range r.audit {
			if _, err := t.exec(`INSERT INTO audit (id, tenant_id, subject, doc) VALUES (?3, ?4, ?5, ?6)`, v.ID, v.Tenant, v.Subject, string(v.Doc)); err != nil {
				return err
			}
		}
		for _, v := range r.inbox {
			if _, err := t.exec(`INSERT INTO inbox (tenant_id, id, at, payload) VALUES (?3, ?4, ?5, ?6)`, v.Tenant, v.ID, stamp(v.At), v.Payload); err != nil {
				return err
			}
		}
		return canonicalGrants(ctx, t.q)
	})
}

const canonicalGrantsMigration = "migrations/0003_canonical_grants.sql"

func canonicalGrants(ctx context.Context, q *sql.Tx) error {
	tenants, err := many[string](&tx{ctx: ctx, q: q, scope: port.Scope{Installation: true}}, `SELECT DISTINCT tenant_id FROM records WHERE kind = 'grant' ORDER BY tenant_id`)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		if err := grant.Rewrite(&tx{ctx: ctx, q: q, scope: port.Scope{Tenant: port.TenantID(tenant)}}, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

var _ port.Snapshotter = (*Store)(nil)
