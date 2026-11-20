package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func newPromptServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "notes", Version: "1.0.0"}, nil)
	for _, name := range []string{"lookup", "file"} {
		srv.AddTool(&mcp.Tool{Name: name, Description: "works on notes", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{"type": "object"}},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "found"}}}, nil
			})
	}
	srv.AddPrompt(&mcp.Prompt{Name: "summarise", Description: "sums up"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "sum up"}}}}, nil
	})
	u := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(u.Close)
	return u
}

func toolsOf(t *testing.T, tok console.AgentToken) []string {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		t.Fatalf("connect as %s: %v", tok.Agent, err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		if name, ok := strings.CutPrefix(tool.Name, "notes__"); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func callsOK(tok console.AgentToken, tool string) bool {
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		return false
	}
	defer cs.Close()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notes__" + tool, Arguments: map[string]any{}})
	return err == nil && !res.IsError && textOf(res) == "found"
}

func (p profiled) heldFor(agent string) string {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var out struct{ Approvals []port.Approval }
		p.b.ok("GET", "/api/approvals", nil, &out)
		for _, a := range out.Approvals {
			if string(a.Agent) == agent {
				return string(a.ID)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.t.Fatalf("no call of %s is held", agent)
	return ""
}

type profiled struct {
	*stack
	conn string
}

func (p profiled) mint(name string) console.AgentToken {
	p.t.Helper()
	var tok console.AgentToken
	p.b.ok("POST", "/api/agents", map[string]any{"name": name}, &tok)
	return tok
}

func (p profiled) summary() console.ConnectorSummary {
	p.t.Helper()
	var conns struct{ Connectors []console.ConnectorSummary }
	p.b.ok("GET", "/api/connectors", nil, &conns)
	for _, c := range conns.Connectors {
		if c.ID == p.conn {
			return c
		}
	}
	p.t.Fatalf("no connector %s", p.conn)
	return console.ConnectorSummary{}
}

func (p profiled) profile(agent string) console.Profile {
	p.t.Helper()
	for _, pr := range p.summary().Profiles {
		if pr.Agent == agent {
			return pr
		}
	}
	p.t.Fatalf("no profile for %s", agent)
	return console.Profile{}
}

func (p profiled) narrow(agent, name string, access console.Access) {
	p.t.Helper()
	p.b.ok("PUT", "/api/connectors/"+p.conn+"/profiles/"+agent, map[string]any{"name": name, "access": access}, nil)
}

func (p profiled) mode(mode console.ProfileMode, keys ...string) {
	p.t.Helper()
	var out struct {
		Keys []struct {
			Agent string `json:"agent"`
			Code  string `json:"code"`
		} `json:"keys"`
	}
	p.b.ok("PUT", "/api/connectors/"+p.conn+"/profiles", map[string]any{"mode": mode, "keys": keys}, &out)
	for _, k := range out.Keys {
		if k.Code != "" {
			p.t.Fatalf("mode %s for %s: %s", mode, k.Agent, k.Code)
		}
	}
}

func (p profiled) effects(agent string) map[string]console.Access {
	p.t.Helper()
	var out struct{ Effects []console.Effect }
	p.b.ok("GET", "/api/connectors/"+p.conn+"/profiles/"+agent, nil, &out)
	m := map[string]console.Access{}
	for _, e := range out.Effects {
		m[e.Name] = e.Access
	}
	return m
}

func profiledStack(t *testing.T) profiled {
	t.Helper()
	s := freshOwner(t, personRelays, func(c *config.Config) { c.Approval.HoldWindow = 10 * time.Second })
	var imported struct{ Connector string }
	s.b.ok("POST", "/api/import", map[string]any{"mcp": newPromptServer(t).URL + "/mcp", "local": true, "choices": map[string]string{"lookup": "on", "file": "on", "prompt:summarise": "on"}}, &imported)
	return profiled{stack: s, conn: imported.Connector}
}

func TestEachAccessKeyHasItsOwnPermissionsOnOneConnector(t *testing.T) {
	p := profiledStack(t)
	alpha, beta := p.mint("alpha"), p.mint("beta")

	t.Run("key A is on while key B is off on the same function", func(t *testing.T) {
		p.narrow("beta", "lookup", console.AccessOff)
		if got := toolsOf(t, alpha); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("alpha lists %v", got)
		}
		if got := toolsOf(t, beta); !slices.Equal(got, []string{"file"}) {
			t.Fatalf("beta lists %v", got)
		}
		if !callsOK(alpha, "lookup") || callsOK(beta, "lookup") {
			t.Fatal("lookup ran for beta or failed for alpha")
		}
		if pr := p.profile("beta"); pr.Mode != console.ProfileCustom || pr.Overrides["lookup"] != console.AccessOff || len(pr.Overrides) != 1 {
			t.Fatalf("beta's profile %+v", pr)
		}
		if pr := p.profile("alpha"); pr.Mode != console.ProfileFollow || len(pr.Overrides) != 0 {
			t.Fatalf("alpha's profile %+v", pr)
		}
	})

	t.Run("key B in custom waits for approval where key A runs at once", func(t *testing.T) {
		p.narrow("beta", "lookup", console.AccessAsk)
		p.narrow("beta", "prompt:summarise", console.AccessOff)
		if got := p.effects("beta"); got["lookup"] != console.AccessAsk || got["file"] != console.AccessOn || got["prompt:summarise"] != console.AccessOff {
			t.Fatalf("the engine answers for beta %v", got)
		}
		if got := p.effects("alpha"); got["lookup"] != console.AccessOn || got["prompt:summarise"] != console.AccessOn {
			t.Fatalf("the engine answers for alpha %v", got)
		}
		if got := toolsOf(t, beta); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("beta lists %v", got)
		}
		ran := make(chan bool, 1)
		go func() { ran <- callsOK(beta, "lookup") }()
		held := p.heldFor("beta")
		if !callsOK(alpha, "lookup") {
			t.Fatal("alpha's call waited too")
		}
		p.b.ok("POST", "/api/approvals/"+held+"/decide", map[string]string{"choice": "always"}, nil)
		if !<-ran {
			t.Fatal("beta's held call did not run once approved")
		}
		deadline := time.Now().Add(10 * time.Second)
		for p.profile("beta").Overrides["lookup"] != "" {
			if time.Now().After(deadline) {
				t.Fatalf("always allow left beta's override: %+v", p.profile("beta"))
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("a key cannot be given more than the connector allows", func(t *testing.T) {
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/functions/file", map[string]string{"access": "ask"}, nil)
		if status, raw := p.b.send("PUT", "/api/connectors/"+p.conn+"/profiles/alpha", map[string]any{"name": "file", "access": "on"}, nil); status != http.StatusBadRequest {
			t.Fatalf("a wider override: %d %s", status, raw)
		}
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/functions/file", map[string]string{"access": "on"}, nil)
	})

	t.Run("choosing the connector's own access clears the override", func(t *testing.T) {
		p.narrow("beta", "lookup", console.AccessOn)
		p.narrow("beta", "prompt:summarise", console.AccessOn)
		if pr := p.profile("beta"); pr.Mode != console.ProfileFollow || len(pr.Overrides) != 0 {
			t.Fatalf("beta's profile %+v", pr)
		}
	})

	t.Run("a key off is unbound and lists none of the connector's tools", func(t *testing.T) {
		p.narrow("beta", "lookup", console.AccessOff)
		p.mode(console.ProfileOff, "beta")
		if got := toolsOf(t, beta); len(got) != 0 {
			t.Fatalf("beta lists %v", got)
		}
		if callsOK(beta, "file") {
			t.Fatal("an unbound key ran a tool")
		}
		if pr := p.profile("beta"); pr.Mode != console.ProfileOff || len(pr.Overrides) != 0 {
			t.Fatalf("beta's profile %+v", pr)
		}
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/items", map[string]string{"name": "prompt:summarise", "access": "on"}, nil)
		renewed := p.mint("beta")
		if got := toolsOf(t, renewed); len(got) != 0 || p.profile("beta").Mode != console.ProfileOff {
			t.Fatalf("after an item change and a renewal beta lists %v", got)
		}
		p.mode(console.ProfileFollow, "beta")
		if got := toolsOf(t, renewed); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("beta following lists %v", got)
		}
	})

	t.Run("a renewed key keeps its overrides", func(t *testing.T) {
		p.narrow("beta", "lookup", console.AccessOff)
		renewed := p.mint("beta")
		if got := toolsOf(t, renewed); !slices.Equal(got, []string{"file"}) {
			t.Fatalf("renewed beta lists %v", got)
		}
	})

	t.Run("a key that follows tracks the connector's changes", func(t *testing.T) {
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/functions/file", map[string]string{"access": "off"}, nil)
		if got := toolsOf(t, alpha); !slices.Equal(got, []string{"lookup"}) {
			t.Fatalf("alpha lists %v after file went off", got)
		}
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/functions/file", map[string]string{"access": "on"}, nil)
		if got := toolsOf(t, alpha); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("alpha lists %v after file came back", got)
		}
	})

	t.Run("one request sets every key and reports each", func(t *testing.T) {
		p.mode(console.ProfileFollow)
		for _, pr := range p.summary().Profiles {
			if pr.Mode != console.ProfileFollow {
				t.Fatalf("after following all: %+v", pr)
			}
		}
		var out struct {
			Keys []struct {
				Agent string `json:"agent"`
				Code  string `json:"code"`
			} `json:"keys"`
		}
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/profiles", map[string]any{"mode": "off", "keys": []string{"alpha", "nobody"}}, &out)
		if len(out.Keys) != 2 || out.Keys[0].Code != "" || out.Keys[1].Code != "not_found" {
			t.Fatalf("results %+v", out.Keys)
		}
		p.mode(console.ProfileFollow)
	})

	t.Run("new keys start off where the connector says so", func(t *testing.T) {
		p.b.ok("PUT", "/api/connectors/"+p.conn+"/new-keys", map[string]bool{"off": true}, nil)
		gamma := p.mint("gamma")
		if got := toolsOf(t, gamma); len(got) != 0 || p.profile("gamma").Mode != console.ProfileOff || !p.summary().NewKeysOff {
			t.Fatalf("a new key lists %v", got)
		}
		if got := toolsOf(t, alpha); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("an existing key lists %v", got)
		}
	})
}

func TestAKeyThatHadExpiredFollowsAConnectorAddedMeanwhile(t *testing.T) {
	p := profiledStack(t)
	if err := p.c.State.PutAgent(console.Agent{Name: "old", ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var imported struct{ Connector string }
	p.b.ok("POST", "/api/import", map[string]any{"mcp": newToolServer(t, "maps", "route").URL + "/mcp", "local": true}, &imported)
	p.mint("old")
	p.conn = imported.Connector
	if pr := p.profile("old"); pr.Mode != console.ProfileFollow {
		t.Fatalf("the renewed key on the new connector: %+v", pr)
	}
	if got := p.effects("old"); got["route"] != console.AccessOn {
		t.Fatalf("the engine answers %v", got)
	}
}

func TestAnExpiredKeyIsTurnedOffAndStaysOffOnceRenewed(t *testing.T) {
	p := profiledStack(t)
	p.mint("live")
	for _, name := range []string{"old", "older"} {
		if err := p.c.State.PutAgent(console.Agent{Name: name, ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	var imported struct{ Connector string }
	p.b.ok("POST", "/api/import", map[string]any{"mcp": newToolServer(t, "maps", "route").URL + "/mcp", "local": true}, &imported)
	p.conn = imported.Connector
	if pr := p.profile("old"); pr.Mode != console.ProfileFollow || !pr.Expired {
		t.Fatalf("an expired key on a new connector: %+v", pr)
	}
	p.mode(console.ProfileOff, "old")
	p.mode(console.ProfileOff)
	p.mint("old")
	p.mint("older")
	for _, name := range []string{"old", "older", "live"} {
		if pr := p.profile(name); pr.Mode != console.ProfileOff {
			t.Errorf("%s after every key went off and a renewal: %+v", name, pr)
		}
	}
}
