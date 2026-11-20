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
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/memory"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

type fakeChannel struct {
	kind     string
	prompts  chan surface.Prompt
	settled  chan surface.Outcome
	mu       sync.Mutex
	click    surface.ClickFunc
	once     sync.Once
	listened chan struct{}
	settings map[string]string
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
	eng  *console.Engine
	path string
	keys port.SecretStore
}

func newRig(t *testing.T) *rig {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, func(c *config.Config) { c.Approval.HoldWindow, c.Gateway.Skills = 10*time.Second, "tools" })
	up := newUpstream(t)
	statePath := t.TempDir() + "/state.json"
	state, err := console.OpenState(statePath, "owner")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", ServicePerson: "console"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	keys := memory.New()
	box := &sealed.Box{}
	box.Unlock(keys)
	act, err := console.OpenActivity(t.TempDir()+"/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeChannel{kind: "telegram", prompts: make(chan surface.Prompt, 8), settled: make(chan surface.Outcome, 8), listened: make(chan struct{})}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Activity: act, Secrets: box,
		NewChannel: func(_ string, s map[string]string) (surface.Channel, error) {
			fake.mu.Lock()
			fake.settings = s
			fake.mu.Unlock()
			return fake, nil
		}, Fetch: (&fixtureCatalog{t: t}).get}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.RunNotifications(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done; srv.Close() })
	jar, _ := cookiejar.New(nil)
	r := &rig{t: t, base: base, b: &browser{t: t, base: base, client: &http.Client{Jar: jar}}, up: up, fake: fake, eng: eng, path: statePath, keys: keys}
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

func (r *rig) allowed(scopeKind, scopeID, connector, function string) bool {
	var rows struct{ Items []port.Grant }
	if err := r.eng.Read(context.Background(), "GET", "/v1/grants?scope_kind="+scopeKind+"&scope_id="+scopeID+"&connector="+connector, nil, &rows); err != nil {
		r.t.Fatal(err)
	}
	return slices.ContainsFunc(rows.Items, func(g port.Grant) bool {
		return g.Selector == port.SelectorFunction+function && g.Mode == port.ModeAllow
	})
}

func (r *rig) eventually(what string, ok func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			r.t.Fatalf("%s did not happen", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *rig) approveFirstPending() {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var held struct{ Approvals []struct{ ID string } }
		r.b.ok("GET", "/api/approvals", nil, &held)
		if len(held.Approvals) > 0 {
			r.b.ok("POST", "/api/approvals/"+held.Approvals[0].ID+"/decide", map[string]any{"choice": "once"}, nil)
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
	r.fake.mu.Lock()
	got := r.fake.settings
	r.fake.mu.Unlock()
	if got["token"] != token || got["owner_user_id"] != "42" {
		t.Fatalf("the channel was built without its secret: %v", got)
	}
	cs := r.connect("", nil)

	t.Run("allow once", func(t *testing.T) {
		pending := r.call(cs, "create", map[string]any{"title": "secret plan"})
		p := within(t, r.fake.prompts, 5*time.Second)
		if p.Connector != "Notes" || p.Function != "create" || p.App != "claude-code" || !slices.Contains(p.Choices, surface.ChoiceSession) || !strings.HasSuffix(p.ConsoleURL, "/approve/"+p.Approval) {
			t.Fatalf("prompt %+v", p)
		}
		if left := time.Until(p.SessionUntil); left <= 0 || left > 8*time.Hour || !strings.HasPrefix(surface.Label(surface.ChoiceSession, p.SessionUntil), "Allow for up to ") {
			t.Fatalf("the session choice does not say how long it lasts: until %v, %q", p.SessionUntil, surface.Label(surface.ChoiceSession, p.SessionUntil))
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
		r.eventually("create turning on", func() bool { return r.allowed("workspace", "default", r.conn.ID, "create") })
		ran(t, within(t, r.call(cs, "create", map[string]any{"title": "third"}), 5*time.Second), "create")
		within(t, r.fake.settled, 5*time.Second)
	})

	t.Run("the settings never return a secret and the state file seals it", func(t *testing.T) {
		status, raw := r.b.send("GET", "/api/notifications", nil, nil)
		if status != http.StatusOK || strings.Contains(string(raw), token) {
			t.Fatalf("%d %s", status, raw)
		}
		if state, err := os.ReadFile(r.path); err != nil || strings.Contains(string(state), "ABCDEFGHIJKLMNOP") {
			t.Fatalf("the state file holds the token in plaintext: %v", err)
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
	if !strings.Contains(paired.Command, "vsh notify --console "+r.base) {
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

	t.Run("a revoked device's link no longer decides", func(t *testing.T) {
		pending := r.call(cs, "create", map[string]any{"title": "after revoke"})
		ev := within(t, events, 8*time.Second)
		link, _ := url.Parse(ev.Link)
		var devices struct{ Devices []console.DeviceView }
		r.b.ok("GET", "/api/notifications", nil, &devices)
		r.b.ok("DELETE", "/api/notifications/devices/"+devices.Devices[0].ID, nil, nil)
		if status, _ := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "once", "token": link.Query().Get("t")}, nil); status != http.StatusUnauthorized {
			t.Fatalf("a revoked device decided: %d", status)
		}
		r.approveFirstPending()
		ran(t, within(t, pending, 8*time.Second), "create")
	})
}

func TestASkillApprovalNamesTheSkillAndAlwaysTurnsTheKeyOn(t *testing.T) {
	r := newRig(t)
	r.b.ok("POST", "/api/notifications/channels", map[string]any{"kind": "telegram", "settings": map[string]string{"token": "123456:ABCDEFGHIJKLMNOPQRST", "owner_user_id": "42"}}, nil)
	within(t, r.fake.listened, 5*time.Second)
	r.b.ok("POST", "/api/agents", map[string]any{"name": "Other"}, nil)
	link := catalogBase + "pdf.yaml"
	var pv console.Preview
	r.b.ok("POST", "/api/preview", map[string]any{"listing": link}, &pv)
	var imported struct{ Skill string }
	r.b.ok("POST", "/api/import", map[string]any{"listing": link, "digest": pv.Skill.Digest, "access": "ask"}, &imported)
	cs := r.connect("", nil)
	out := make(chan callResult, 1)
	go func() {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "skill_load", Arguments: map[string]any{"skill": "pdf"}})
		out <- callResult{res, err}
	}()
	p := within(t, r.fake.prompts, 5*time.Second)
	if p.Function != "pdf" || p.Connector != "Skill" || p.Kind != "read" || !slices.Contains(p.Choices, surface.ChoiceSession) {
		t.Fatalf("a skill prompt: %+v", p)
	}
	if o, err := r.fake.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceAlways, User: "42"}); err != nil || o.State != surface.StateAlways {
		t.Fatalf("always: %+v %v", o, err)
	}
	if got := within(t, out, 8*time.Second); got.err != nil || got.res.IsError {
		t.Fatalf("skill_load after always: %+v %v", got.res, got.err)
	}
	r.eventually("the key turning on for the skill", func() bool { return r.allowed("agent", "claude-code", imported.Skill, "skill") })
	if r.allowed("agent", "other", imported.Skill, "skill") {
		t.Fatal("always for one key turned the skill on for another key")
	}

}

func TestChannelsComeUpWhenTheConsoleSecretsUnlock(t *testing.T) {
	r := newRig(t)
	const token = "123456:ABCDEFGHIJKLMNOPQRST"
	r.b.ok("POST", "/api/notifications/channels", map[string]any{"kind": "telegram", "settings": map[string]string{"token": token, "owner_user_id": "42"}}, nil)
	within(t, r.fake.listened, 5*time.Second)

	state, err := console.OpenState(r.path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	built := make(chan map[string]string, 4)
	locked := &sealed.Box{}
	c := &console.Console{Engine: r.eng, State: state, Sessions: console.NewSessions(state), MCPURL: r.base + "/mcp", Secrets: locked,
		NewChannel: func(_ string, s map[string]string) (surface.Channel, error) {
			built <- s
			return &fakeChannel{kind: "telegram", prompts: make(chan surface.Prompt, 8), settled: make(chan surface.Outcome, 8), listened: make(chan struct{})}, nil
		}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.RunNotifications(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	select {
	case s := <-built:
		t.Fatalf("a channel was built while the secrets were locked: %v", s)
	case <-time.After(300 * time.Millisecond):
	}
	locked.Unlock(r.keys)
	if s := within(t, built, 5*time.Second); s["token"] != token {
		t.Fatalf("the channel came up without its secret: %v", s)
	}
}

func TestPairingTheSameComputerAgainGivesItANewKeyInsteadOfAnotherEntry(t *testing.T) {
	r := newRig(t)
	var first, again struct {
		Device console.DeviceView
		Key    string
	}
	r.b.ok("POST", "/api/notifications/devices", map[string]any{"name": "Linux · Chromium"}, &first)
	r.b.ok("POST", "/api/notifications/devices", map[string]any{"name": "Linux · Chromium", "id": first.Device.ID}, &again)
	var list struct{ Devices []console.DeviceView }
	r.b.ok("GET", "/api/notifications", nil, &list)
	if len(list.Devices) != 1 || again.Device.ID != first.Device.ID {
		t.Fatalf("pairing again added an entry: %+v", list.Devices)
	}
	if again.Key == first.Key {
		t.Fatal("pairing again kept the old key")
	}
	req, _ := http.NewRequest(http.MethodGet, r.base+"/api/notify/stream", nil)
	req.Header.Set("Authorization", "Bearer "+first.Key)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("the old key still opens the stream: %v %v", res, err)
	} else {
		res.Body.Close()
	}
	r.b.ok("POST", "/api/notifications/devices", map[string]any{"id": "unknown"}, nil)
	r.b.ok("GET", "/api/notifications", nil, &list)
	if len(list.Devices) != 2 {
		t.Fatalf("an unknown id must pair a new computer: %d", len(list.Devices))
	}
}

func TestAComputerPairsWithoutANameAndIsRenamedLater(t *testing.T) {
	r := newRig(t)
	var paired struct{ Device console.DeviceView }
	r.b.ok("POST", "/api/notifications/devices", map[string]any{}, &paired)
	if paired.Device.Name != "This computer" {
		t.Fatalf("default name %q", paired.Device.Name)
	}
	r.b.ok("PATCH", "/api/notifications/devices/"+paired.Device.ID, map[string]any{"name": "  Linux · Firefox "}, nil)
	var list struct{ Devices []console.DeviceView }
	r.b.ok("GET", "/api/notifications", nil, &list)
	if len(list.Devices) != 1 || list.Devices[0].Name != "Linux · Firefox" {
		t.Fatalf("renamed %+v", list.Devices)
	}
	if status, _ := r.b.send("PATCH", "/api/notifications/devices/nope", map[string]any{"name": "x"}, nil); status != http.StatusNotFound {
		t.Fatalf("an unknown device: %d", status)
	}
	if status, _ := r.b.send("PATCH", "/api/notifications/devices/"+paired.Device.ID, map[string]any{"name": strings.Repeat("x", 61)}, nil); status != http.StatusBadRequest {
		t.Fatalf("a long name: %d", status)
	}
}
