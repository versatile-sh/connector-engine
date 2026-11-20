package service_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/authz/table"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
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

func TestARelistRefreshesExposureOffTheCallPathAndWarnsWhenItEnds(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	want(t, st.Ok(w.Svc.ListAgentFunctions(w.Ctx, w.admin, port.Subject{Agent: "x", Workspace: workspace})).Exposure.Exposed, true, "exposed")
	before := len(w.Events())
	quiet, _ := st.Definition("search", "Searches", `"annotations":{"readOnlyHint":true,"openWorldHint":false}`)
	w.Runtime.SetDefinitions(listing, quiet)
	st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	deadline := time.Now().Add(2 * time.Second)
	for !slices.ContainsFunc(w.Events()[before:], func(e port.Event) bool {
		return e.Type == "connector.agent.exposure_warning" && e.Data["exposed"] == false
	}) {
		if time.Now().After(deadline) {
			t.Fatal("event 13 is emitted when the exposure ends")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestABatchedSearchAnswersLikeOneSearchPerConnector(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	sub := port.Subject{Tenant: tenant, Workspace: workspace, Agent: "x"}
	qs := []port.SearchQuery{{Subject: sub, Connector: w.conn.ID}, {Subject: port.Subject{Tenant: tenant, Workspace: workspace, Agent: "y"}, Connector: w.conn.ID}, {Subject: sub, Connector: "missing"}}
	batch := st.Ok(w.Auth.SearchBatch(w.Ctx, tenant, qs))
	for i, q := range qs {
		if one := st.Ok(w.Auth.Search(w.Ctx, q.Subject, q.Connector)); !reflect.DeepEqual(one, batch[i]) {
			t.Fatalf("query %d: batch %v, one %v", i, batch[i], one)
		}
	}
}
