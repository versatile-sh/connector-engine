package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/backup"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	storememory "github.com/versatile-sh/connector-engine/store/memory"
)

const (
	passphrase = "a passphrase nobody guesses"
	apiKey     = "sk-live-0123456789"
)

func newToolServer(t *testing.T, name, tool string) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1.0.0"}, nil)
	srv.AddTool(&mcp.Tool{Name: tool, Description: "finds", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "found"}}}, nil
		})
	u := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(u.Close)
	return u
}

type stack struct {
	t         *testing.T
	dir       string
	store     *storememory.Store
	c         *console.Console
	b         *browser
	base      string
	engine    chan error
	restarted chan struct{}
}

func durableEngine(t *testing.T, publicURL, dir string, store *storememory.Store, tweaks ...func(*config.Config)) (string, string, chan error) {
	t.Helper()
	c := config.Default()
	c.Mode = config.ModeDev
	c.Server.Listen, c.Server.GRPCListen, c.Server.OperatorListen, c.Server.PublicURL, c.Server.Snapshot = "127.0.0.1:0", "", "127.0.0.1:0", publicURL, true
	c.Auth.LocalIssuer.Enabled, c.Auth.AgentTokenMaxLifetime, c.Auth.Operator.TokenRef = true, 720*time.Hour, "env:CN_OPERATOR_TOKEN"
	c.Bootstrap.Roles = []config.RoleRow{{Tenant: "local", Person: "owner", Role: "tenant_admin", Scope: "t/local"}}
	c.Runtime.AllowPrivateHosts = true
	c.Gateway.Skills = "tools"
	c.Secrets.Adapter, c.Secrets.Dir = "localfile", filepath.Join(dir, "engine-secrets")
	for _, tweak := range tweaks {
		tweak(&c)
	}
	r := app.Core()
	app.Register(r, app.Stores, app.Adapter{Name: "kept"}, func(context.Context, app.Env) (port.Store, func(), error) { return store, nil, nil })
	file, err := c.With(r.Builtins()...).LoadAdapters([]byte("apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: dev\nports:\n  store: {use: builtin:kept}\n  secrets: {use: builtin:localfile}\n  auth: {use: builtin:local}\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	a, err := app.New(ctx, c, file, app.Options{Adapters: r, Lookup: func(k string) string {
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
	go func() { done <- a.Run(ctx, l); a.Close() }()
	t.Cleanup(func() { stop() })
	return "http://" + l.HTTP.Addr().String(), "http://" + l.Operator.Addr().String(), done
}

func openStack(t *testing.T, dir string, store *storememory.Store, tweaks ...func(*config.Config)) *stack {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator, done := durableEngine(t, "http://console.test", dir, store, tweaks...)
	state, err := console.OpenState(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	activity, err := console.OpenActivity(filepath.Join(dir, "activity"), nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := secretslocal.Open(filepath.Join(dir, "console-secrets"))
	if err != nil {
		t.Fatal(err)
	}
	box := &sealed.Box{}
	box.Unlock(keys)
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := &stack{t: t, dir: dir, store: store, base: base, engine: done, restarted: make(chan struct{})}
	catalog := &fixtureCatalog{t: t, tampered: map[string]string{}}
	s.c = &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Secrets: box, Activity: activity, Fetch: catalog.get,
		Restart: func() { close(s.restarted) }}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: s.c.Handler()}}
	srv.Start()
	t.Cleanup(func() { srv.Close(); activity.Close() })
	jar, _ := cookiejar.New(nil)
	s.b = &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	return s
}

func (s *stack) signIn() {
	s.t.Helper()
	var out struct{ CSRF string }
	s.b.ok("POST", "/api/login", map[string]string{"username": "me", "password": ownerPassword}, &out)
	s.b.csrf = out.CSRF
}

func (s *stack) upload(path string, file []byte, pass string) (int, []byte) {
	s.t.Helper()
	return s.uploadAs(path, file, pass, ownerPassword)
}

func (s *stack) uploadAs(path string, file []byte, pass, password string) (int, []byte) {
	s.t.Helper()
	return s.uploadForm(path, file, map[string]string{"passphrase": pass, "password": password})
}

func (s *stack) uploadForm(path string, file []byte, fields map[string]string) (int, []byte) {
	s.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "backup")
	_, _ = fw.Write(file)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", s.base+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", s.base)
	if s.b.csrf != "" {
		req.Header.Set("X-CSRF-Token", s.b.csrf)
	}
	resp, err := s.b.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (s *stack) export() []byte {
	s.t.Helper()
	status, raw := s.b.send("POST", "/api/backup/export", map[string]any{"mode": "full", "password": ownerPassword, "passphrase": passphrase, "confirm": true}, nil)
	if status != http.StatusOK {
		s.t.Fatalf("export: %d %s", status, raw)
	}
	return raw
}

func (s *stack) restoreAndRestart(file []byte) *stack {
	s.t.Helper()
	if status, raw := s.upload("/api/backup/import", file, passphrase); status != http.StatusOK {
		s.t.Fatalf("import: %d %s", status, raw)
	}
	select {
	case err := <-s.engine:
		if err != nil {
			s.t.Fatalf("the restored engine stopped with %v", err)
		}
	case <-time.After(10 * time.Second):
		s.t.Fatal("the restored engine did not stop")
	}
	<-s.restarted
	next := openStack(s.t, s.dir, s.store)
	next.signIn()
	return next
}

type view struct {
	Connectors []console.ConnectorSummary
	Skills     []console.SkillSummary
	Agents     []string
}

func (s *stack) view() view {
	s.t.Helper()
	var v view
	var conns struct{ Connectors []console.ConnectorSummary }
	s.b.ok("GET", "/api/connectors", nil, &conns)
	var skills struct{ Skills []console.SkillSummary }
	s.b.ok("GET", "/api/skills", nil, &skills)
	v.Connectors, v.Skills = conns.Connectors, skills.Skills
	for _, a := range s.c.State.Agents() {
		v.Agents = append(v.Agents, a.Name)
	}
	return v
}

func sameView(t *testing.T, want, got view) {
	t.Helper()
	w, _ := json.Marshal(want)
	g, _ := json.Marshal(got)
	if string(w) != string(g) {
		t.Fatalf("got %s\nwant %s", g, w)
	}
}

func callLookup(t *testing.T, mcpURL, token string, call bool) {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: mcpURL, HTTPClient: &http.Client{Transport: bearer(token)}}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if strings.HasSuffix(tool.Name, "__lookup") {
			if !call {
				return
			}
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{}})
			if err != nil || res.IsError {
				t.Fatalf("search: %v %s", err, textOf(res))
			}
			return
		}
	}
	t.Fatalf("no lookup tool among %d", len(tools.Tools))
}

func seeded(t *testing.T) (*stack, console.AgentToken, port.Sealed) {
	t.Helper()
	shimInterpreters(t)
	src := openStack(t, t.TempDir(), storememory.New())
	src.b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": ownerPassword}, nil)
	src.signIn()
	plain, keyed := newToolServer(t, "notes", "lookup"), newToolServer(t, "vault", "search")
	src.b.ok("POST", "/api/import", map[string]any{"mcp": plain.URL + "/mcp", "local": true}, nil)
	var imported struct{ Connector string }
	src.b.ok("POST", "/api/import", map[string]any{"mcp": keyed.URL + "/mcp", "local": true, "header_secret": true}, &imported)
	src.b.ok("POST", "/api/connectors/"+imported.Connector+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": apiKey}}, nil)
	var tok console.AgentToken
	src.b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)
	src.b.ok("PUT", "/api/connectors/"+imported.Connector+"/profiles/claude-code", map[string]any{"names": []string{"search"}, "access": "off"}, nil)
	src.b.ok("PUT", "/api/connectors/"+imported.Connector+"/new-keys", map[string]bool{"off": true}, nil)
	var pv console.Preview
	src.b.ok("POST", "/api/preview", map[string]any{"listing": catalogBase + "pdf.yaml"}, &pv)
	src.b.ok("POST", "/api/import", map[string]any{"listing": catalogBase + "pdf.yaml", "digest": pv.Skill.Digest, "access": "ask"}, nil)
	if err := src.c.State.PutOAuthClient(console.OAuthClient{ID: "client-1", Name: "An app", RedirectURIs: []string{"http://127.0.0.1:9/cb"}, LastUsed: time.Now().UTC().Truncate(time.Second)}); err != nil {
		t.Fatal(err)
	}
	src.c.Activity.Record(console.Capture{ID: "20261120-0001", At: time.Now().UTC(), Agent: "claude-code", Tool: "vault__search", Outcome: console.OutcomeOK})
	src.c.Activity.Flush()
	box, err := src.c.Secrets.Seal(context.Background(), "a console secret")
	if err != nil {
		t.Fatal(err)
	}
	callLookup(t, tok.MCPURL, tok.Token, true)
	return src, tok, box
}

func TestAFullBackupMovesEverythingToAFreshInstall(t *testing.T) {
	src, tok, box := seeded(t)
	first := src.view().Connectors[0].ID
	if _, err := src.c.State.PutSlot(console.KeySlot{Name: "from-a-setup", Profiles: map[string]console.Profile{first: {Mode: console.ProfileCustom, Overrides: map[string]console.Access{"lookup": console.AccessAsk}}}}); err != nil {
		t.Fatal(err)
	}
	want := src.view()
	if !slices.ContainsFunc(want.Connectors, func(c console.ConnectorSummary) bool {
		return c.NewKeysOff && len(c.Profiles) == 1 && c.Profiles[0].Overrides["search"] == console.AccessOff
	}) || !slices.ContainsFunc(want.Connectors, func(c console.ConnectorSummary) bool { return len(c.Slots) == 1 }) {
		t.Fatalf("the seeded install has no key profile or permission set: %+v", want.Connectors)
	}

	if status, raw := src.b.send("POST", "/api/backup/export", map[string]any{"mode": "full", "password": ownerPassword, "passphrase": passphrase}, nil); status != http.StatusBadRequest || !strings.Contains(string(raw), "confirm_required") {
		t.Fatalf("an export without the confirmation: %d %s", status, raw)
	}
	if status, _ := src.b.send("POST", "/api/backup/export", map[string]any{"mode": "full", "password": "not the password", "passphrase": passphrase, "confirm": true}, nil); status != http.StatusUnauthorized {
		t.Fatalf("an export with a wrong password: %d", status)
	}
	file := src.export()
	if bytes.Contains(file, []byte(apiKey)) || bytes.Contains(file, []byte("claude-code")) {
		t.Fatal("the backup file holds plaintext")
	}
	var acct struct {
		LastExport time.Time `json:"last_export"`
	}
	src.b.ok("GET", "/api/account", nil, &acct)
	if time.Since(acct.LastExport) > time.Minute {
		t.Fatalf("last export %v", acct.LastExport)
	}

	dst := openStack(t, t.TempDir(), storememory.New())
	for name, f := range map[string][]byte{"a wrong passphrase": nil, "a damaged file": append(bytes.Clone(file[:len(file)-1]), file[len(file)-1]^1), "a newer format": bytes.Replace(file, []byte(`"format":1`), []byte(`"format":9`), 1)} {
		pass := passphrase
		if f == nil {
			f, pass = file, "another passphrase entirely"
		}
		if status, raw := dst.upload("/api/backup/inspect", f, pass); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, raw)
		}
	}
	var sum console.BackupSummary
	status, raw := dst.upload("/api/backup/inspect", file, passphrase)
	if err := json.Unmarshal(raw, &sum); status != http.StatusOK || err != nil {
		t.Fatalf("inspect: %d %s", status, raw)
	}
	if sum.Kind != "full" || sum.Replaces || sum.Counts["connectors"] != 2 || sum.Counts["skills"] != 1 || sum.Counts["keys"] != 1 || sum.Counts["oauth_clients"] != 1 {
		t.Fatalf("summary %+v", sum)
	}

	moved := dst.restoreAndRestart(file)
	sameView(t, want, moved.view())
	if c, ok := moved.c.State.OAuthClient("client-1"); !ok || c.Name != "An app" {
		t.Fatalf("the OAuth client: %+v %v", c, ok)
	}
	if got, err := moved.c.Secrets.Open(context.Background(), box); err != nil || got != "a console secret" {
		t.Fatalf("a console secret: %q %v", got, err)
	}
	wantCaptures, _ := src.c.Activity.Captures(time.Time{})
	captures, err := moved.c.Activity.Captures(time.Time{})
	if err != nil || len(captures) != 2 || !slices.EqualFunc(captures, wantCaptures, func(a, b console.Capture) bool { return a.ID == b.ID }) {
		t.Fatalf("activity: %+v %v", captures, err)
	}
	callLookup(t, moved.base+"/mcp", tok.Token, false)

	t.Run("a signed-in import asks for the password again", func(t *testing.T) {
		if status, _ := moved.uploadAs("/api/backup/import", file, passphrase, "not the password"); status != http.StatusUnauthorized {
			t.Fatalf("an import with a wrong password: %d", status)
		}
	})
	t.Run("importing the same backup again changes nothing", func(t *testing.T) {
		again := moved.restoreAndRestart(file)
		sameView(t, want, again.view())
	})
}

func TestAFailedRestoreLeavesTheInstallAsItWas(t *testing.T) {
	src, _, box := seeded(t)
	file := src.export()
	want := src.view()
	stateBefore, _ := os.ReadFile(filepath.Join(src.dir, "state.json"))
	keysBefore := dirNames(t, filepath.Join(src.dir, "console-secrets"))
	m, files, err := backup.Open(bytes.NewReader(file), []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	reseal := func(name string, edit func([]byte) []byte) []byte {
		f := map[string][]byte{}
		for k, v := range files {
			f[k] = v
		}
		f[name] = edit(f[name])
		var out bytes.Buffer
		if err := backup.Seal(&out, []byte(passphrase), m, f); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	for name, broken := range map[string][]byte{
		"the engine refuses its rows": reseal("engine/snapshot.json", func(b []byte) []byte {
			var snap app.Snapshot
			if err := json.Unmarshal(b, &snap); err != nil {
				t.Fatal(err)
			}
			snap.Store.Tables["connectors"] = append(snap.Store.Tables["connectors"], json.RawMessage(`"not a row"`))
			out, _ := json.Marshal(snap)
			return out
		}),
		"the console keys do not open their state": reseal("console/keys.json", func([]byte) []byte { return []byte(`{"root":"AAAA","state":{}}`) }),
		"the console state has no owner":           reseal("console/state.json", func([]byte) []byte { return []byte(`{}`) }),
	} {
		t.Run(name, func(t *testing.T) {
			if status, raw := src.upload("/api/backup/import", broken, passphrase); status < 400 {
				t.Fatalf("a broken backup was restored: %d %s", status, raw)
			}
			select {
			case err := <-src.engine:
				t.Fatalf("the engine stopped: %v", err)
			default:
			}
			sameView(t, want, src.view())
			if after, _ := os.ReadFile(filepath.Join(src.dir, "state.json")); !bytes.Equal(after, stateBefore) {
				t.Fatal("the console state changed")
			}
			if got := dirNames(t, filepath.Join(src.dir, "console-secrets")); !slices.Equal(got, keysBefore) {
				t.Fatalf("the console key files changed: %v, had %v", got, keysBefore)
			}
			if got, err := src.c.Secrets.Open(context.Background(), box); err != nil || got != "a console secret" {
				t.Fatalf("a console secret: %q %v", got, err)
			}
		})
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
