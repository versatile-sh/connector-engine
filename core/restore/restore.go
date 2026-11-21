// Package restore checks the rows a snapshot restore wrote, inside the unit of work that wrote them, and records the restore.
package restore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/catalog"
	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

// Event is the type of the event and the action of the audit row a restore writes.
const Event = "connector.snapshot.restored"

const (
	shownMax    = 64
	warningsMax = 20
)

// Quote quotes a value taken from a snapshot for a refusal or a warning, cut to 64 characters.
func Quote[V ~string](v V) string {
	if r := []rune(string(v)); len(r) > shownMax {
		return fmt.Sprintf("%q…", string(r[:shownMax]))
	}
	return fmt.Sprintf("%q", string(v))
}

func refuse(format string, args ...any) error {
	return port.Errorf(port.CodeInvalidArgument, "snapshot: "+format, args...)
}

func warn(out *port.Restored, format string, args ...any) {
	if out.WarningCount++; len(out.Warnings) < warningsMax {
		out.Warnings = append(out.Warnings, fmt.Sprintf(format, args...))
	}
}

// Apply checks and canonicalizes each tenant's restored rows, then audits the restore with one event; a refused row refuses it whole.
func Apply(tenants []port.TenantID, in func(port.TenantID) (port.Tx, error), by string, now time.Time) (port.Restored, error) {
	out := port.Restored{Counts: map[string]int{}}
	for _, tenant := range tenants {
		if tenant == port.Installation {
			continue
		}
		tx, err := in(tenant)
		if err != nil {
			return out, err
		}
		if err := check(tx, tenant, now, &out); err != nil {
			return out, err
		}
	}
	tx, err := in(port.Installation)
	if err != nil {
		return out, err
	}
	if err := installationOnly(tx); err != nil {
		return out, err
	}
	all, err := tx.ListTenants()
	if err != nil {
		return out, err
	}
	out.Counts["tenants"] = len(all)
	if err := tx.Audit(port.AuditRow{At: now, Tenant: port.Installation, Actor: by, Action: Event, Subject: "snapshot",
		Detail: map[string]any{"counts": out.Counts, "warnings": out.WarningCount, "first_warnings": out.Warnings}}); err != nil {
		return out, err
	}
	raw, err := json.Marshal(catalog.SnapshotRestored{By: by, Counts: out.Counts, Warnings: out.WarningCount})
	if err != nil {
		return out, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return out, err
	}
	_, err = tx.Emit(port.Event{Type: Event, Subject: "snapshot", Time: now, Data: data})
	return out, err
}

func installationOnly(tx port.Tx) error {
	grants, err := tx.ListGrants(port.GrantFilter{})
	if err != nil {
		return err
	}
	if len(grants) > 0 {
		return refuse("grant %s is stored under the installation, not under a tenant", Quote(grants[0].ID))
	}
	connectors, err := tx.ListConnectors(port.ConnectorFilter{})
	if err != nil {
		return err
	}
	if len(connectors) > 0 {
		return refuse("connector %s is stored under the installation, not under a tenant", Quote(connectors[0].ID))
	}
	bindings, err := tx.ListBindings(port.BindingFilter{})
	if err != nil {
		return err
	}
	if len(bindings) > 0 {
		return refuse("binding %s/%s is stored under the installation, not under a tenant", Quote(bindings[0].Connector), Quote(bindings[0].Agent))
	}
	return nil
}

func check(tx port.Tx, tenant port.TenantID, now time.Time, out *port.Restored) error {
	rows, err := tx.ListGrants(port.GrantFilter{})
	if err != nil {
		return err
	}
	keys := map[string]port.GrantID{}
	for _, g := range rows {
		if g.Tenant != tenant {
			return refuse("grant %s is a row of tenant %s stored under %s", Quote(g.ID), Quote(g.Tenant), Quote(tenant))
		}
		if id, ok := keys[g.Key()]; ok {
			return refuse("grants %s and %s have one key", Quote(id), Quote(g.ID))
		}
		keys[g.Key()] = g.ID
	}
	if err := grant.Rewrite(tx, now); err != nil {
		return err
	}
	connectors, err := tx.ListConnectors(port.ConnectorFilter{})
	if err != nil {
		return err
	}
	functions := map[port.ConnectorID]map[string]bool{}
	for _, c := range connectors {
		if c.Tenant != tenant {
			return refuse("connector %s is a row of tenant %s stored under %s", Quote(c.ID), Quote(c.Tenant), Quote(tenant))
		}
		fns, err := tx.ListFunctions(c.ID)
		if err != nil {
			return err
		}
		functions[c.ID] = map[string]bool{}
		for _, f := range fns {
			functions[c.ID][f.Name] = true
		}
		out.Counts["functions"] += len(fns)
	}
	bindings, err := tx.ListBindings(port.BindingFilter{})
	if err != nil {
		return err
	}
	for _, b := range bindings {
		switch {
		case b.Tenant != tenant:
			return refuse("binding %s/%s is a row of tenant %s stored under %s", Quote(b.Connector), Quote(b.Agent), Quote(b.Tenant), Quote(tenant))
		case functions[b.Connector] == nil:
			return refuse("binding %s/%s names a connector the snapshot lacks", Quote(b.Connector), Quote(b.Agent))
		}
	}
	if rows, err = tx.ListGrants(port.GrantFilter{}); err != nil {
		return err
	}
	for _, g := range rows {
		var refused *port.Error
		if err := grant.Validate(g, grant.Policy{}); errors.As(err, &refused) {
			return refuse("grant %s is refused by the row rules: %s", Quote(g.ID), Quote(refused.Detail))
		}
		name, fn := strings.CutPrefix(g.Selector, port.SelectorFunction)
		switch {
		case g.Connector != "" && functions[g.Connector] == nil:
			warn(out, "grant %s names connector %s, which the snapshot lacks", Quote(g.ID), Quote(g.Connector))
		case g.Connector != "" && fn && !functions[g.Connector][name]:
			warn(out, "grant %s names function %s, which the snapshot lacks", Quote(g.ID), Quote(name))
		}
	}
	out.Counts["connectors"] += len(connectors)
	out.Counts["bindings"] += len(bindings)
	out.Counts["grants"] += len(rows)
	return nil
}
