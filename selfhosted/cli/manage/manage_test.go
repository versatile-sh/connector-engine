package manage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/internal/testmcp/notes"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/manage"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
)

const password = "a long enough password"

type world struct {
	t       *testing.T
	url     string
	environ []string
	config  string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir, err := os.MkdirTemp("", "vsh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	in, err := standalone.Start(context.Background(), standalone.Options{Data: filepath.Join(dir, "data"), Version: "test", Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Stop() })
	req, _ := http.NewRequest(http.MethodPost, in.URL+"/api/setup", strings.NewReader(`{"username":"me","password":"`+password+`"}`))
	req.Header.Set("Origin", in.URL)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %v %v", err, resp)
	}
	resp.Body.Close()
	config := filepath.Join(dir, "config")
	return &world{t: t, url: in.URL, config: config, environ: []string{"VSH_URL=" + in.URL, "VSH_NO_KEYCHAIN=1", "VSH_CONFIG_DIR=" + config, "CN_DATA_DIR=" + filepath.Join(dir, "none")}}
}

func (w *world) vsh(stdin string, args ...string) (int, string, string) {
	w.t.Helper()
	root := &command.Command{Name: "vsh", Subs: append(manage.Commands(), &command.Command{Name: "skills", Subs: []*command.Command{manage.SkillsList()}})}
	var out, errw bytes.Buffer
	code := command.Main(context.Background(), root, args, strings.NewReader(stdin), &out, &errw, w.environ)
	return code, out.String(), errw.String()
}

func (w *world) ok(stdin string, args ...string) string {
	w.t.Helper()
	code, out, errw := w.vsh(stdin, args...)
	if code != 0 {
		w.t.Fatalf("vsh %v: exit %d\n%s%s", args, code, out, errw)
	}
	return out
}

func (w *world) fails(code int, want string, args ...string) {
	w.t.Helper()
	got, out, errw := w.vsh("", args...)
	if got != code || !strings.Contains(out+errw, want) {
		w.t.Fatalf("vsh %v: exit %d, want %d with %q\n%s%s", args, got, code, want, out, errw)
	}
}

func notesServer(t *testing.T) string {
	srv := notes.New(notes.Options{})
	up := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(up.Close)
	return up.URL + "/mcp"
}

func TestManageAnInstallAsTheSignedInUser(t *testing.T) {
	w := newWorld(t)
	if code, out, _ := w.vsh("", "status"); code != 0 || !strings.Contains(out, "not signed in") {
		t.Fatalf("status before login: %d %s", code, out)
	}
	w.fails(command.ExitAuth, "next: run 'vsh login'", "keys")
	w.fails(command.ExitUsage, "--password-stdin", "login", "--user", "me")
	if code, _, errw := w.vsh("wrong password\n", "login", "--user", "me", "--password-stdin"); code != command.ExitAuth || !strings.Contains(errw, "wrong username or password") {
		t.Fatalf("wrong password: %d %s", code, errw)
	}
	out := w.ok(password+"\n", "login", "--user", "me", "--password-stdin")
	files, _ := filepath.Glob(filepath.Join(w.config, "sessions", "*.json"))
	if len(files) != 1 || !strings.Contains(out, files[0]) {
		t.Fatalf("login said %q, sessions %v", out, files)
	}
	st, _ := os.Stat(files[0])
	raw, _ := os.ReadFile(files[0])
	var kept manage.Session
	if err := json.Unmarshal(raw, &kept); err != nil || st.Mode().Perm() != 0o600 || strings.Contains(out, kept.Cookie) {
		t.Fatalf("session file mode %v, %v, printed %q", st.Mode().Perm(), err, out)
	}
	if out := w.ok("", "status"); !strings.Contains(out, "me") {
		t.Fatalf("status: %s", out)
	}

	w.ok("", "connectors", "import", "--mcp", notesServer(t), "--local", "--yes")
	var list []struct {
		Alias     string `json:"alias"`
		Functions []struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Access string `json:"access"`
		} `json:"functions"`
	}
	if err := json.Unmarshal([]byte(w.ok("", "connectors", "--json")), &list); err != nil || len(list) != 1 {
		t.Fatalf("connectors --json: %v %v", err, list)
	}
	alias := list[0].Alias
	if out := w.ok("", "connectors", "show", alias); !strings.Contains(out, "READ") || !strings.Contains(out, "search") {
		t.Fatalf("show: %s", out)
	}

	access := func(key, name string) string {
		t.Helper()
		var k struct {
			Functions []struct{ Name, Access string } `json:"functions"`
			Profiles  []struct {
				Agent     string            `json:"agent"`
				Mode      string            `json:"mode"`
				Overrides map[string]string `json:"overrides"`
			} `json:"profiles"`
		}
		_ = json.Unmarshal([]byte(w.ok("", "connectors", "show", alias, "--json")), &k)
		if key != "" {
			for _, p := range k.Profiles {
				if p.Agent == key {
					if name == "" {
						return p.Mode
					}
					return p.Overrides[name]
				}
			}
			return ""
		}
		for _, f := range k.Functions {
			if f.Name == name {
				return f.Access
			}
		}
		return ""
	}
	w.ok("", "access", alias, "--kind", "read", "on")
	w.ok("", "access", alias, "search", "ask")
	if got := access("", "search"); got != "ask" {
		t.Fatalf("search is %q after access ask", got)
	}
	w.fails(command.ExitUsage, "--kind is read", "access", alias, "--kind", "nope", "on")
	w.fails(command.ExitFailure, "no connector named", "access", "nope", "on")

	token := strings.TrimSpace(w.ok("", "keys", "create", "agent-one"))
	if len(token) < 20 {
		t.Fatalf("keys create printed %q", token)
	}
	if out := w.ok("", "keys"); !strings.Contains(out, "agent-one") || strings.Contains(out, token) {
		t.Fatalf("keys list: %s", out)
	}
	w.ok("", "access", alias, "--key", "agent-one", "custom")
	w.ok("", "access", alias, "search", "off", "--key", "agent-one")
	if got := access("agent-one", "search"); got != "off" {
		t.Fatalf("agent-one's search is %q", got)
	}
	w.ok("", "access", alias, "--key", "agent-one", "follow")
	if got := access("agent-one", ""); got != "follow" {
		t.Fatalf("agent-one's mode is %q", got)
	}

	w.ok("", "approvals")
	w.fails(command.ExitFailure, "", "approvals", "approve", "nope")
	w.ok("", "activity", "--since", "1h")
	w.ok("", "skills", "list")

	w.fails(command.ExitUsage, "--yes", "keys", "revoke", "agent-one")
	w.ok("", "keys", "revoke", "agent-one", "--yes")
	w.ok("", "connectors", "remove", alias, "--yes")
	if out := w.ok("", "connectors"); !strings.Contains(out, "no connectors yet") {
		t.Fatalf("after remove: %s", out)
	}

	listing := filepath.Join(t.TempDir(), "crm.json")
	raw, err := os.ReadFile("../../../source/files/testdata/crm.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(listing, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if out := w.ok("", "connectors", "import", listing, "--yes"); !strings.Contains(out, "installed") {
		t.Fatalf("import from a file: %s", out)
	}
	w.fails(command.ExitUsage, "neither a URL nor a file", "connectors", "import", filepath.Join(t.TempDir(), "missing.yaml"), "--yes")

	w.ok("", "logout")
	w.fails(command.ExitAuth, "vsh login", "keys")
	if files, _ := filepath.Glob(filepath.Join(w.config, "sessions", "*.json")); len(files) != 0 {
		t.Fatalf("logout kept %v", files)
	}
}

func TestUnreachableConsole(t *testing.T) {
	dir := t.TempDir()
	root := &command.Command{Name: "vsh", Subs: manage.Commands()}
	var out, errw bytes.Buffer
	environ := []string{"VSH_URL=http://127.0.0.1:1", "VSH_NO_KEYCHAIN=1", "VSH_CONFIG_DIR=" + dir, "CN_DATA_DIR=" + dir}
	if code := command.Main(context.Background(), root, []string{"status"}, strings.NewReader(""), &out, &errw, environ); code != command.ExitUnreachable || !strings.Contains(errw.String(), "vsh up") {
		t.Fatalf("status: %d %s %s", code, out.String(), errw.String())
	}
}
