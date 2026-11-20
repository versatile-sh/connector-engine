package console_test

import (
	"context"
	"errors"
	"maps"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/console"
	"github.com/agents-control/connector-engine/console/testmcp/notes"
	"github.com/agents-control/connector-engine/port"
)

func personRelays(c *config.Config) {
	c.Gateway.RelayInstructions, c.Gateway.UpstreamContent, c.Gateway.Skills = "pinned", "granted", "tools+prompts"
}

func signedIn(t *testing.T, tweaks ...func(*config.Config)) (*browser, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, tweaks...)
	state, err := console.OpenState(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	act, err := console.OpenActivity(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(act.Close)
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Activity: act}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	return b, base
}

func (b *browser) key(name string) console.AgentToken {
	b.t.Helper()
	var tok console.AgentToken
	b.ok("POST", "/api/agents", map[string]any{"name": name}, &tok)
	return tok
}

func (b *browser) connect(tok console.AgentToken) *mcp.ClientSession {
	b.t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: tok.Agent, Version: "1"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		b.t.Fatal(err)
	}
	b.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func (b *browser) agentSession(name string) *mcp.ClientSession {
	b.t.Helper()
	return b.connect(b.key(name))
}

func itemsOf(items []console.Item) map[string]console.Access {
	out := map[string]console.Access{}
	for _, it := range items {
		out[it.Name] = it.Access
	}
	return out
}

func TestTheConsoleImportsSetsAndServesConnectorItems(t *testing.T) {
	b, _ := signedIn(t, personRelays)
	up := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return notes.New(notes.Options{}) }, nil))
	t.Cleanup(up.Close)
	early := b.key("early")

	var pv console.Preview
	b.ok("POST", "/api/preview", map[string]any{"mcp": up.URL, "local": true}, &pv)
	want := map[string]console.Access{port.InstructionsItem: "on", "prompt:summarize": "off", "prompt:daily": "off", "res:file:///README.md": "on", "res:notes://{id}": "on"}
	if got := itemsOf(pv.Items); !maps.Equal(got, want) {
		t.Fatalf("SK-1 pre-fill: %v", got)
	}
	for _, it := range pv.Items {
		if it.Name == port.InstructionsItem && it.Preview != notes.Instructions || it.Name == "prompt:summarize" && !strings.Contains(it.Preview, "- topic (required)") {
			t.Fatalf("preview of %s: %q", it.Name, it.Preview)
		}
	}
	b.ok("POST", "/api/import", map[string]any{"mcp": up.URL, "local": true, "choices": map[string]string{"prompt:daily": "on", "search": "on"}}, nil)
	var list struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &list)
	conn := list.Connectors[0]
	want["prompt:daily"] = "on"
	if got := itemsOf(conn.Items); !maps.Equal(got, want) || slices.ContainsFunc(conn.Functions, func(f console.FunctionView) bool { return port.ItemKind(f.Name) != "" }) {
		t.Fatalf("items read back: %v, functions %+v", got, conn.Functions)
	}
	a := conn.Alias
	for name, cs := range map[string]*mcp.ClientSession{"a key made before the import": b.connect(early), "a key made after": b.agentSession("late")} {
		if !strings.Contains(cs.InitializeResult().Instructions, "> "+notes.Instructions) {
			t.Errorf("%s: instructions not relayed: %q", name, cs.InitializeResult().Instructions)
		}
		prompts, err := cs.ListPrompts(context.Background(), nil)
		if err != nil || len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != a+"__daily" {
			t.Fatalf("%s: prompts %+v %v", name, prompts, err)
		}
		got, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: a + "__daily"})
		if err != nil || len(got.Messages) != 2 {
			t.Fatalf("%s: daily %+v %v", name, got, err)
		}
		read, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: a + "+file:///README.md"})
		if err != nil || read.Contents[0].Text != notes.Readme {
			t.Fatalf("%s: readme %+v %v", name, read, err)
		}
	}

	if status, _ := b.send("PUT", "/api/connectors/"+conn.ID+"/items", map[string]string{"name": port.InstructionsItem, "access": "ask"}, nil); status != http.StatusBadRequest {
		t.Fatalf("ask on instructions: %d", status)
	}
	b.ok("PUT", "/api/connectors/"+conn.ID+"/items", map[string]string{"name": "prompt:summarize", "access": "on"}, nil)
	b.ok("PUT", "/api/connectors/"+conn.ID+"/items", map[string]string{"name": "res:file:///README.md", "access": "off"}, nil)
	fresh := b.agentSession("fresh")
	if prompts, err := fresh.ListPrompts(context.Background(), nil); err != nil || len(prompts.Prompts) != 2 {
		t.Fatalf("a prompt set on is not listed: %+v %v", prompts, err)
	}
	var rpc *jsonrpc.Error
	if _, err := fresh.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: a + "+file:///README.md"}); !errors.As(err, &rpc) || rpc.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("a resource set off still reads: %v", err)
	}
	_, raw := b.send("GET", "/api/activity", nil, nil)
	if !strings.Contains(string(raw), `"function":"prompt:daily"`) || !strings.Contains(string(raw), `"function":"res:file:///README.md"`) {
		t.Fatalf("activity misses the prompt and the read: %s", raw)
	}
}
