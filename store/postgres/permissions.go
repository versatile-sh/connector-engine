package postgres

import (
	"encoding/json"

	"github.com/versatile-sh/connector-engine/port"
)

func (t *tx) LockPermissions() (int64, error) {
	return one[int64](t, `INSERT INTO permission_head AS h (tenant_id, revision) VALUES ($1, 0)
		ON CONFLICT (tenant_id) DO UPDATE SET revision = h.revision RETURNING revision`, string(t.scope.Tenant))
}

func (t *tx) AppendPermissionChanges(changes []port.PermissionChange) (int64, error) {
	stamped, err := port.PermissionChanges(t.scope.Tenant, changes)
	if err != nil {
		return 0, err
	}
	rev, err := one[int64](t, `INSERT INTO permission_head AS h (tenant_id, revision) VALUES ($1, 1)
		ON CONFLICT (tenant_id) DO UPDATE SET revision = h.revision + 1 RETURNING revision`, string(t.scope.Tenant))
	if err != nil {
		return 0, err
	}
	for i := range stamped {
		stamped[i].Revision = rev
	}
	_, err = t.exec(`INSERT INTO permission_changes SELECT * FROM jsonb_populate_recordset(NULL::permission_changes, $1::jsonb)`, stamped)
	return rev, err
}

func (t *tx) ListPermissionChanges(revision int64, seq int, limit int) ([]port.PermissionChange, error) {
	out, err := many[port.PermissionChange](t, `SELECT to_jsonb(x) FROM permission_changes x
		WHERE tenant_id = $1 AND (revision, seq) > ($2, $3) ORDER BY revision, seq LIMIT nullif($4::int, 0)`, string(t.scope.Tenant), revision, seq, max(limit, 0))
	for i := range out {
		for _, v := range []*json.RawMessage{&out[i].Before, &out[i].After} {
			if string(*v) == "null" {
				*v = nil
			}
		}
	}
	return out, err
}
