package postgres

import (
	"context"
	"time"

	"github.com/agents-control/connector-engine/port"
)

// Prune deletes the delivered events, idempotency keys, audit rows and finished calls of tenant that are older than r allows.
func (s *Store) Prune(ctx context.Context, tenant port.TenantID, now time.Time, r port.Retention) (map[string]int, error) {
	counts := map[string]int{}
	err := s.run(ctx, port.Scope{Tenant: tenant, Installation: tenant == port.Installation}, modeWrite, func(t *tx) error {
		for _, q := range []struct {
			name string
			age  time.Duration
			sql  string
		}{
			{"events", r.Delivered, `DELETE FROM outbox WHERE tenant_id = $1 AND delivered_at < $2`},
			{"inbox", r.Inbox, `DELETE FROM inbox WHERE tenant_id = $1 AND at < $2`},
			{"audit", r.Audit, `DELETE FROM audit WHERE tenant_id = $1 AND (doc->>'at')::timestamptz < $2`},
			{"calls", r.Calls, `DELETE FROM records WHERE kind = 'call' AND tenant_id = $1 AND doc->>'state' <> 'started' AND (doc->>'finished_at')::timestamptz < $2`},
		} {
			if q.age <= 0 {
				continue
			}
			n, err := t.exec(q.sql, string(tenant), now.Add(-q.age))
			if err != nil {
				return err
			}
			counts[q.name] = int(n)
		}
		return nil
	})
	return counts, err
}

var _ port.Pruner = (*Store)(nil)
