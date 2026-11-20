package console_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/console"
	"github.com/agents-control/connector-engine/surface"
)

type fakeChannel struct {
	kind     string
	prompts  chan surface.Prompt
	settled  chan surface.Outcome
	mu       sync.Mutex
	click    surface.ClickFunc
	once     sync.Once
	listened chan struct{}
}

func (f *fakeChannel) Kind() string { return f.kind }

func (f *fakeChannel) Notify(_ context.Context, p surface.Prompt) (surface.Ref, error) {
	f.prompts <- p
	return surface.Ref("m-" + p.Approval), nil
}

func (f *fakeChannel) Settle(_ context.Context, _ surface.Ref, _ surface.Prompt, o surface.Outcome) error {
	f.settled <- o
	return nil
}

func (f *fakeChannel) Listen(ctx context.Context, click surface.ClickFunc) error {
	f.mu.Lock()
	f.click = click
	f.mu.Unlock()
	f.once.Do(func() { close(f.listened) })
	<-ctx.Done()
	return nil
}

func (f *fakeChannel) press(ctx context.Context, c surface.Click) (surface.Outcome, error) {
	f.mu.Lock()
	click := f.click
	f.mu.Unlock()
	return click(ctx, c)
}

func within[T any](t *testing.T, ch <-chan T, d time.Duration) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("nothing within %s", d)
	}
	var zero T
	return zero
}

type rig struct {
	t    *testing.T
	base string
	b    *browser
	up   *upstream
	conn console.ConnectorSummary
	tok  console.AgentToken
	fake *fakeChannel
}

func newRig(t *testing.T) *rig {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, func(c *config.Config) { c.Approval.HoldWindow = 10 * time.Second })
	up := newUpstream(t)
	state, err := console.OpenState(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake := &fakeChannel{kind: "telegram", prompts: make(chan surface.Prompt, 8), settled: make(chan surface.Outcome, 8), listened: make(chan struct{})}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour,
		NewChannel: func(string, map[string]string) (surface.Channel, error) { return fake, nil }}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.RunNotifications(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done; srv.Close() })
	jar, _ := cookiejar.New(nil)
	r := &rig{t: t, base: base, b: &browser{t: t, base: base, client: &http.Client{Jar: jar}}, up: up, fake: fake}
	var setup struct{ CSRF string }
	r.b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	r.b.csrf = setup.CSRF
	r.b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true, "choices": map[string]string{"search": "on", "create": "ask", "purge": "ask"}}, nil)
	var list struct{ Connectors []console.ConnectorSummary }
	r.b.ok("GET", "/api/connectors", nil, &list)
	r.conn = list.Connectors[0]
	r.b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &r.tok)
	return r
}

func (r *rig) connect(version string, elicit func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientSession {
	r.t.Helper()
	var opts *mcp.ClientOptions
	if elicit != nil {
		opts = &mcp.ClientOptions{ElicitationHandler: elicit}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, opts)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: r.tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(r.tok.Token)}},
		&mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { cs.Close() })
	return cs
}

type callResult struct {
	res *mcp.CallToolResult
	err error
}

func (r *rig) call(cs *mcp.ClientSession, function string, args map[string]any) <-chan callResult {
	out := make(chan callResult, 1)
	go func() {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: r.conn.Alias + "__" + function, Arguments: args})
		out <- callResult{res, err}
	}()
	return out
}

func (r *rig) access(function string) console.Access {
	var list struct{ Connectors []console.ConnectorSummary }
	r.b.ok("GET", "/api/connectors", nil, &list)
	for _, f := range list.Connectors[0].Functions {
		if f.Name == function {
			return f.Access
		}
	}
	return ""
}

func (r *rig) approveFirstPending() {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var held struct{ Approvals []struct{ ID string } }
		r.b.ok("GET", "/api/approvals", nil, &held)
		if len(held.Approvals) > 0 {
			r.b.ok("POST", "/api/approvals/"+held.Approvals[0].ID, map[string]any{"approve": true}, nil)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.t.Fatal("no approval appeared")
}

func ran(t *testing.T, got callResult, function string) {
	t.Helper()
	if got.err != nil || got.res.IsError || textOf(got.res) != "ran "+function {
		t.Fatalf("%s: %+v %v", function, got.res, got.err)
	}
}

func TestAChatButtonDecidesAHeldCallAndTheCallResumes(t *testing.T) {
	r := newRig(t)
	const token = "123456:ABCDEFGHIJKLMNOPQRST"
	var view console.ChannelView
	r.b.ok("POST", "/api/notifications/channels", map[string]any{"kind": "telegram", "label": "phone", "settings": map[string]string{"token": token, "owner_user_id": "42"}}, &view)
	if view.SecretHint != "QRST" || view.Owner != "42" || !view.Interactive {
		t.Fatalf("channel view %+v", view)
	}
	within(t, r.fake.listened, 5*time.Second)
	cs := r.connect("", nil)

	t.Run("allow once", func(t *testing.T) {
		pending := r.call(cs, "create", map[string]any{"title": "secret plan"})
		p := within(t, r.fake.prompts, 5*time.Second)
		if p.Connector != "Notes" || p.Function != "create" || p.App != "claude-code" || !slices.Contains(p.Choices, surface.ChoiceSession) || !strings.HasSuffix(p.ConsoleURL, "/approve/"+p.Approval) {
			t.Fatalf("prompt %+v", p)
		}
		if raw, _ := json.Marshal(p); strings.Contains(string(raw), "secret plan") {
			t.Fatalf("the prompt carries the arguments: %s", raw)
		}
		o, err := r.fake.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "42"})
		if err != nil || o.State != surface.StateApproved || o.Channel != "telegram" || o.By != "telegram:42" {
			t.Fatalf("click outcome %+v %v", o, err)
		}
		ran(t, within(t, pending, 8*time.Second), "create")
		if r.up.count("create") != 1 {
			t.Fatalf("upstream ran %d times", r.up.count("create"))
		}
		if s := within(t, r.fake.settled, 5*time.Second); s.State != surface.StateApproved {
			t.Fatalf("settled %+v", s)
		}
		if _, err := r.fake.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "42"}); err == nil {
			t.Fatal("a second click on a settled prompt decided")
		}
	})

	t.Run("always allow flips the function on", func(t *testing.T) {
		pending := r.call(cs, "create", map[string]any{"title": "second"})
		p := within(t, r.fake.prompts, 5*time.Second)
		if o, err := r.fake.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceAlways, User: "42"}); err != nil || o.State != surface.StateAlways {
			t.Fatalf("always %+v %v", o, err)
		}
		ran(t, within(t, pending, 8*time.Second), "create")
		if a := r.access("create"); a != console.AccessOn {
			t.Fatalf("access after always: %s", a)
		}
		ran(t, within(t, r.call(cs, "create", map[string]any{"title": "third"}), 5*time.Second), "create")
		within(t, r.fake.settled, 5*time.Second)
	})

	t.Run("the settings never return a secret", func(t *testing.T) {
		status, raw := r.b.send("GET", "/api/notifications", nil, nil)
		if status != http.StatusOK || strings.Contains(string(raw), token) {
			t.Fatalf("%d %s", status, raw)
		}
		if status, _ := r.b.send("POST", "/api/notifications/channels", map[string]any{"kind": "discord_webhook", "settings": map[string]string{"url": "https://evil.example/hook"}}, nil); status != http.StatusBadRequest {
			t.Fatalf("a webhook off discord.com: %d", status)
		}
		if status, _ := r.b.send("DELETE", "/api/notifications/channels/"+view.ID, nil, nil); status != http.StatusNoContent {
			t.Fatalf("remove: %d", status)
		}
	})
}

func streamEvents(t *testing.T, base, key string) <-chan console.HubEvent {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/notify/stream", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil || resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("stream: %v %v", resp, err)
	}
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	out := make(chan console.HubEvent, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var ev console.HubEvent
				if json.Unmarshal([]byte(data), &ev) == nil {
					out <- ev
				}
			}
		}
	}()
	return out
}

func TestADesktopLinkDecidesOnceWithoutASession(t *testing.T) {
	r := newRig(t)
	var paired struct {
		Key     string
		Command string
	}
	r.b.ok("POST", "/api/notifications/devices", map[string]any{"name": "laptop"}, &paired)
	if !strings.Contains(paired.Command, "cnctl notify --console "+r.base) {
		t.Fatalf("command %q", paired.Command)
	}
	if status, _ := (&browser{t: t, base: r.base, client: http.DefaultClient}).send("GET", "/api/notify/stream", nil, map[string]string{"Authorization": "Bearer nope"}); status != http.StatusUnauthorized {
		t.Fatalf("a wrong device key: %d", status)
	}
	events := streamEvents(t, r.base, paired.Key)
	cs := r.connect("", nil)
	pending := r.call(cs, "create", map[string]any{"title": "from the desktop"})
	ev := within(t, events, 8*time.Second)
	if ev.Type != "prompt" || ev.Prompt == nil || ev.Link == "" {
		t.Fatalf("event %+v", ev)
	}
	link, _ := url.Parse(ev.Link)
	token := link.Query().Get("t")
	anon := &browser{t: t, base: r.base, client: &http.Client{}}
	if status, _ := anon.send("GET", "/api/approvals/"+ev.Approval, nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("no session and no token: %d", status)
	}
	var view console.ApprovalView
	if status, raw := anon.send("GET", "/api/approvals/"+ev.Approval+"?t="+url.QueryEscape(token), nil, nil); status != http.StatusOK || json.Unmarshal(raw, &view) != nil || !strings.Contains(string(view.Arguments), "from the desktop") {
		t.Fatalf("the page with the link: %d %s", status, raw)
	}
	if status, _ := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "once", "token": token + "x"}, nil); status != http.StatusUnauthorized {
		t.Fatalf("a forged token: %d", status)
	}
	status, raw := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "once", "token": token}, nil)
	if status != http.StatusOK || !strings.Contains(string(raw), `"already_decided":false`) {
		t.Fatalf("decide with the link: %d %s", status, raw)
	}
	ran(t, within(t, pending, 8*time.Second), "create")
	if status, _ := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "deny", "token": token}, nil); status != http.StatusUnauthorized {
		t.Fatalf("the link was used twice: %d", status)
	}
	if ev := within(t, events, 5*time.Second); ev.Type != "settled" || ev.Outcome.Channel != "desktop" {
		t.Fatalf("settled %+v", ev)
	}
}
