package memory

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func purge[V any](tb table[V], tenant port.TenantID) int {
	n := 0
	for k, r := range tb.rows {
		if r.tenant == tenant {
			delete(tb.rows, k)
			n++
		}
	}
	return n
}

// PurgeTenant deletes the rows of a tenant, its audit and inbox rows and its delivered events, and keeps the registry row.
func (s *Store) PurgeTenant(ctx context.Context, tenant port.TenantID) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	if tenant == port.Installation {
		return nil, port.Errorf(port.CodeInvalidArgument, "the installation is not a tenant")
	}
	s.outside()
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.d
	counts := map[string]int{
		"policies": purge(d.policies, tenant), "rules": purge(d.rules, tenant), "connectors": purge(d.connectors, tenant),
		"functions": purge(d.functions, tenant), "bindings": purge(d.bindings, tenant), "grants": purge(d.grants, tenant),
		"approvals": purge(d.approvals, tenant), "pauses": purge(d.pauses, tenant), "tombstones": purge(d.tombstones, tenant),
		"credentials": purge(d.credentials, tenant), "overlays": purge(d.overlays, tenant), "calls": purge(d.calls, tenant),
		"exposures": purge(d.exposures, tenant), "clients": purge(d.clients, tenant), "operations": purge(d.operations, tenant),
		"roles": purge(d.roles, tenant), "members": purge(d.members, tenant), "source_syncs": purge(d.syncs, tenant), "fanouts": purge(d.fanouts, tenant),
		"listings": purge(d.listings, tenant), "versions": purge(d.versions, tenant), "offers": purge(d.offers, tenant),
	}
	before := len(d.audit)
	d.audit = slices.DeleteFunc(d.audit, func(r row[port.AuditRow]) bool { return r.tenant == tenant })
	counts["audit"] = before - len(d.audit)
	before = len(d.events)
	d.events = slices.DeleteFunc(d.events, func(e event) bool { return e.delivered && e.e.Tenant == tenant })
	counts["events"] = before - len(d.events)
	before = len(d.permChanges)
	d.permChanges = slices.DeleteFunc(d.permChanges, func(c port.PermissionChange) bool { return c.Tenant == tenant })
	counts["permission_changes"] = before - len(d.permChanges)
	delete(d.permHeads, tenant)
	prefix := key(string(tenant), "")
	maps.DeleteFunc(d.inbox, func(k string, _ time.Time) bool { return strings.HasPrefix(k, prefix) })
	maps.DeleteFunc(d.inboxData, func(k string, _ []byte) bool { return strings.HasPrefix(k, prefix) })
	return counts, nil
}

var _ port.TenantPurger = (*Store)(nil)
