package memory

import (
	"context"
	"slices"

	"github.com/agents-control/connector-engine/port"
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

// PurgeTenant deletes the rows of a tenant, its audit rows and its delivered events, and keeps the registry row.
func (s *Store) PurgeTenant(ctx context.Context, tenant port.TenantID) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	if tenant == port.Installation {
		return nil, port.Errorf(port.CodeInvalidArgument, "the installation is not a tenant")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.d
	counts := map[string]int{
		"policies": purge(d.policies, tenant), "rules": purge(d.rules, tenant), "connectors": purge(d.connectors, tenant),
		"functions": purge(d.functions, tenant), "bindings": purge(d.bindings, tenant), "grants": purge(d.grants, tenant),
		"approvals": purge(d.approvals, tenant), "pauses": purge(d.pauses, tenant), "tombstones": purge(d.tombstones, tenant),
		"credentials": purge(d.credentials, tenant), "overlays": purge(d.overlays, tenant), "calls": purge(d.calls, tenant),
		"exposures": purge(d.exposures, tenant), "operations": purge(d.operations, tenant),
		"roles": purge(d.roles, tenant), "members": purge(d.members, tenant),
		"listings": purge(d.listings, tenant), "versions": purge(d.versions, tenant), "offers": purge(d.offers, tenant),
	}
	before := len(d.audit)
	d.audit = slices.DeleteFunc(d.audit, func(r row[port.AuditRow]) bool { return r.tenant == tenant })
	counts["audit"] = before - len(d.audit)
	before = len(d.events)
	d.events = slices.DeleteFunc(d.events, func(e event) bool { return e.delivered && e.e.Tenant == tenant })
	counts["events"] = before - len(d.events)
	return counts, nil
}

var _ port.TenantPurger = (*Store)(nil)
