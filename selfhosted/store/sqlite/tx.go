package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	kindTenant     = "tenant"
	kindListing    = "listing"
	kindVersion    = "version"
	kindOffer      = "offer"
	kindPolicy     = "policy"
	kindRule       = "rule"
	kindConnector  = "connector"
	kindFunction   = "function"
	kindBinding    = "binding"
	kindGrant      = "grant"
	kindApproval   = "approval"
	kindPause      = "pause"
	kindTombstone  = "tombstone"
	kindCredential = "credential"
	kindOverlay    = "overlay"
	kindCall       = "call"
	kindExposure   = "exposure"
	kindClient     = "client"
	kindSourceSync = "source_sync"
	kindOperation  = "operation"
	kindFanOut     = "fanout"
	kindRole       = "role"
	kindMember     = "member"
)

const (
	owns     = `(tenant_id = ?1 OR (tenant_id = '' AND ?2))`
	visible  = `(tenant_id = ?1 OR (tenant_id = '' AND (?2 OR kind IN ('listing', 'version'))) OR (?2 AND kind IN ('tenant', 'offer')))`
	writable = `(CASE WHEN kind IN ('tenant', 'offer') THEN ?2 ELSE ` + owns + ` END)`

	docSQL     = `SELECT json_set(doc, '$.revision', rev) FROM records WHERE `
	getSQL     = docSQL + `kind = ?3 AND k = ?4 AND ` + visible
	revisedSQL = `INSERT INTO records AS r (kind, k, tenant_id, uniq, rev, doc)
		SELECT ?3, ?4, ?5, ?6, 1, ?7
		WHERE ?8 <= 0 OR EXISTS (SELECT 1 FROM records WHERE kind = ?3 AND k = ?4 AND rev = ?8)
		ON CONFLICT (kind, k) DO UPDATE SET doc = CASE WHEN r.kind = 'grant' AND json_extract(r.doc, '$.uses') IS NOT NULL
			THEN json_set(excluded.doc, '$.uses', json_extract(r.doc, '$.uses')) ELSE excluded.doc END, uniq = excluded.uniq, rev = r.rev + 1
		WHERE (?8 < 0 OR r.rev = ?8) AND r.tenant_id = excluded.tenant_id
		RETURNING rev`
)

type tx struct {
	ctx   context.Context
	q     *sql.Tx
	scope port.Scope
}

func (t *tx) args(a ...any) []any {
	return append([]any{string(t.scope.Tenant), t.scope.Installation}, a...)
}

func (t *tx) exec(query string, a ...any) (int64, error) {
	res, err := t.q.ExecContext(t.ctx, query, t.args(a...)...)
	if err != nil {
		return 0, fail(err)
	}
	n, err := res.RowsAffected()
	return n, fail(err)
}

func one[T any](t *tx, query string, a ...any) (v T, err error) {
	return v, fail(t.q.QueryRowContext(t.ctx, query, t.args(a...)...).Scan(&v))
}

func scan[T any](t *tx, query string, row func(*sql.Rows) (T, error), a ...any) ([]T, error) {
	rows, err := t.q.QueryContext(t.ctx, query, t.args(a...)...)
	if err != nil {
		return nil, fail(err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := row(rows)
		if err != nil {
			return nil, fail(err)
		}
		out = append(out, v)
	}
	return out, fail(rows.Err())
}

func many[T any](t *tx, query string, a ...any) ([]T, error) {
	return scan(t, query, func(r *sql.Rows) (v T, err error) { return v, r.Scan(&v) }, a...)
}

func docs[V any](t *tx, query string, a ...any) ([]V, error) {
	return scan(t, query, func(r *sql.Rows) (v V, err error) {
		var raw string
		if err = r.Scan(&raw); err == nil {
			err = json.Unmarshal([]byte(raw), &v)
		}
		return v, err
	}, a...)
}

func encode(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", port.Errorf(port.CodeInvalidArgument, "record does not encode: %v", err)
	}
	return string(raw), nil
}

func key(parts ...string) string { return strings.Join(parts, "\x1f") }

func shown(k string) string { return strings.ReplaceAll(k, "\x1f", "/") }

func want(pairs ...string) []string {
	var out []string
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			out = append(out, pairs[i], pairs[i+1])
		}
	}
	return out
}

func (t *tx) ownsTenant(owner port.TenantID) bool {
	return owner == t.scope.Tenant || (owner == port.Installation && t.scope.Installation)
}

func (t *tx) mayWrite(kind string, owner port.TenantID) error {
	ok := t.ownsTenant(owner)
	if kind == kindTenant || kind == kindOffer {
		ok = t.scope.Installation
	}
	if !ok {
		return port.Errorf(port.CodePermissionDenied, "%s rows of tenant %q are outside this unit of work", kind, owner)
	}
	return nil
}

func get[V any](t *tx, kind, k string) (v V, err error) {
	raw, err := one[string](t, getSQL, kind, k)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &v)
	}
	return v, err
}

func maybe[V any](t *tx, kind, k string) (V, bool, error) {
	v, err := get[V](t, kind, k)
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func list[V any](t *tx, kind string, match []string, order string, keep func(V) bool) ([]V, error) {
	query := docSQL + `kind = ?3 AND ` + visible
	args := []any{kind}
	for i := 0; i < len(match); i += 2 {
		query += fmt.Sprintf(` AND json_extract(doc, '$.%s') = ?%d`, match[i], len(args)+3)
		args = append(args, match[i+1])
	}
	all, err := docs[V](t, query+` ORDER BY `+order, args...)
	if err != nil || keep == nil {
		return all, err
	}
	out := all[:0]
	for _, v := range all {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (t *tx) put(kind, k string, owner port.TenantID, uniq *string, v any, expect int64) (int64, error) {
	if err := t.mayWrite(kind, owner); err != nil {
		return 0, err
	}
	doc, err := encode(v)
	if err != nil {
		return 0, err
	}
	rev, err := one[int64](t, revisedSQL, kind, k, string(owner), uniq, doc, expect)
	if port.IsCode(err, port.CodeNotFound) {
		if expect == port.AnyRevision {
			return 0, port.Errorf(port.CodeConflict, "%s %s changes tenant", kind, shown(k))
		}
		return 0, port.Errorf(port.CodeRevisionMismatch, "%s %s is not at revision %d", kind, shown(k), expect)
	}
	if err != nil {
		return 0, err
	}
	return rev, t.track(kind, k, owner, v)
}

func (t *tx) set(kind, k string, owner port.TenantID, v any) error {
	_, err := t.put(kind, k, owner, nil, v, port.AnyRevision)
	return err
}

func (t *tx) insert(kind, k string, owner port.TenantID, uniq *string, v any) error {
	if err := t.mayWrite(kind, owner); err != nil {
		return err
	}
	doc, err := encode(v)
	if err != nil {
		return err
	}
	if _, err := t.exec(`INSERT INTO records (kind, k, tenant_id, uniq, rev, doc) VALUES (?3, ?4, ?5, ?6, 1, ?7)`, kind, k, string(owner), uniq, doc); err != nil {
		return err
	}
	return t.track(kind, k, owner, v)
}

func (t *tx) update(kind, k string, owner port.TenantID, uniq *string, v any, from string) error {
	doc, err := encode(v)
	if err != nil {
		return err
	}
	n, err := t.exec(`UPDATE records SET doc = ?7, uniq = ?6, rev = rev + 1
		WHERE kind = ?3 AND k = ?4 AND tenant_id = ?5 AND json_extract(doc, '$.state') = ?8 AND `+writable, kind, k, string(owner), uniq, doc, from)
	if err != nil {
		return err
	}
	if n == 1 {
		return t.track(kind, k, owner, v)
	}
	if _, err := one[int](t, `SELECT 1 FROM records WHERE kind = ?3 AND k = ?4 AND `+visible, kind, k); err != nil {
		return err
	}
	return port.Errorf(port.CodeConflict, "%s %s is not %s", kind, shown(k), from)
}

func (t *tx) del(kind, k string) error {
	n, err := t.exec(`DELETE FROM records WHERE kind = ?3 AND k = ?4 AND `+writable, kind, k)
	if err == nil && n == 0 {
		return port.Errorf(port.CodeNotFound, "%s %s not found", kind, shown(k))
	}
	if err != nil {
		return err
	}
	return t.untrack(kind, k)
}
