package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/cli/tools"
	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

type cli struct {
	t   *testing.T
	cmd *tools.Command
}

func (w *world) cli(agent, session string) cli {
	env := map[string]string{"CN_ENDPOINT": w.srv.URL, "CN_AGENT_TOKEN": w.token(w.claims(agent, session))}
	return cli{t: w.t, cmd: tools.New(func(k string) string { return env[k] })}
}

func (c cli) run(args ...string) (stdout string, err error) {
	c.t.Helper()
	var out, errw bytes.Buffer
	err = c.cmd.Run(context.Background(), args, &out, &errw)
	return out.String(), err
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		var list []any
		if json.Unmarshal(raw, &list) != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return map[string]any{"list": list}
	}
	return m
}

func TestCLIListSearchAndCall(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.ThresholdFunctions = 2 }})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
	w.row(port.ScopeAgent, "x", "fn:report", port.ModeAllow)
	c := w.cli("x", "s1")
	dir := t.TempDir()

	out, err := c.run("list", "--out", filepath.Join(dir, "list.json"))
	if err != nil || !strings.HasPrefix(out, "2 tools: find, call") {
		t.Fatalf("list summary: %q %v", out, err)
	}
	if got := readJSON(t, filepath.Join(dir, "list.json"))["list"].([]any); len(got) != 2 {
		t.Fatalf("list file: %v", got)
	}

	out, err = c.run("search", "customer", "--out", filepath.Join(dir, "found.json"))
	if err != nil || !strings.HasPrefix(out, "1 matches: s__report") || len(out) > 200 {
		t.Fatalf("search summary: %q %v", out, err)
	}
	found := readJSON(t, filepath.Join(dir, "found.json"))["structuredContent"].(map[string]any)["tools"].([]any)
	if len(found) != 1 || found[0].(map[string]any)["description"] != "Reports a customer" {
		t.Fatalf("search file: %v", found)
	}

	out, err = c.run("call", "s__create_issue", "--args", `{"q":"hello"}`, "--out", filepath.Join(dir, "call.json"))
	if err != nil || !strings.HasPrefix(out, "ok") || strings.Contains(out, "hello") {
		t.Fatalf("call summary: %q %v", out, err)
	}
	if got := readJSON(t, filepath.Join(dir, "call.json"))["structuredContent"].(map[string]any); got["q"] != "hello" {
		t.Fatalf("call file: %v", got)
	}
}

func TestCLICallWritesATempFileByDefaultAndProjects(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:report", port.ModeAllow)
	st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "report", true))
	out, err := w.cli("x", "s1").run("call", "s__report", "--fields", "/name,/note")
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(out[strings.LastIndex(out, " ")+1:])
	t.Cleanup(func() { _ = os.Remove(path) })
	got := readJSON(t, path)["structuredContent"].(map[string]any)
	if len(got) != 2 || got["name"] != "Ana" || got["note"] != "vip" {
		t.Fatalf("projected file %s: %v", path, got)
	}
	if strings.Contains(out, cpf) {
		t.Fatal("stdout carries the data")
	}
}

func TestCLIPendingErrorAndUsage(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	c := w.cli("x", "s1")
	dir := t.TempDir()

	out, err := c.run("call", "s__create_issue", "--args", "{}", "--out", filepath.Join(dir, "p.json"))
	if err != nil || !strings.HasPrefix(out, "pending_approval approval_id=") || !strings.Contains(out, "approval_status") {
		t.Fatalf("pending summary: %q %v", out, err)
	}
	out, err = c.run("call", "s__search", "--args", `{"q":1}`, "--out", filepath.Join(dir, "e.json"))
	if err == nil || !strings.HasPrefix(out, "error invalid_argument") {
		t.Fatalf("error summary: %q %v", out, err)
	}
	if got := readJSON(t, filepath.Join(dir, "e.json")); got["isError"] != true {
		t.Fatalf("error file: %v", got)
	}
	if _, err := c.run("call", "s__nothing", "--args", "{}"); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("unknown tool: %v", err)
	}
	for _, bad := range [][]string{{}, {"nope"}, {"call"}, {"search"}, {"call", "x", "--args", "{bad"}} {
		if _, err := c.run(bad...); err == nil {
			t.Fatalf("%v should fail", bad)
		}
	}
	noEnv := tools.New(func(string) string { return "" })
	if err := noEnv.Run(context.Background(), []string{"list"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "CN_AGENT_TOKEN") {
		t.Fatalf("missing environment: %v", err)
	}
}
