package memory

import (
	"bytes"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

func (t *tx) LockPermissions() (int64, error) {
	if !t.write {
		return 0, port.Errorf(port.CodeInternal, "write in a read transaction")
	}
	return t.d.permHeads[t.scope.Tenant], nil
}

func (t *tx) AppendPermissionChanges(changes []port.PermissionChange) (int64, error) {
	if !t.write {
		return 0, port.Errorf(port.CodeInternal, "write in a read transaction")
	}
	stamped, err := port.PermissionChanges(t.scope.Tenant, changes)
	if err != nil {
		return 0, err
	}
	rev := t.d.permHeads[t.scope.Tenant] + 1
	for i := range stamped {
		stamped[i].Revision = rev
	}
	t.d.permHeads[t.scope.Tenant] = rev
	t.d.permChanges = append(t.d.permChanges, stamped...)
	return rev, nil
}

func (t *tx) ListPermissionChanges(revision int64, seq int, limit int) ([]port.PermissionChange, error) {
	out := []port.PermissionChange{}
	for _, c := range t.d.permChanges {
		if c.Tenant != t.scope.Tenant || c.Revision < revision || (c.Revision == revision && c.Seq <= seq) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		c.Before, c.After = bytes.Clone(c.Before), bytes.Clone(c.After)
		out = append(out, c)
	}
	return out, nil
}

func keepLivePermissions(work *data, live data) {
	work.permChanges = slices.DeleteFunc(work.permChanges, func(c port.PermissionChange) bool { return live.permHeads[c.Tenant] > 0 })
	for tenant, rev := range live.permHeads {
		if rev > 0 {
			work.permHeads[tenant] = rev
		}
	}
	work.permChanges = append(work.permChanges, live.permChanges...)
}
