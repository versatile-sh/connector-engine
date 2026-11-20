package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

const operatorToken = "console-test-operator"

type upstream struct {
	*httptest.Server
	mu    sync.Mutex
	calls map[string]int
}

func (u *upstream) count(name string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls[name]
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{calls: map[string]int{}}
	srv := mcp.NewServer(&mcp.Implementation{Name: "notes", Title: "Notes", Version: "1.2.0", Description: "A notebook"}, nil)
	yes, no := true, false
	for _, spec := range []struct {
		name string
		ann  *mcp.ToolAnnotations
	}{{"search", &mcp.ToolAnnotations{ReadOnlyHint: true}}, {"create", &mcp.ToolAnnotations{DestructiveHint: &no}}, {"purge", &mcp.ToolAnnotations{DestructiveHint: &yes}}, {"mystery", nil}} {
		srv.AddTool(&mcp.Tool{Name: spec.name, Description: "does " + spec.name, Annotations: spec.ann, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				u.mu.Lock()
				u.calls[req.Params.Name]++
				u.mu.Unlock()
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ran " + req.Params.Name}}}, nil
			})
	}
	u.Server = httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(u.Close)
	return u
}

func bootEngine(t *testing.T, publicURL string, tweaks ...func(*config.Config)) (string, string) {
	t.Helper()
	c := config.Default()
	c.Mode = config.ModeDev
	c.Server.Listen, c.Server.GRPCListen, c.Server.OperatorListen = "127.0.0.1:0", "", "127.0.0.1:0"
	c.Server.PublicURL = publicURL
	c.Auth.LocalIssuer.Enabled = true
	c.Auth.AgentTokenMaxLifetime = 720 * time.Hour
	c.Auth.Operator.TokenRef = "env:CN_OPERATOR_TOKEN"
	c.Bootstrap.Roles = []config.RoleRow{{Tenant: "local", Person: "owner", Role: "tenant_admin", Scope: "t/local"}}
	c.Runtime.AllowPrivateHosts = true
	c.Approval.HoldWindow = 300 * time.Millisecond
	c.Approval.SweepInterval = 100 * time.Millisecond
	for _, tweak := range tweaks {
		tweak(&c)
	}
	file, err := c.With(app.Core().Builtins()...).LoadAdapters([]byte("apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: dev\nports:\n  store: {use: memory}\n  secrets: {use: memory}\n  auth: {use: builtin:local}\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	a, err := app.New(ctx, c, file, app.Options{Lookup: func(k string) string {
		if k == "CN_OPERATOR_TOKEN" {
			return operatorToken
		}
		return ""
	}})
	if err != nil {
		stop()
		t.Fatal(err)
	}
	l, err := a.Listen()
	if err != nil {
		stop()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx, l) }()
	t.Cleanup(func() { stop(); <-done; a.Close() })
	return "http://" + l.HTTP.Addr().String(), "http://" + l.Operator.Addr().String()
}

type browser struct {
	t      *testing.T
	base   string
	client *http.Client
	csrf   string
}

func (b *browser) send(method, path string, body any, headers map[string]string) (int, []byte) {
	b.t.Helper()
	var in io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		in = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.base+path, in)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", b.base)
	if b.csrf != "" {
		req.Header.Set("X-CSRF-Token", b.csrf)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (b *browser) ok(method, path string, body, out any) {
	b.t.Helper()
	status, raw := b.send(method, path, body, nil)
	if status >= 300 {
		b.t.Fatalf("%s %s: %d %s", method, path, status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			b.t.Fatalf("%s %s: %v %s", method, path, err, raw)
		}
	}
}

type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func textOf(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestTheConsoleImportsGrantsAndServesAnAgent(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base)
	up := newUpstream(t)
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Secrets: &sealed.Box{}}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}

	var sess map[string]any
	b.ok("GET", "/api/session", nil, &sess)
	if sess["owner_exists"] != false || sess["signed_in"] != false {
		t.Fatalf("first visit: %v", sess)
	}
	if status, _ := b.send("POST", "/api/setup", map[string]string{"username": "me", "password": "short"}, nil); status != http.StatusBadRequest {
		t.Fatalf("a short password: %d", status)
	}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	for _, want := range []string{"locked", "plaintext"} {
		var keys struct {
			KeyProtection string `json:"key_protection"`
		}
		b.ok("GET", "/api/session", nil, &keys)
		if keys.KeyProtection != want {
			t.Fatalf("key protection %q, want %q", keys.KeyProtection, want)
		}
		store, err := secretslocal.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		c.Secrets.Unlock(store)
	}
	if status, _ := b.send("POST", "/api/setup", map[string]string{"username": "x", "password": "another long one"}, nil); status != http.StatusConflict {
		t.Fatalf("a second owner: %d", status)
	}

	t.Run("guards", func(t *testing.T) {
		if status, _ := b.send("GET", "/api/session", nil, map[string]string{"Host": "evil.example:80"}); status != http.StatusMisdirectedRequest {
			t.Errorf("a foreign Host: %d", status)
		}
		if status, _ := b.send("POST", "/api/agents", map[string]any{"name": "x"}, map[string]string{"Origin": ""}); status != http.StatusForbidden {
			t.Errorf("no Origin: %d", status)
		}
		if status, _ := b.send("POST", "/api/agents", map[string]any{"name": "x"}, map[string]string{"Origin": "http://evil.example"}); status != http.StatusForbidden {
			t.Errorf("a foreign Origin: %d", status)
		}
		if status, _ := b.send("POST", "/api/agents", map[string]any{"name": "x"}, map[string]string{"X-CSRF-Token": "nope"}); status != http.StatusForbidden {
			t.Errorf("a wrong CSRF token: %d", status)
		}
		if status, raw := b.send("POST", "/local/token", map[string]any{"sub": "owner", "tenant": "local"}, nil); status == http.StatusOK || strings.Contains(string(raw), "token") {
			t.Errorf("the local issuer is reachable through the console: %d %s", status, raw)
		}
		if status, _ := b.send("GET", "/v1/connectors", nil, nil); status == http.StatusOK {
			t.Errorf("the engine API is reachable through the console")
		}
		_, raw := b.send("GET", "/api/session", nil, nil)
		if strings.Contains(string(raw), operatorToken) {
			t.Errorf("the operator token reached the browser")
		}
		if status, _ := b.send("POST", "/api/preview", map[string]any{"mcp": up.URL + "/mcp"}, nil); status != http.StatusBadRequest {
			t.Errorf("an http MCP without the local tick: %d", status)
		}
	})

	var pv console.Preview
	b.ok("POST", "/api/preview", map[string]any{"mcp": up.URL + "/mcp", "local": true}, &pv)
	proposed := map[string]console.Access{}
	kinds := map[string]port.Class{}
	for _, tool := range pv.Tools {
		proposed[tool.Name], kinds[tool.Name] = tool.Proposed, tool.Kind
	}
	if pv.Title != "Notes" || pv.Description != "A notebook" || proposed["search"] != console.AccessOn || proposed["create"] != console.AccessAsk || proposed["purge"] != console.AccessAsk || proposed["mystery"] != console.AccessAsk {
		t.Fatalf("preview: %+v", pv)
	}
	if kinds["search"] != port.ClassRead || kinds["create"] != port.ClassWrite || kinds["purge"] != port.ClassDestructive || kinds["mystery"] != port.ClassDestructive {
		t.Fatalf("preview kinds: %v", kinds)
	}
	if status, _ := b.send("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true, "choices": map[string]string{"create": "write"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("the old write choice: %d", status)
	}
	var imported struct{ Connector string }
	b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true, "choices": map[string]string{"create": "on", "mystery": "off"}}, &imported)

	var list struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &list)
	if len(list.Connectors) != 1 {
		t.Fatalf("connectors: %+v", list)
	}
	conn := list.Connectors[0]
	got := map[string]console.Access{}
	classes := map[string]port.Class{}
	for _, f := range conn.Functions {
		got[f.Name], classes[f.Name] = f.Access, f.Class
	}
	want := map[string]console.Access{"search": "on", "create": "on", "purge": "ask", "mystery": "off"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("access read back: %v, want %v", got, want)
		}
	}
	if classes["search"] != port.ClassRead || classes["create"] != port.ClassWrite || classes["purge"] != port.ClassWrite {
		t.Fatalf("classes by nature: %v", classes)
	}

	var tok console.AgentToken
	b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)
	if tok.Agent != "claude-code" || tok.Token == "" || tok.MCPURL != base+"/mcp" {
		t.Fatalf("agent token: %+v", tok)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		t.Fatalf("agent connect through the console: %v", err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	a := conn.Alias + "__"
	for _, n := range []string{"search", "create", "purge"} {
		if !slices.Contains(names, a+n) {
			t.Fatalf("listed %v, want %s", names, a+n)
		}
	}
	if slices.Contains(names, a+"mystery") {
		t.Fatalf("an off function is listed: %v", names)
	}
	for _, n := range []string{"search", "create"} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: a + n, Arguments: map[string]any{}})
		if err != nil || res.IsError || textOf(res) != "ran "+n {
			var d map[string]any
			_ = eng.Call(context.Background(), "POST", "/v1/access:explain", map[string]any{"workspace": "default", "agent": "claude-code", "on_behalf_of": "owner", "connector": conn.ID, "function": n}, &d)
			t.Logf("explain %v", d)
			t.Fatalf("%s: %v %v", n, res, err)
		}
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: a + "purge", Arguments: map[string]any{}})
	if err != nil || up.count("purge") != 0 {
		t.Fatalf("an ask call ran at once: %v %v", res, err)
	}
	var approvals struct {
		Approvals []struct{ ID, Function string }
	}
	b.ok("GET", "/api/approvals", nil, &approvals)
	if len(approvals.Approvals) != 1 || approvals.Approvals[0].Function != "purge" {
		t.Fatalf("approvals: %+v", approvals)
	}
	b.ok("POST", "/api/approvals/"+approvals.Approvals[0].ID, map[string]any{"approve": true}, nil)
	deadline := time.Now().Add(10 * time.Second)
	for up.count("purge") == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if up.count("purge") != 1 {
		t.Fatalf("the approved call did not run")
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: a + "purge", Arguments: map[string]any{"again": true}}); err != nil || up.count("purge") != 1 {
		t.Fatalf("approving a call in the console allowed the next one of its session: %v %v", res, err)
	}

	b.ok("PUT", "/api/connectors/"+conn.ID+"/functions/purge", map[string]string{"access": "on"}, nil)
	for _, n := range []string{"search", "create", "purge"} {
		before := up.count(n)
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: a + n, Arguments: map[string]any{"round": 2}})
		if err != nil || res.IsError || up.count(n) != before+1 {
			t.Fatalf("%s on, with a bound agent: %v %v", n, res, err)
		}
	}

	raw, err := os.ReadFile("../../source/files/testdata/notes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := console.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	m = console.Untrusted(m)
	m.Remote.URL = up.URL + "/mcp"
	second, err := c.Install(context.Background(), m, map[string]console.Access{"search": console.AccessOn}, true)
	if err != nil {
		t.Fatalf("install from a listing manifest: %v", err)
	}
	b.ok("GET", "/api/connectors", nil, &list)
	for _, cv := range list.Connectors {
		if cv.ID != second {
			continue
		}
		for _, f := range cv.Functions {
			if want := map[string]console.Access{"search": "on"}[f.Name]; want != "" && f.Access != want || want == "" && f.Access != console.AccessAsk {
				t.Errorf("listing install %s: %s", f.Name, f.Access)
			}
		}
	}

	b.ok("PUT", "/api/connectors/"+conn.ID+"/functions/create", map[string]string{"access": "off"}, nil)
	created := up.count("create")
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: a + "create", Arguments: map[string]any{}}); err == nil || up.count("create") != created {
		t.Fatalf("a function set off still runs: %v, upstream %d", err, up.count("create"))
	}
}

func TestReachRefusesPlainHTTPAndPrivateAddresses(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"http://example.com/mcp", "ftp://example.com", "https://user:pw@example.com/"} {
		if _, err := (console.Reach{}).CheckURL(raw); err == nil {
			t.Errorf("%s accepted", raw)
		}
	}
	for _, raw := range []string{"https://127.0.0.1:1/x", "https://169.254.169.254/latest", "https://10.0.0.1/x", "https://[::1]:1/x", "https://localhost:1/x"} {
		if _, _, err := (console.Reach{}).Get(ctx, raw, 10, ""); err == nil || !strings.Contains(err.Error(), "non-public") {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if _, err := (console.Reach{Local: true}).CheckURL("http://127.0.0.1:3000/mcp"); err != nil {
		t.Errorf("a ticked local URL: %v", err)
	}
}

func TestARedirectToAPrivateAddressIsRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1:9/secret", http.StatusFound)
	}))
	defer srv.Close()
	tr := console.Reach{}.Transport()
	tr.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	client := &http.Client{Transport: tr}
	_, err := client.Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("got %v", err)
	}
}

func TestAThirdPartyListingLosesWhatOnlyAReviewGives(t *testing.T) {
	raw, err := os.ReadFile("../../source/files/testdata/notes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := console.ParseManifest(raw)
	if err != nil || m.Name != "com.example/notes" || m.Review == nil || m.Functions[0].Class != "read" {
		t.Fatalf("parse: %+v %v", m, err)
	}
	u := console.Untrusted(m)
	if u.Review != nil || u.TrustLevel != "" || u.Functions[0].Class != "" || u.Functions[0].ClassSource != "" || u.Functions[0].Name != "read_note" || u.Remote == nil {
		t.Fatalf("untrusted: %+v", u)
	}
	if _, err := console.ParseManifest([]byte("name: a.b/c\nkind: mcp_remote\nsurprise: 1\n")); err == nil {
		t.Fatal("an unknown key was accepted")
	}
}

func TestIconsAreImagesNeverSVG(t *testing.T) {
	ctx := context.Background()
	png := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	if _, kind, err := console.FetchIcon(ctx, png); err != nil || kind != "image/png" {
		t.Fatalf("png: %s %v", kind, err)
	}
	svg := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjxzY3JpcHQ+YWxlcnQoMSk8L3NjcmlwdD48L3N2Zz4="
	if _, _, err := console.FetchIcon(ctx, svg); err == nil {
		t.Fatal("an SVG icon was accepted")
	}
}

func TestOnlyReadOnlyHintProposesOn(t *testing.T) {
	no := false
	cases := []struct {
		ann  *mcp.ToolAnnotations
		want console.Access
	}{{nil, console.AccessAsk}, {&mcp.ToolAnnotations{ReadOnlyHint: true}, console.AccessOn}, {&mcp.ToolAnnotations{DestructiveHint: &no}, console.AccessAsk}}
	for _, c := range cases {
		if _, _, got := console.Propose(c.ann); got != c.want {
			t.Errorf("%+v: %s", c.ann, got)
		}
	}
	if _, destructive, _ := console.Propose(nil); !destructive {
		t.Error("no hints means destructive")
	}
}

func TestTheOwnerSignsInWithThePasswordOnly(t *testing.T) {
	s, err := console.OpenState(t.TempDir()+"/s.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CheckLogin("a", "b"); err != console.ErrNoOwner {
		t.Fatalf("before setup: %v", err)
	}
	if _, err := s.CreateFirstOwner("me", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		user, pass string
		want       bool
	}{{"me", "correct horse battery", true}, {"me", "wrong password!", false}, {"you", "correct horse battery", false}} {
		if _, ok, err := s.CheckLogin(c.user, c.pass); err != nil || ok != c.want {
			t.Errorf("%s/%s: %v %v", c.user, c.pass, ok, err)
		}
	}
}

func TestASessionOutlivesAConsoleRestartAndNotItsSignOut(t *testing.T) {
	path := t.TempDir() + "/s.json"
	state, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := state.CreateFirstOwner("me", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	csrf, err := console.NewSessions(state).Start(rec, owner.Person)
	if err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), cookie.Value) {
		t.Fatal("the state file holds the raw session cookie")
	}
	reopened, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	after := console.NewSessions(reopened)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(cookie)
	if got, ok := after.Get(req); !ok || got != csrf {
		t.Fatalf("after restart: %q %v", got, ok)
	}
	after.End(httptest.NewRecorder(), req)
	again, _ := console.OpenState(path, "owner")
	if _, ok := console.NewSessions(again).Get(req); ok {
		t.Fatal("a signed-out session came back after a restart")
	}
}

func TestTheHostAllowlistFollowsTheConsolePort(t *testing.T) {
	public, _ := url.Parse("http://127.0.0.1:9123")
	g := console.Guard{Hosts: console.LoopbackHosts(public)}
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for host, want := range map[string]int{
		"127.0.0.1:9123": http.StatusNoContent, "localhost:9123": http.StatusNoContent, "[::1]:9123": http.StatusNoContent,
		"127.0.0.1:7465": http.StatusMisdirectedRequest, "evil.example:9123": http.StatusMisdirectedRequest, "127.0.0.1": http.StatusMisdirectedRequest,
	} {
		req := httptest.NewRequest("GET", "/api/session", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %s: %d, want %d", host, rec.Code, want)
		}
	}
	post := httptest.NewRequest("POST", "/api/agents", nil)
	post.Host, post.Header["Origin"] = "localhost:9123", []string{"http://127.0.0.1:9123"}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post)
	if rec.Code != http.StatusNoContent {
		t.Errorf("a same-console Origin at the custom port: %d", rec.Code)
	}
}

func initialize(t *testing.T, h http.Handler, host, token string) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestARevokedAccessKeyStopsAtOnceAndStaysRevokedAfterARestart(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := lis.Addr().String()
	base := "http://" + host
	public, operator := bootEngine(t, base)
	up := newUpstream(t)
	path := t.TempDir() + "/state.json"
	state, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	newConsole := func(s *console.State) *console.Console {
		return &console.Console{Engine: eng, State: s, Sessions: console.NewSessions(s), Guard: console.Guard{Hosts: []string{host}},
			MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour}
	}
	c := newConsole(state)
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true}, nil)

	var tok, kept console.AgentToken
	b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)
	b.ok("POST", "/api/agents", map[string]any{"name": "Codex"}, &kept)
	h := c.Handler()
	if got := initialize(t, h, host, tok.Token); got != http.StatusOK {
		t.Fatalf("a fresh key: %d", got)
	}
	if status, _ := b.send("DELETE", "/api/agents/claude-code", nil, map[string]string{"X-CSRF-Token": ""}); status != http.StatusForbidden {
		t.Fatalf("a revoke without the CSRF token: %d", status)
	}
	if status, _ := b.send("DELETE", "/api/agents/nobody", nil, nil); status != http.StatusNotFound {
		t.Fatalf("revoking an unknown key: %d", status)
	}
	b.ok("DELETE", "/api/agents/claude-code", nil, nil)

	if got := initialize(t, h, host, tok.Token); got != http.StatusUnauthorized {
		t.Fatalf("a revoked key on /mcp: %d", got)
	}
	if got := initialize(t, h, host, kept.Token); got != http.StatusOK {
		t.Fatalf("another key after a revoke: %d", got)
	}
	var agents struct{ Agents []console.Agent }
	b.ok("GET", "/api/agents", nil, &agents)
	if len(agents.Agents) != 1 || agents.Agents[0].Name != "codex" {
		t.Fatalf("keys after a revoke: %+v", agents.Agents)
	}
	var grants struct{ Items []port.Grant }
	if err := eng.Call(context.Background(), http.MethodGet, "/v1/grants?scope_kind=agent&scope_id=claude-code", nil, &grants); err != nil || len(grants.Items) != 0 {
		t.Fatalf("grants of a revoked agent: %+v %v", grants.Items, err)
	}

	reopened, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if got := initialize(t, newConsole(reopened).Handler(), host, tok.Token); got != http.StatusUnauthorized {
		t.Fatalf("a revoked key after a console restart: %d", got)
	}
}

func TestTheFirstRunGuideStaysClosedOnceDismissedOrFinished(t *testing.T) {
	for _, finish := range []bool{false, true} {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		host := lis.Addr().String()
		base := "http://" + host
		public, operator := bootEngine(t, base)
		up := newUpstream(t)
		path := t.TempDir() + "/state.json"
		state, err := console.OpenState(path, "owner")
		if err != nil {
			t.Fatal(err)
		}
		eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
		if err := eng.EnsureTenant(context.Background()); err != nil {
			t.Fatal(err)
		}
		c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{host}}, MCPURL: base + "/mcp", MaxAgentTTL: time.Hour}
		srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
		srv.Start()
		jar, _ := cookiejar.New(nil)
		b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
		var setup struct{ CSRF string }
		b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
		b.csrf = setup.CSRF
		dismissed := func() bool {
			var s struct {
				Dismissed bool `json:"onboarding_dismissed"`
			}
			b.ok("GET", "/api/session", nil, &s)
			return s.Dismissed
		}
		if dismissed() {
			t.Fatal("a new console starts with the guide closed")
		}
		if finish {
			b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true}, nil)
			if dismissed() {
				t.Fatal("the guide closed with a step left")
			}
			b.ok("POST", "/api/agents", map[string]any{"name": "cursor"}, nil)
		} else {
			if status, _ := b.send("POST", "/api/onboarding/dismiss", nil, map[string]string{"X-CSRF-Token": ""}); status != http.StatusForbidden {
				t.Fatalf("a dismiss without the CSRF token: %d", status)
			}
			b.ok("POST", "/api/onboarding/dismiss", nil, nil)
		}
		if !dismissed() {
			t.Fatalf("finish %v: the guide is still open", finish)
		}
		if finish {
			raw, _ := os.ReadFile(path)
			if err := os.WriteFile(path, bytes.Replace(raw, []byte(`"onboarding_dismissed": true`), []byte(`"onboarding_dismissed": false`), 1), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := console.OpenState(path, "owner")
			if err != nil || before.OnboardingDismissed() {
				t.Fatalf("a state from before the guide was latched: %v", err)
			}
			c.State = before
			b.ok("DELETE", "/api/agents/cursor", nil, nil)
			if !before.OnboardingDismissed() {
				t.Fatal("revoking the only key of a finished guide opened it again")
			}
		}
		srv.Close()
		reopened, err := console.OpenState(path, "owner")
		if err != nil || !reopened.OnboardingDismissed() {
			t.Fatalf("finish %v: the guide came back after a restart: %v", finish, err)
		}
	}
}

func TestAFailedStateWriteChangesNothing(t *testing.T) {
	dir := t.TempDir()
	state, err := console.OpenState(dir+"/s.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PutAgent(console.Agent{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := state.PutAgent(console.Agent{Name: "b"}); err == nil {
		t.Skip("the directory stays writable for this user")
	}
	if ok, err := state.RevokeAgent("a", time.Now()); ok || err == nil {
		t.Fatalf("a revoke that could not be written: %v %v", ok, err)
	}
	if got := state.Agents(); len(got) != 1 || got[0].Name != "a" || state.Revoked("a", 0) {
		t.Fatalf("the state changed in memory: %+v", got)
	}
}
