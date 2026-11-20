package memory

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
)

func expired(age time.Duration, now, at time.Time) bool { return age > 0 && now.Sub(at) > age }

// Prune deletes the delivered events, idempotency keys, audit rows and finished calls of tenant that are older than r allows.
func (s *Store) Prune(ctx context.Context, tenant port.TenantID, now time.Time, r port.Retention) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	s.outside()
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.d
	counts := map[string]int{}
	before := len(d.events)
	d.events = slices.DeleteFunc(d.events, func(e event) bool {
		return e.delivered && e.e.Tenant == tenant && expired(r.Delivered, now, e.e.Time)
	})
	counts["events"] = before - len(d.events)
	before = len(d.audit)
	d.audit = slices.DeleteFunc(d.audit, func(a row[port.AuditRow]) bool { return a.tenant == tenant && expired(r.Audit, now, a.v.At) })
	counts["audit"] = before - len(d.audit)
	before = len(d.inbox)
	prefix := key(string(tenant), "")
	maps.DeleteFunc(d.inbox, func(k string, at time.Time) bool { return strings.HasPrefix(k, prefix) && expired(r.Inbox, now, at) })
	counts["inbox"] = before - len(d.inbox)
	for k, c := range d.calls.rows {
		if c.tenant == tenant && c.v.State != port.CallStarted && expired(r.Calls, now, c.v.FinishedAt) {
			delete(d.calls.rows, k)
			counts["calls"]++
		}
	}
	return counts, nil
}

var _ port.Pruner = (*Store)(nil)
