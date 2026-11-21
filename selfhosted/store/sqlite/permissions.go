package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

type headRow struct {
	Tenant   string `json:"tenant_id"`
	Revision int64  `json:"revision"`
}

func (t *tx) LockPermissions() (int64, error) {
	rev, err := one[int64](t, `INSERT INTO permission_head (tenant_id, revision) VALUES (?1, 0)
		ON CONFLICT (tenant_id) DO UPDATE SET revision = revision RETURNING revision`)
	t.locked = err == nil
	return rev, err
}

func (t *tx) AppendPermissionChanges(changes []port.PermissionChange) (int64, error) {
	if !t.locked {
		return 0, port.Errorf(port.CodeInternal, "access setting changes are appended after LockPermissions in the same transaction")
	}
	stamped, err := port.PermissionChanges(t.scope.Tenant, changes)
	if err != nil {
		return 0, err
	}
	rev, err := one[int64](t, `INSERT INTO permission_head (tenant_id, revision) VALUES (?1, 1)
		ON CONFLICT (tenant_id) DO UPDATE SET revision = revision + 1 RETURNING revision`)
	if err != nil {
		return 0, err
	}
	for i := range stamped {
		stamped[i].Revision = rev
	}
	return rev, t.insertChanges(stamped)
}

func (t *tx) insertChanges(changes []port.PermissionChange) error {
	for _, c := range changes {
		if _, err := t.exec(`INSERT INTO permission_changes VALUES (?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, nullif(?11, 'null'), nullif(?12, 'null'))`,
			string(c.Tenant), c.Revision, c.Seq, stamp(c.At), c.Actor, c.Op, c.Kind, c.Key, value(c.Before), value(c.After)); err != nil {
			return err
		}
	}
	return nil
}

func value(raw json.RawMessage) *string {
	if raw == nil {
		return nil
	}
	s := string(raw)
	return &s
}

func (t *tx) ListPermissionChanges(revision int64, seq int, limit int) ([]port.PermissionChange, error) {
	return t.permissionChanges(`WHERE tenant_id = ?1 AND (revision, seq) > (?3, ?4) ORDER BY revision, seq LIMIT ?5`, revision, seq, limitOf(limit))
}

func (t *tx) permissionChanges(where string, a ...any) ([]port.PermissionChange, error) {
	return scan(t, `SELECT tenant_id, revision, seq, at, actor, op, kind, key, before, after FROM permission_changes `+where, func(q *sql.Rows) (c port.PermissionChange, err error) {
		var at string
		var before, after sql.NullString
		if err = q.Scan(&c.Tenant, &c.Revision, &c.Seq, &at, &c.Actor, &c.Op, &c.Kind, &c.Key, &before, &after); err == nil {
			c.At, err = time.Parse(time.RFC3339Nano, at)
		}
		if before.Valid {
			c.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			c.After = json.RawMessage(after.String)
		}
		return c, err
	}, a...)
}

func (t *tx) importPermissions(heads []headRow, changes []port.PermissionChange) error {
	for _, h := range heads {
		if _, err := t.exec(`DELETE FROM permission_head WHERE tenant_id = ?3 AND revision = 0`, h.Tenant); err != nil {
			return err
		}
		n, err := t.exec(`INSERT INTO permission_head (tenant_id, revision) VALUES (?3, ?4) ON CONFLICT (tenant_id) DO NOTHING`, h.Tenant, h.Revision)
		if err != nil {
			return err
		}
		for _, c := range changes {
			if n == 1 && string(c.Tenant) == h.Tenant {
				if err := t.insertChanges([]port.PermissionChange{c}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
