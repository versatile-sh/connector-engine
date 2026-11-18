package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	st "github.com/agents-control/connector-engine/service/servicetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var eras = []string{"2026-07-28", "2025-06-18"}

const listBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"raw","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`

func eachEra(t *testing.T, fn func(t *testing.T, version string)) {
	for _, v := range eras {
		t.Run(v, func(t *testing.T) { fn(t, v) })
	}
}

func structuredOf(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(res.StructuredContent)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("structured content %s: %v", raw, err)
	}
	return m
}

func TestOwnerCaseOneFunctionForOneAgent(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		w.bind("x")
		w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
		cs, _ := w.agent("x", "s1", version)

		if got := names(t, cs); !slices.Equal(got, []string{"s__create_issue"}) {
			t.Fatalf("list: %v", got)
		}
		res, err := callTool(cs, "s__create_issue", map[string]any{"q": "a"})
		if err != nil || res.IsError {
			t.Fatalf("allowed call: %v %v", res, err)
		}
		_, denied := callTool(cs, "s__search", map[string]any{"q": "a"})
		_, missing := callTool(cs, "s__nothing", map[string]any{"q": "a"})
		if denied == nil || missing == nil {
			t.Fatalf("both must fail: %v %v", denied, missing)
		}
		if strings.ReplaceAll(denied.Error(), "s__search", "X") != strings.ReplaceAll(missing.Error(), "s__nothing", "X") || !strings.Contains(denied.Error(), "unknown tool") {
			t.Fatalf("a denied function looks like an unknown one: %q vs %q", denied, missing)
		}
		if w.Runtime.CallsTo("search") != 0 {
			t.Fatal("a denied call reached the upstream")
		}
	})
}

func TestReadOnlyAgent(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelWrite))
		w.bind("y")
		st.Ok(w.Svc.SetAgentLevel(w.Ctx, w.admin, w.conn.ID, "y", port.LevelRead))
		cs, _ := w.agent("y", "s1", version)
		if got := names(t, cs); !slices.Equal(got, []string{"s__report", "s__search"}) {
			t.Fatalf("read-only list: %v", got)
		}
		if _, err := callTool(cs, "s__create_issue", map[string]any{}); err == nil {
			t.Fatal("a write is refused to a read-only agent")
		}
	})
}

func TestAskEachTimeHoldsThenAnswers(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		w.bind("x")
		w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
		cs, _ := w.agent("x", "s1", version)
		if got := names(t, cs); !slices.Contains(got, "approval_status") || !slices.Contains(got, "approval_cancel") {
			t.Fatalf("approval tools are listed with an ask function: %v", got)
		}

		res, err := callTool(cs, "s__search", map[string]any{"q": "a"})
		if err != nil {
			t.Fatal(err)
		}
		pending := structuredOf(t, res)
		if res.IsError || pending["status"] != "pending_approval" || pending["approval_id"] == "" || pending["poll_after_seconds"] == nil || pending["expires_at"] == "" {
			t.Fatalf("pending shape: %v", pending)
		}
		id := port.ApprovalID(pending["approval_id"].(string))
		if w.Runtime.CallsTo("search") != 0 {
			t.Fatal("nothing runs before the decision")
		}

		st.Ok(w.Svc.DecideApproval(w.Ctx, w.alice, id, port.DecisionApprove, "", "test"))
		for range 2 {
			if _, err := w.Worker.Sweep(w.Ctx); err != nil {
				t.Fatal(err)
			}
		}
		status, err := callTool(cs, "approval_status", map[string]any{"approval_id": string(id)})
		if err != nil || status.IsError {
			t.Fatalf("status: %v %v", status, err)
		}
		if again, err := callTool(cs, "approval_status", map[string]any{"approval_id": string(id)}); err != nil || again.IsError {
			t.Fatalf("a delivered answer stays readable: %v %v", again, err)
		}
		if n := w.Runtime.CallsTo("search"); n != 1 {
			t.Fatalf("the call ran %d times", n)
		}
	})
}

func TestApprovalInsideTheHoldWindowReturnsTheResult(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.HoldWindow = 5 * time.Second }})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	cs, _ := w.agent("x", "s1", "2026-07-28")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			open := st.Ok(w.Svc.ListApprovals(w.Ctx, w.alice, port.ApprovalFilter{States: []port.ApprovalState{port.ApprovalPending}}))
			if len(open) > 0 {
				st.Ok(w.Svc.DecideApproval(w.Ctx, w.alice, open[0].ID, port.DecisionApprove, "", "test"))
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		for time.Now().Before(deadline) && w.Runtime.CallsTo("search") == 0 {
			_, _ = w.Worker.Sweep(w.Ctx)
			time.Sleep(5 * time.Millisecond)
		}
	}()
	res, err := callTool(cs, "s__search", map[string]any{"q": "inside"})
	wg.Wait()
	if err != nil || res.IsError {
		t.Fatalf("call: %v %v", res, err)
	}
	if got := structuredOf(t, res); got["q"] != "inside" {
		t.Fatalf("the real result comes back inside the window: %v", got)
	}
	if n := w.Runtime.CallsTo("search"); n != 1 {
		t.Fatalf("the call ran %d times", n)
	}
}

func TestApprovalCancel(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	cs, _ := w.agent("x", "s1", "2026-07-28")
	res, _ := callTool(cs, "s__search", map[string]any{"q": "a"})
	id := structuredOf(t, res)["approval_id"]
	out, err := callTool(cs, "approval_cancel", map[string]any{"approval_id": id})
	if err != nil || out.IsError || structuredOf(t, out)["status"] != "cancelled" {
		t.Fatalf("cancel: %v %v", out, err)
	}
	other, _ := w.agent("x", "s2", "2026-07-28")
	if res, err := callTool(other, "approval_status", map[string]any{"approval_id": id}); err != nil || !res.IsError {
		t.Fatalf("another session cannot read the record: %v %v", res, err)
	}
}

func TestFindAndCallAboveTheThreshold(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{cfg: func(c *gateway.Config) { c.ThresholdFunctions = 2 }})
		w.bind("x")
		w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
		w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)
		w.row(port.ScopeAgent, "x", "fn:report", port.ModeAllow)
		cs, _ := w.agent("x", "s1", version)
		if got := names(t, cs); !slices.Equal(got, []string{"find", "call"}) {
			t.Fatalf("collapsed list: %v", got)
		}
		res, err := callTool(cs, "find", map[string]any{"query": "customer"})
		if err != nil || res.IsError {
			t.Fatalf("find: %v %v", res, err)
		}
		tools := structuredOf(t, res)["tools"].([]any)
		if len(tools) == 0 || tools[0].(map[string]any)["name"] != "s__report" {
			t.Fatalf("find ranks by words: %v", tools)
		}
		if hidden, _ := callTool(cs, "find", map[string]any{"query": "repository"}); len(structuredOf(t, hidden)["tools"].([]any)) != 0 {
			t.Fatalf("find never returns a function the agent has no grant for: %v", structuredOf(t, hidden))
		}
		out, err := callTool(cs, "call", map[string]any{"name": "s__create_issue", "arguments": map[string]any{"q": "z"}})
		if err != nil || out.IsError || structuredOf(t, out)["q"] != "z" {
			t.Fatalf("call: %v %v", out, err)
		}
		if _, err := callTool(cs, "call", map[string]any{"name": "s__delete_repo", "arguments": map[string]any{}}); err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Fatalf("call of a denied function: %v", err)
		}
	})
}

func TestTokenThresholdAlsoCollapses(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.ThresholdTokens = 10 }})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAllow)
	cs, _ := w.agent("x", "s1", "2026-07-28")
	if got := names(t, cs); !slices.Equal(got[:2], []string{"find", "call"}) {
		t.Fatalf("collapsed list: %v", got)
	}
}

func TestProjectionTrimsAndFilterMasks(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		w.bind("x")
		w.row(port.ScopeAgent, "x", "fn:report", port.ModeAllow)
		cs, _ := w.agent("x", "s1", version)

		full, err := callTool(cs, "s__report", map[string]any{})
		if err != nil || full.IsError {
			t.Fatalf("report: %v %v", full, err)
		}
		raw, _ := json.Marshal(full)
		if strings.Contains(string(raw), cpf) || !strings.Contains(string(raw), "***") && !strings.Contains(string(raw), "[") {
			t.Fatalf("the filter masks the CPF: %s", raw)
		}
		if structuredOf(t, full)["note"] != "vip" {
			t.Fatalf("projection is off by default: %v", structuredOf(t, full))
		}

		st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "report", true))
		trimmed, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "s__report", Arguments: map[string]any{}, Meta: mcp.Meta{"connector-engine/fields": []string{"/name"}}})
		if err != nil || trimmed.IsError {
			t.Fatalf("projected: %v %v", trimmed, err)
		}
		got := structuredOf(t, trimmed)
		if got["name"] != "Ana" || len(got) != 1 {
			t.Fatalf("projection keeps the named path only: %v", got)
		}
		bad, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "s__report", Arguments: map[string]any{}, Meta: mcp.Meta{"connector-engine/fields": []string{"/missing"}}})
		if err != nil || !bad.IsError || structuredOf(t, bad)["code"] != string(port.CodeInvalidPath) {
			t.Fatalf("an invalid path is a tool error: %v %v", bad, err)
		}
	})
}

func TestProjectionThroughTheCallMetaTool(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.ThresholdFunctions = 1 }})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAllow)
	st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "report", true))
	cs, _ := w.agent("x", "s1", "2026-07-28")
	res, err := callTool(cs, "call", map[string]any{"name": "s__report", "arguments": map[string]any{}, "fields": []string{"/note"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %v", res, err)
	}
	if got := structuredOf(t, res); len(got) != 1 || got["note"] != "vip" {
		t.Fatalf("fields trims: %v", got)
	}
}

func TestTokenInArgumentsIsRefused(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		w.bind("x")
		w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
		cs, tok := w.agent("x", "s1", version)
		res, err := callTool(cs, "s__search", map[string]any{"q": "see " + tok})
		if err != nil || !res.IsError || structuredOf(t, res)["code"] != string(port.CodeTokenInArguments) {
			t.Fatalf("token in arguments: %v %v", res, err)
		}
		if w.Runtime.CallsTo("search") != 0 {
			t.Fatal("the call was forwarded")
		}
	})
}

func TestResultNeverCarriesTheAgentToken(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	cs, tok := w.agent("x", "s1", "2026-07-28")
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
		return port.Result{Content: json.RawMessage(`[{"type":"text","text":"echo ` + tok + `"}]`), Structured: json.RawMessage(`{"t":"` + tok + `"}`)}, nil
	})
	res, err := callTool(cs, "s__search", map[string]any{})
	raw, _ := json.Marshal(res)
	if err != nil || strings.Contains(string(raw), tok) {
		t.Fatalf("token leaked: %s %v", raw, err)
	}
}

func rawPost(t *testing.T, url, token, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestRefusedTokens(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) {
		c.ResourceMetadataURL = "https://engine.test/.well-known/oauth-protected-resource"
	}})
	body := listBody
	hdr := map[string]string{"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/list"}

	person := w.Person(tenant, "bob")
	personTok := w.token(port.Claims{Tenant: tenant, Subject: "bob", Audience: []string{audience}, ExpiresAt: st.Epoch.Add(time.Hour)})
	other := w.claims("x", "s1")
	other.Audience = []string{"https://elsewhere.test/mcp"}
	expired := w.claims("x", "s1")
	expired.ExpiresAt = st.Epoch.Add(-time.Hour)
	_ = person
	for name, tok := range map[string]string{
		"no token":       "",
		"garbage":        "not-a-token",
		"person":         personTok,
		"other audience": w.token(other),
		"expired":        w.token(expired),
	} {
		t.Run(name, func(t *testing.T) {
			resp := rawPost(t, w.srv.URL, tok, body, hdr)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status %d", resp.StatusCode)
			}
			if !strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata=") {
				t.Fatalf("WWW-Authenticate: %q", resp.Header.Get("WWW-Authenticate"))
			}
		})
	}
	if _, err := w.connect(personTok, "2026-07-28"); err == nil {
		t.Fatal("a person token cannot open a session")
	}
	if resp := rawPost(t, w.srv.URL, w.token(w.claims("x", "s1")), body, hdr); resp.StatusCode != http.StatusOK {
		t.Fatalf("an agent token passes: %d", resp.StatusCode)
	}
}

func TestViewTokenIsRefusedUnlessConfigured(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	c := w.claims("x", "s1")
	c.Caller = port.CallerView
	resp := rawPost(t, w.srv.URL, w.token(c), listBody, map[string]string{"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/list"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a view token under apps.appOnlyFunctions = refuse: %d", resp.StatusCode)
	}
	if _, err := w.connect(w.token(c), "2026-07-28"); err == nil {
		t.Fatal("a view token opens no session by default")
	}
}

func TestViewTokenIsServed(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.ViewTokens = true }})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	c := w.claims("x", "s1")
	c.Caller = port.CallerView
	cs, err := w.connect(w.token(c), "2026-07-28")
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if got := names(t, cs); !slices.Equal(got, []string{"s__search"}) {
		t.Fatalf("view list: %v", got)
	}
}

func TestRemovedTenantIsRefused(t *testing.T) {
	w := newWorld(t, options{})
	tok := w.token(w.claims("x", "s1"))
	st.Ok(w.Svc.RemoveTenant(w.Ctx, w.Operator(), tenant))
	resp := rawPost(t, w.srv.URL, tok, listBody, map[string]string{"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/list"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestResourcesPromptsAndCompletionAreRefused(t *testing.T) {
	eachEra(t, func(t *testing.T, version string) {
		w := newWorld(t, options{})
		cs, _ := w.agent("x", "s1", version)
		ctx := context.Background()
		if _, err := cs.ListResources(ctx, nil); err == nil || !strings.Contains(err.Error(), "method not found") {
			t.Fatalf("resources/list: %v", err)
		}
		if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "ui://x"}); err == nil || !strings.Contains(err.Error(), "method not found") {
			t.Fatalf("resources/read: %v", err)
		}
		if _, err := cs.ListPrompts(ctx, nil); err == nil || !strings.Contains(err.Error(), "method not found") {
			t.Fatalf("prompts/list: %v", err)
		}
		if _, err := cs.Complete(ctx, &mcp.CompleteParams{Ref: &mcp.CompleteReference{Type: "ref/prompt", Name: "p"}, Argument: mcp.CompleteParamsArgument{Name: "a", Value: "b"}}); err == nil || !strings.Contains(err.Error(), "method not found") {
			t.Fatalf("completion/complete: %v", err)
		}
	})
}

func TestListCarriesPrivateCacheScopeAndTTL(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	tok := w.token(w.claims("x", "s1"))
	resp := rawPost(t, w.srv.URL, tok, listBody, map[string]string{"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/list"})
	var body struct {
		Result struct {
			TTLMs      int    `json:"ttlMs"`
			CacheScope string `json:"cacheScope"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Result.CacheScope != "private" || body.Result.TTLMs != 60000 {
		t.Fatalf("cache fields: %+v %v", body, err)
	}
}

func TestToolsChangedNotifiesASession(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("x")
	notified := make(chan struct{}, 4)
	c := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, &mcp.ClientOptions{ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { notified <- struct{}{} }})
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: w.srv.URL, HTTPClient: &http.Client{Transport: bearer{w.token(w.claims("x", "s1"))}}}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	time.Sleep(100 * time.Millisecond)
	w.gw.ToolsChanged()
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("no notifications/tools/list_changed")
	}
}
