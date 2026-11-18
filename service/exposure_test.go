package service_test

import (
	"slices"
	"testing"

	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func forceAsk(c *service.Config, d *service.Deps) {
	c.ForceAsk = true
	d.Authorizer = table.New(d.Store, d.Clock, table.Settings{DestructiveMax: port.ModeAskOnce, ForceAsk: true})
}

func TestTheTrifectaWarnsAndIsExplained(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	before := len(w.Events())
	w.allowAll("x")

	list := st.Ok(w.Svc.ListAgentFunctions(w.Ctx, w.admin, port.Subject{Agent: "x", Workspace: workspace}))
	want(t, list.Exposure.Exposed, true, "unflagged functions count for both legs and one writes")
	want(t, slices.Contains(list.Exposure.LegC, string(w.conn.ID)+"/create_issue"), true, "a write function is in leg c")
	want(t, slices.Contains(w.Types(before), "connector.agent.exposure_warning"), true, "the warning event")

	d := st.Ok(w.Svc.ExplainAccess(w.Ctx, w.admin, port.Subject{Agent: "x", Workspace: workspace}, w.conn.ID, "create_issue"))
	want(t, d.Exposed, true, "explain says the function is in leg c")
	want(t, d.Mode != port.ModeAsk, true, "warn only: the mode is untouched")
}

func TestForceAskTurnsLegCFunctionsIntoAsk(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){forceAsk}})
	w.acceptAll()
	w.allowAll("x")

	d := st.Ok(w.Svc.ExplainAccess(w.Ctx, w.admin, port.Subject{Agent: "x", Workspace: workspace}, w.conn.ID, "create_issue"))
	want(t, d.Mode, port.ModeAsk, "a leg c function is held every time")
	want(t, slices.Contains(d.Notes, port.NoteForcedAsk), true, "forced_ask note")
}
