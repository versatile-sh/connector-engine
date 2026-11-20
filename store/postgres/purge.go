package postgres

import (
	"context"

	"github.com/versatile-sh/connector-engine/port"
)

// PurgeTenant deletes the rows of a tenant, its audit and inbox rows and its delivered events, and keeps the registry row.
func (s *Store) PurgeTenant(ctx context.Context, tenant port.TenantID) (map[string]int, error) {
	if tenant == port.Installation {
		return nil, port.Errorf(port.CodeInvalidArgument, "the installation is not a tenant")
	}
	var counts map[string]int
	err := s.run(ctx, port.Scope{Tenant: tenant, Installation: true}, modeWrite, func(t *tx) (err error) {
		counts, err = one[map[string]int](t, `WITH d AS (DELETE FROM records WHERE tenant_id = $1 AND kind <> 'tenant' RETURNING kind)
			SELECT coalesce(jsonb_object_agg(CASE kind WHEN 'policy' THEN 'policies' ELSE kind || 's' END, n), '{}')
			FROM (SELECT kind, count(*) AS n FROM d GROUP BY kind) c`, string(tenant))
		if err != nil {
			return err
		}
		for name, sql := range map[string]string{
			"audit":  `DELETE FROM audit WHERE tenant_id = $1`,
			"events": `DELETE FROM outbox WHERE tenant_id = $1 AND delivered_at IS NOT NULL`,
			"inbox":  `DELETE FROM inbox WHERE tenant_id = $1`,
		} {
			n, err := t.exec(sql, string(tenant))
			if err != nil {
				return err
			}
			counts[name] = int(n)
		}
		return nil
	})
	return counts, err
}

var _ port.TenantPurger = (*Store)(nil)
