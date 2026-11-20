package standalone

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/testmcp/notes"
	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
)

const password = "correct horse battery"

type fakeKeychain map[string]string

func (f fakeKeychain) Get(_ context.Context, a string) (string, error) {
	if v, ok := f[a]; ok {
		return v, nil
	}
	return "", keychain.ErrNotFound
}

func (f fakeKeychain) Set(_ context.Context, a, s string) error { f[a] = s; return nil }

func (f fakeKeychain) Delete(_ context.Context, a string) error { delete(f, a); return nil }

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func start(t *testing.T, data string, kc keychain.Keychain) *Install {
	t.Helper()
	in, err := Start(context.Background(), Options{Data: data, Keychain: kc, Version: "test", Log: quiet()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Stop() })
	return in
}

func notesServer(t *testing.T) string {
	t.Helper()
	srv := notes.New(notes.Options{})
	up := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(up.Close)
	return up.URL + "/mcp"
}

type browser struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
}

func newBrowser(t *testing.T, base string) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, base: base, c: &http.Client{Jar: jar, Timeout: time.Minute}}
}

func (b *browser) ok(method, path string, body, out any) {
	b.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, b.base+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", b.base)
	req.Header.Set("X-CSRF-Token", b.csrf)
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		b.t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, got)
	}
	if out != nil {
		if err := json.Unmarshal(got, out); err != nil {
			b.t.Fatalf("%s %s: %v %s", method, path, err, got)
		}
	}
}

func (b *browser) signIn(path string) {
	var s struct{ CSRF string }
	b.ok("POST", path, map[string]string{"username": "me", "password": password}, &s)
	b.csrf = s.CSRF
}

type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func callSearch(t *testing.T, endpoint, token string) error {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearer(token)}}, nil)
	if err != nil {
		return err
	}
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		return err
	}
	for _, tool := range tools.Tools {
		if strings.HasSuffix(tool.Name, "__search") {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{}})
			if err == nil && (res.IsError || len(res.Content) == 0 || res.Content[0].(*mcp.TextContent).Text != "ran search") {
				t.Fatalf("search answered %+v", res)
			}
			return err
		}
	}
	t.Fatalf("no search tool among %d tools", len(tools.Tools))
	return nil
}

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ce")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "data")
}

func holdSession(t *testing.T, endpoint, token string) func() {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "attached", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearer(token)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return func() { _ = cs.Close() }
}

func seed(t *testing.T, base string) string {
	t.Helper()
	b := newBrowser(t, base)
	b.signIn("/api/setup")
	b.ok("POST", "/api/import", map[string]any{"mcp": notesServer(t), "local": true}, nil)
	var tok struct{ Token string }
	b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)
	return tok.Token
}

func TestUpServesAnAgentAndKeepsItsDataAcrossARestart(t *testing.T) {
	data, kc := shortDir(t), fakeKeychain{}
	took := time.Now()
	in := start(t, data, kc)
	t.Logf("first start in %v", time.Since(took))
	if u, _ := url.Parse(in.URL); u.Hostname() != "127.0.0.1" {
		t.Fatalf("the console listens on %s", in.URL)
	}
	token := seed(t, in.URL)
	if err := callSearch(t, in.URL+"/mcp", token); err != nil {
		t.Fatal(err)
	}
	release := holdSession(t, in.URL+"/mcp", token)
	defer release()
	took = time.Now()
	if err := in.Stop(); err != nil {
		t.Fatal(err)
	}
	t.Logf("stop with an attached agent in %v", time.Since(took))
	if took := time.Since(took); took > drainGrace+5*time.Second {
		t.Fatalf("an attached agent held the stop for %v", took)
	}
	if info, err := os.Stat(data); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the data directory is %v, %v", info.Mode(), err)
	}
	for _, dir := range []string{filepath.Join(data, "engine", "secrets"), filepath.Join(data, "console", "secrets")} {
		if form := local.FormOf(dir); form != local.Wrapped {
			t.Fatalf("%s rests %s, want wrapped by the keychain", dir, form)
		}
	}
	if len(kc) != 1 {
		t.Fatalf("the keychain holds %v", kc)
	}
	took = time.Now()
	again := start(t, data, kc)
	t.Logf("restart in %v", time.Since(took))
	if err := callSearch(t, again.URL+"/mcp", token); err != nil {
		t.Fatalf("the key minted before the restart: %v", err)
	}
	newBrowser(t, again.URL).signIn("/api/login")
}

func run(t *testing.T, name string, stdin string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Commands("test")[name](config.Config{}, args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestCommandsDriveARunningInstall(t *testing.T) {
	data := shortDir(t)
	if code, out := run(t, "status", "", "--data", data); code != 3 || !strings.Contains(out, "no install runs") {
		t.Fatalf("status before up: %d %s", code, out)
	}
	in := start(t, data, nil)
	if _, err := Start(context.Background(), Options{Data: data, Log: quiet()}); err == nil {
		t.Fatal("a second install started on the same data directory")
	}
	if code, out := run(t, "status", "", "--data", data); code != 0 || !strings.Contains(out, in.URL) {
		t.Fatalf("status: %d %s", code, out)
	}
	seed(t, in.URL)
	if code, out := run(t, "reset-password", "a new long password\na new long password\n", "--data", data); code != 0 {
		t.Fatalf("reset-password: %d %s", code, out)
	}
	var s struct{ CSRF string }
	newBrowser(t, in.URL).ok("POST", "/api/login", map[string]string{"username": "me", "password": "a new long password"}, &s)
	setup := filepath.Join(t.TempDir(), "setup.yaml")
	if code, out := run(t, "export", "", "--data", data, "--setup", "--out", setup); code != 0 {
		t.Fatalf("export --setup: %d %s", code, out)
	}
	if raw, err := os.ReadFile(setup); err != nil || len(raw) == 0 {
		t.Fatalf("the setup file: %v %s", err, raw)
	}
	if code, out := run(t, "uninstall", "", "--data", data); code != 2 {
		t.Fatalf("uninstall without a choice: %d %s", code, out)
	}
	if code, out := run(t, "stop", "", "--data", data); code != 0 || !strings.Contains(out, "stopped") {
		t.Fatalf("stop: %d %s", code, out)
	}
	if err := in.Wait(); err != nil {
		t.Fatalf("the install stopped with %v", err)
	}
	if code, out := run(t, "uninstall", "", "--data", t.TempDir(), "--purge", "--yes", "--no-keychain"); code != 1 || !strings.Contains(out, "holds no install") {
		t.Fatalf("uninstall --purge of a directory without an install: %d %s", code, out)
	}
	if code, out := run(t, "uninstall", "", "--data", data+string(filepath.Separator), "--purge", "--yes", "--no-keychain"); code != 0 {
		t.Fatalf("uninstall --purge: %d %s", code, out)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("the data directory survived the purge: %v", err)
	}
}

func TestStartRefusesADataDirectoryOpenToOthers(t *testing.T) {
	data := shortDir(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(context.Background(), Options{Data: data, Log: quiet()}); runtime.GOOS != "windows" && (err == nil || !strings.Contains(err.Error(), "chmod 700")) {
		t.Fatalf("a 0755 data directory: %v", err)
	}
}

func TestDataDirFollowsTheOS(t *testing.T) {
	env := map[string]string{"XDG_DATA_HOME": "/xdg"}
	got, err := DataDir(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"linux": "/xdg/connector-engine"}[runtime.GOOS]; want != "" && got != want {
		t.Fatalf("data dir %s, want %s", got, want)
	}
	env["CN_DATA_DIR"] = "/explicit"
	if got, _ := DataDir(func(k string) string { return env[k] }); got != filepath.Clean("/explicit") {
		t.Fatalf("CN_DATA_DIR gave %s", got)
	}
}
