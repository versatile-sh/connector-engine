package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

type cliRun struct {
	stdout string
	err    error
}

func (a agentSession) cnctl(t *testing.T, args ...string) cliRun {
	t.Helper()
	cmd := exec.Command(a.kit.w.bin.cnctl, append([]string{"tools"}, args...)...)
	cmd.Env = append(os.Environ(), "CN_ENDPOINT="+a.kit.w.eng.mcpURL(), "CN_AGENT_TOKEN="+a.token)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return cliRun{stdout: out.String(), err: err}
}

func readJSONFile(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

var approvalIDInSummary = regexp.MustCompile(`approval_id=(\S+)`)

func cliTools(t *testing.T, w *world) {
	k := w.newKit(t, "tcli")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:customer", port.ModeAllow)
	k.grant(t, port.ScopeAgent, "x", "fn:search", port.ModeAsk)
	k.admin(t).call(t, nil, "setProjection", map[string]any{"on": true}, string(k.conn), "customer")
	x := k.agent(t, "x", "s1")
	dir := t.TempDir()

	listFile := filepath.Join(dir, "list.json")
	out := x.cnctl(t, "list", "--out", listFile)
	var listed []struct{ Name string }
	readJSONFile(t, listFile, &listed)
	if out.err != nil || !strings.Contains(out.stdout, "s__customer") || !strings.Contains(out.stdout, listFile) || len(listed) < 2 {
		t.Fatalf("tools list: %v %q %v", out.err, out.stdout, listed)
	}

	searchFile := filepath.Join(dir, "search.json")
	out = x.cnctl(t, "search", "customer", "--out", searchFile)
	var found struct {
		StructuredContent struct{ Tools []struct{ Name string } }
	}
	readJSONFile(t, searchFile, &found)
	if out.err != nil || !strings.Contains(out.stdout, "1 matches: s__customer") || len(found.StructuredContent.Tools) != 1 || found.StructuredContent.Tools[0].Name != "s__customer" {
		t.Fatalf("tools search: %v %q %+v", out.err, out.stdout, found)
	}

	callFile := filepath.Join(dir, "call.json")
	out = x.cnctl(t, "call", "s__customer", "--args", "{}", "--fields", "/name", "--out", callFile)
	var called struct {
		StructuredContent map[string]any
		IsError           bool
	}
	readJSONFile(t, callFile, &called)
	if out.err != nil || !strings.HasPrefix(out.stdout, "ok,") || called.IsError || len(called.StructuredContent) != 1 || called.StructuredContent["name"] != "Ana" {
		t.Fatalf("tools call: %v %q %+v", out.err, out.stdout, called)
	}

	before := w.mcp.count("search")
	heldFile := filepath.Join(dir, "held.json")
	out = x.cnctl(t, "call", "s__search", "--args", "{}", "--out", heldFile)
	m := approvalIDInSummary.FindStringSubmatch(out.stdout)
	if out.err != nil || m == nil || !strings.Contains(out.stdout, "pending_approval") || w.mcp.count("search") != before {
		t.Fatalf("a held call from the CLI: %v %q", out.err, out.stdout)
	}
	k.decide(t, m[1], port.DecisionApprove)
	var final struct{ Content []struct{ Text string } }
	statusFile := filepath.Join(dir, "status.json")
	waitFor(t, "the approved call to answer through the CLI", 15*time.Second, func() bool {
		run := x.cnctl(t, "call", "approval_status", "--args", `{"approval_id":"`+m[1]+`"}`, "--out", statusFile)
		return run.err == nil && strings.HasPrefix(run.stdout, "ok,")
	})
	readJSONFile(t, statusFile, &final)
	if len(final.Content) == 0 || !strings.Contains(final.Content[0].Text, "ran search") || w.mcp.count("search") != before+1 {
		t.Fatalf("the answer through the CLI: %+v, upstream %d", final, w.mcp.count("search")-before)
	}
}

func eventsSigned(t *testing.T, w *world) {
	k := w.newKit(t, "tevents")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	if res, err := k.agent(t, "x", "s1").call("s__search", map[string]any{}); err != nil || res.IsError {
		t.Fatalf("search: %v %v", res, err)
	}
	want := []string{"connector.connector.installed", "connector.grant.created", "connector.call.finished"}
	waitFor(t, "the signed events at the webhook", 20*time.Second, func() bool {
		var types []string
		for _, e := range w.sink.eventsFor("tevents") {
			types = append(types, e.Type)
		}
		for _, name := range want {
			if !slices.Contains(types, name) {
				return false
			}
		}
		return true
	})
	if bad := w.sink.failures(); len(bad) != 0 {
		t.Fatalf("the receiver refused deliveries: %v", bad)
	}
	seen := map[string]bool{}
	for _, e := range w.sink.eventsFor("tevents") {
		if e.SpecVersion != "1.0" || e.ID == "" || seen[e.ID] {
			t.Fatalf("a CloudEvent with a bad envelope or a repeated id: %+v", e)
		}
		seen[e.ID] = true
		if e.Type == "connector.connector.installed" && e.Subject != string(k.conn) {
			t.Fatalf("the install event is about %q, not the connector %s", e.Subject, k.conn)
		}
	}
}

func restartSurvives(t *testing.T, w *world) {
	if !w.hasPG {
		t.Skip("the restart case needs the Postgres store")
	}
	k := w.newKit(t, "trestart")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:search", port.ModeAsk)
	before := w.mcp.count("search")
	held, err := k.agent(t, "x", "s1").call("s__search", map[string]any{"q": "restart"})
	if err != nil {
		t.Fatal(err)
	}
	id := isPending(t, held)
	conn := k.conn

	w.eng.stop(t)
	w.eng = startEngine(t, w.bin, w.cfg)

	var view struct {
		Connector port.Connector `json:"connector"`
		Functions []port.Function
	}
	k.admin(t).call(t, &view, "getConnector", nil, string(conn))
	if view.Connector.ID != conn || len(view.Functions) == 0 {
		t.Fatalf("the connector after the restart: %+v", view.Connector)
	}
	var pending items[port.Approval]
	k.as(t, "alice").call(t, &pending, "listApprovals", map[string]any{"state": port.ApprovalPending})
	if len(pending.Items) != 1 || string(pending.Items[0].ID) != id {
		t.Fatalf("the pending approval did not survive: %+v", pending.Items)
	}
	if w.mcp.count("search") != before {
		t.Fatal("the held call ran during the restart")
	}
	k.decide(t, id, port.DecisionApprove)
	x := k.agent(t, "x", "s1")
	if done := x.answered(t, id); done.IsError || !strings.Contains(textOf(done), "ran search") {
		t.Fatalf("the approval decided after the restart: %v", done)
	}
	time.Sleep(600 * time.Millisecond)
	if n := w.mcp.count("search") - before; n != 1 {
		t.Fatalf("the call ran %d times across the restart, want once", n)
	}
	if got := x.names(t); !slices.Contains(got, "s__search") {
		t.Fatalf("the grants survived: %v", got)
	}
}
