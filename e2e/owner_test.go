package e2e

import (
	"slices"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/port"
)

var scenarios = []scenario{
	{"owner_i_only_F_for_agent_X", ownerOne},
}

func ownerOne(t *testing.T, w *world) {
	k := w.newKit(t, "ti")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
	x := k.agent(t, "x", "s1")

	if got := x.names(t); !slices.Equal(got, []string{"s__create_issue"}) {
		t.Fatalf("agent X is listed %v, want only create_issue", got)
	}
	before := w.mcp.count("search")
	if res, err := x.call("s__create_issue", map[string]any{"q": "a"}); err != nil || res.IsError {
		t.Fatalf("the granted function: %v %v", res, err)
	}
	_, denied := x.call("s__search", map[string]any{})
	_, missing := x.call("s__nothing", map[string]any{})
	if denied == nil || missing == nil || !strings.Contains(denied.Error(), "unknown tool") {
		t.Fatalf("a denied function must look unknown: %v / %v", denied, missing)
	}
	if got := w.mcp.count("search") - before; got != 0 {
		t.Fatalf("a refused call reached the upstream %d times", got)
	}
	if w.mcp.count("create_issue") != 1 {
		t.Fatalf("create_issue ran %d times", w.mcp.count("create_issue"))
	}
}
