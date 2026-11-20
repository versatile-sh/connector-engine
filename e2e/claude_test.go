package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

type claudeAgent struct {
	config, dir string
}

func newClaudeAgent(t *testing.T, mcpURL, token string) claudeAgent {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "mcp.json")
	writeJSON(t, cfg, map[string]any{"mcpServers": map[string]any{"connector": map[string]any{
		"type": "http", "url": mcpURL, "headers": map[string]string{"Authorization": "Bearer " + token},
	}}})
	return claudeAgent{config: cfg, dir: dir}
}

type claudeRun struct {
	result  string
	tools   []string
	results []string
}

func (r claudeRun) called(name string) bool {
	return slices.ContainsFunc(r.tools, func(n string) bool { return strings.HasSuffix(n, "__"+name) })
}

func (r claudeRun) sawResult(text string) bool {
	return slices.ContainsFunc(r.results, func(s string) bool { return strings.Contains(s, text) })
}

func (c claudeAgent) ask(t *testing.T, prompt string) claudeRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt,
		"--mcp-config", c.config, "--strict-mcp-config", "--setting-sources", "project",
		"--model", "haiku", "--allowedTools", "mcp__connector__*", "--no-session-persistence", "--output-format", "stream-json", "--verbose")
	cmd.Dir = c.dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("claude -p: %v\n%s", err, out)
	}
	var run claudeRun
	for _, line := range bytes.Split(out, []byte("\n")) {
		var ev struct {
			Type    string `json:"type"`
			Result  string `json:"result"`
			Message struct {
				Content []struct {
					Type    string          `json:"type"`
					Name    string          `json:"name"`
					Content json.RawMessage `json:"content"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Type == "result" {
			run.result = ev.Result
		}
		for _, part := range ev.Message.Content {
			switch part.Type {
			case "tool_use":
				run.tools = append(run.tools, part.Name)
			case "tool_result":
				run.results = append(run.results, string(part.Content))
			}
		}
	}
	t.Logf("claude called %v and said: %s", run.tools, run.result)
	return run
}

func TestClaudeCode(t *testing.T) {
	if os.Getenv("CN_CLAUDE") != "1" {
		t.Skip("set CN_CLAUDE=1 to drive a real Claude Code session (spends tokens)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not on PATH")
	}
	w := &world{t: t, bin: build(t), tenants: map[string]bool{}}
	w.sink, w.mcp = newSink(t), newMCPUpstream(t, issueTools()...)
	dir := t.TempDir()
	w.cfg = settings{dir: filepath.Join(dir, "engine"), sinkURL: w.sink.URL, filesDir: filepath.Join(dir, "files"), openAPIDir: filepath.Join(dir, "openapi"), grpcTarget: "127.0.0.1:1", tenants: []string{"tclaude"}}
	for _, d := range []string{w.cfg.filesDir, w.cfg.openAPIDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(t, filepath.Join(w.cfg.filesDir, "issues.json"), remoteListing("acme.dev/issues", "1.0.0", w.mcp.url()))
	w.eng = startEngine(t, w.bin, w.cfg)
	t.Cleanup(func() { w.eng.stop(t) })
	w.operator().call(t, nil, "syncSource", nil, "files")

	k := w.newKit(t, "tclaude")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "cc")
	k.grant(t, port.ScopeAgent, "cc", "fn:search", port.ModeAllow)
	k.grant(t, port.ScopeAgent, "cc", "fn:create_issue", port.ModeAsk)
	token := w.eng.agent(t, agentID{tenant: "tclaude", workspace: workspace, agent: "cc", session: "claude-1", onBehalfOf: "alice"})
	cc := newClaudeAgent(t, w.eng.mcpURL(), token)

	t.Run("lists only what it was granted and calls a read", func(t *testing.T) {
		run := cc.ask(t, "Using the tools of the connector MCP server: first list the exact names of every tool that server gives you, one per line. Then call s__search with q set to hello and quote its text result exactly.")
		if n := w.mcp.count("search"); n != 1 || !run.called("s__search") || !run.sawResult("ran search") {
			t.Fatalf("upstream search ran %d times; claude called %v and saw %v", n, run.tools, run.results)
		}
		if strings.Contains(run.result, "delete_repo") || strings.Contains(run.result, "s__customer") {
			t.Fatalf("an ungranted function reached claude: %s", run.result)
		}
	})

	t.Run("a held write is approved by a person and runs once", func(t *testing.T) {
		admin, approver := k.admin(t), k.as(t, "alice")
		decided := make(chan string, 1)
		go func() {
			ctx := context.Background()
			for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
				var pending items[port.Approval]
				if raw, _, err := admin.run(ctx, "listApprovals", map[string]any{"state": "pending"}); err != nil || json.Unmarshal(raw, &pending) != nil || len(pending.Items) == 0 {
					continue
				}
				id := string(pending.Items[0].ID)
				time.Sleep(5 * time.Second)
				if _, _, err := approver.run(ctx, "decideApproval", map[string]any{"decision": string(port.DecisionApprove)}, id); err == nil {
					decided <- id
					return
				}
			}
			decided <- ""
		}()
		run := cc.ask(t, "Using the connector MCP server, call s__create_issue with title set to from-claude. If the result says the call is pending approval, call approval_status with that approval id, waiting a few seconds between tries, until it is no longer pending (at most 12 tries). Then quote the final text result exactly.")
		if id := <-decided; id == "" {
			t.Fatal("no pending approval appeared")
		}
		if n := w.mcp.count("create_issue"); n != 1 {
			t.Fatalf("upstream create_issue ran %d times, want exactly 1", n)
		}
		if !run.sawResult("pending_approval") || !run.called("approval_status") || !run.sawResult("ran create_issue") {
			t.Fatalf("claude did not go through pending, approval_status and the result: called %v, saw %v", run.tools, run.results)
		}
	})

	t.Run("an ungranted function cannot be called", func(t *testing.T) {
		run := cc.ask(t, "Using the connector MCP server, try to call s__delete_repo with no arguments, then say what happened.")
		if run.sawResult("ran delete_repo") {
			t.Fatalf("claude saw a delete_repo result: %v", run.results)
		}
		if n := w.mcp.count("delete_repo"); n != 0 {
			t.Fatalf("upstream delete_repo ran %d times", n)
		}
	})
}
