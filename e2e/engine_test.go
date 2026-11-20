package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	operatorToken = "e2e-operator"
	sinkSecret    = "whsec_MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
	publicURL     = "http://engine.e2e"
	bootTimeout   = 30 * time.Second
)

var bootLine = regexp.MustCompile(`http=(\S+) grpc=(\S+) operator=(\S+)`)

type binaries struct{ engine, vsh string }

func build(t *testing.T) binaries {
	t.Helper()
	dir := t.TempDir()
	b := binaries{engine: filepath.Join(dir, "connector-engine"), vsh: filepath.Join(dir, "vsh")}
	for out, pkg := range map[string]string{b.engine: "github.com/versatile-sh/connector-engine/cmd/connector-engine", b.vsh: "github.com/versatile-sh/connector-engine/cmd/vsh"} {
		if o, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", pkg, err, o)
		}
	}
	return b
}

type settings struct {
	dir        string
	dsn        string
	sinkURL    string
	filesDir   string
	openAPIDir string
	grpcTarget string
	tenants    []string
}

func (s settings) config() string {
	var roles strings.Builder
	for _, tn := range s.tenants {
		fmt.Fprintf(&roles, "    - {tenant: %s, person: ana, role: tenant_admin, scope: t/%s}\n", tn, tn)
	}
	return fmt.Sprintf(`mode: dev
server:
  listen: 127.0.0.1:0
  grpcListen: ""
  operatorListen: 127.0.0.1:0
  publicURL: %s
auth:
  localIssuer: {enabled: true}
  operator: {tokenRef: "env:CN_OPERATOR_TOKEN"}
bootstrap:
  roles:
%s
approval:
  holdWindow: 400ms
  sweepInterval: 200ms
resultFilter: {adapter: builtin}
runtime:
  adapters: [remote, declarative, grpc]
  allowPrivateHosts: true
grpc: {insecure: true}
secrets: {adapter: localfile, dir: %s}
sources:
  files: {enabled: true, dir: %s}
  openapi: {enabled: true, dir: %s}
  reflection: {enabled: true, targets: ["%s"], insecure: true}
`, publicURL, roles.String(), filepath.Join(s.dir, "secrets"), s.filesDir, s.openAPIDir, s.grpcTarget)
}

func (s settings) adapters() string {
	store := "{use: memory}"
	if s.dsn != "" {
		store = "{use: builtin:postgres, config: {databaseUrlRef: \"env:CN_DATABASE_URL\"}}"
	}
	return fmt.Sprintf(`apiVersion: acplug/v1
kind: Adapters
engine: connector-engine
mode: dev
ports:
  store: %s
  secrets: {use: builtin:localfile}
  auth: {use: builtin:local}
  sink: {use: builtin:webhook, config: {url: "%s", secretRef: "env:CN_SINK_SECRET"}}
`, store, s.sinkURL)
}

type engine struct {
	bin      binaries
	cmd      *exec.Cmd
	done     chan error
	HTTP     string
	Operator string
	logPath  string
}

func startEngine(t *testing.T, bin binaries, s settings) *engine {
	t.Helper()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, ad := filepath.Join(s.dir, "connector-engine.yaml"), filepath.Join(s.dir, "adapters.yaml")
	for path, body := range map[string]string{cfg: s.config(), ad: s.adapters()} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := &engine{bin: bin, done: make(chan error, 1), logPath: filepath.Join(s.dir, fmt.Sprintf("engine-%d.log", time.Now().UnixNano()))}
	logFile, err := os.Create(e.logPath)
	if err != nil {
		t.Fatal(err)
	}
	e.cmd = exec.Command(bin.engine, "--config", cfg, "--adapters", ad)
	e.cmd.Env = append(os.Environ(), "CN_OPERATOR_TOKEN="+operatorToken, "CN_SINK_SECRET="+sinkSecret, "CN_DATABASE_URL="+s.dsn)
	e.cmd.Stderr = logFile
	out, err := e.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { e.done <- e.cmd.Wait(); logFile.Close() }()
	lines := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	select {
	case line, ok := <-lines:
		m := bootLine.FindStringSubmatch(line)
		if !ok || m == nil {
			t.Fatalf("no boot line (%q): %s", line, e.log())
		}
		e.HTTP, e.Operator = "http://"+m[1], "http://"+m[3]
	case <-time.After(bootTimeout):
		t.Fatalf("the engine did not boot: %s", e.log())
	}
	go func() { _, _ = io.Copy(io.Discard, out) }()
	return e
}

func (e *engine) log() string {
	b, _ := os.ReadFile(e.logPath)
	return string(b)
}

func (e *engine) stop(t *testing.T) {
	t.Helper()
	if e.cmd == nil {
		return
	}
	_ = e.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-e.done:
		if err != nil {
			t.Errorf("the engine did not stop cleanly: %v\n%s", err, e.log())
		}
	case <-time.After(20 * time.Second):
		_ = e.cmd.Process.Kill()
		<-e.done
		t.Errorf("the engine ignored SIGTERM")
	}
	e.cmd = nil
}

func (e *engine) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(claims)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.HTTP+"/local/token", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+operatorToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Token string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Token == "" {
		t.Fatalf("token for %v: %v", claims, err)
	}
	return out.Token
}

func (e *engine) person(t *testing.T, tenant, sub string) string {
	return e.token(t, map[string]any{"sub": sub, "tenant": tenant})
}

type agentID struct{ tenant, workspace, agent, session, onBehalfOf string }

func (e *engine) agent(t *testing.T, a agentID) string {
	return e.token(t, map[string]any{"sub": a.agent, "tenant": a.tenant, "workspace": a.workspace, "agent": a.agent, "session": a.session, "on_behalf_of": a.onBehalfOf})
}

func (e *engine) mcpURL() string { return e.HTTP + "/mcp" }
