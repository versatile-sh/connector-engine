package service_test

import (
	"testing"

	"github.com/agents-control/connector-engine/port"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func TestAgentRowNamesAnExistingConnector(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	row := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "no-such-connector", Selector: "*", Mode: port.ModeAsk}
	if _, err := w.Svc.CreateGrant(w.Ctx, w.admin, row); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("a row on a connector that does not exist: %v", err)
	}
	row.Connector = w.conn.ID
	st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, row))
}
