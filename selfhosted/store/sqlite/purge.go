package sqlite

import (
	"context"

	"github.com/versatile-sh/connector-engine/port"
)

// PurgeTenant deletes the rows of a tenant, its audit and inbox rows and its delivered events, and keeps the registry row.
func (s *Store) PurgeTenant(ctx context.Context, tenant port.TenantID) (map[string]int, error) {
	if tenant == port.Installation {
		return nil, port.Errorf(port.CodeInvalidArgument, "the installation is not a tenant")
	}
	counts := map[string]int{}
	err := s.run(ctx, port.Scope{Tenant: tenant, Installation: true}, s.writer, func(t *tx) error {
		kinds, err := many[string](t, `DELETE FROM records WHERE tenant_id = ?3 AND kind <> 'tenant' RETURNING kind`, string(tenant))
		if err != nil {
			return err
		}
		for _, kind := range kinds {
			if kind == kindPolicy {
				counts["policies"]++
			} else {
				counts[kind+"s"]++
			}
		}
		if _, err := t.exec(`DELETE FROM due_work WHERE tenant_id = ?3`, string(tenant)); err != nil {
			return err
		}
		for name, sql := range map[string]string{
			"audit":              `DELETE FROM audit WHERE tenant_id = ?3`,
			"events":             `DELETE FROM outbox WHERE tenant_id = ?3 AND delivered_at IS NOT NULL`,
			"inbox":              `DELETE FROM inbox WHERE tenant_id = ?3`,
			"permission_changes": `DELETE FROM permission_changes WHERE tenant_id = ?3`,
			"permission_head":    `DELETE FROM permission_head WHERE tenant_id = ?3`,
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
