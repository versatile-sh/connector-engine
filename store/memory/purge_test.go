package memory_test

import (
	"context"
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/store/memory"
)

func TestPurgeTenantDeletesItsRowsOnly(t *testing.T) {
	s, ctx := memory.New(), context.Background()
	put := func(tenant port.TenantID) {
		err := s.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
			if _, err := tx.PutConnector(port.Connector{ID: port.ConnectorID("c-" + string(tenant)), Tenant: tenant}, port.AnyRevision); err != nil {
				return err
			}
			return tx.Audit(port.AuditRow{Tenant: tenant, Actor: "a", Action: "x"})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	put("a")
	put("b")
	counts, err := s.PurgeTenant(ctx, "a")
	if err != nil || counts["connectors"] != 1 || counts["audit"] != 1 {
		t.Fatalf("counts %v, err %v", counts, err)
	}
	for tenant, want := range map[port.TenantID]int{"a": 0, "b": 1} {
		_ = s.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
			conns, _ := tx.ListConnectors(port.ConnectorFilter{})
			if len(conns) != want {
				t.Errorf("tenant %s keeps %d connectors, want %d", tenant, len(conns), want)
			}
			return nil
		})
	}
	if _, err := s.PurgeTenant(ctx, port.Installation); err == nil {
		t.Fatal("the installation is not a tenant")
	}
}
