package sqlite

import (
	"context"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

// Prune deletes the delivered events, idempotency keys, audit rows and finished calls of tenant that are older than r allows.
func (s *Store) Prune(ctx context.Context, tenant port.TenantID, now time.Time, r port.Retention) (map[string]int, error) {
	counts := map[string]int{}
	err := s.run(ctx, port.Scope{Tenant: tenant, Installation: tenant == port.Installation}, s.writer, func(t *tx) error {
		for _, q := range []struct {
			name string
			age  time.Duration
			sql  string
		}{
			{"events", r.Delivered, `DELETE FROM outbox WHERE tenant_id = ?3 AND delivered_at < ?4`},
			{"inbox", r.Inbox, `DELETE FROM inbox WHERE tenant_id = ?3 AND at < ?4`},
			{"audit", r.Audit, `DELETE FROM audit WHERE tenant_id = ?3 AND cn_time(json_extract(doc, '$.at')) < ?4`},
			{"calls", r.Calls, `DELETE FROM records WHERE kind = 'call' AND tenant_id = ?3 AND json_extract(doc, '$.state') <> 'started' AND cn_time(json_extract(doc, '$.finished_at')) < ?4`},
		} {
			if q.age <= 0 {
				continue
			}
			n, err := t.exec(q.sql, string(tenant), stamp(now.Add(-q.age)))
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
