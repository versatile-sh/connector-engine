package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/app"
	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/store/storetest"
)

var pg *storetest.Postgres

func TestMain(m *testing.M) {
	var err error
	if pg, err = storetest.StartPostgres(); err != nil {
		fmt.Fprintln(os.Stderr, "postgres tests skipped:", err)
	}
	code := m.Run()
	if pg != nil {
		pg.Stop()
	}
	os.Exit(code)
}

const localAdapters = `apiVersion: acplug/v1
kind: Adapters
engine: connector-engine
mode: dev
%s
ports:
  store: {use: %s%s}
  secrets: {use: memory}
  auth: {use: builtin:local}
`

func devConfig(t *testing.T) config.Config {
	t.Helper()
	c := config.Default()
	c.Mode = config.ModeDev
	c.Server.Listen, c.Server.GRPCListen, c.Server.OperatorListen = "127.0.0.1:0", "127.0.0.1:0", "127.0.0.1:0"
	c.Server.PublicURL = "http://engine.test"
	c.Auth.LocalIssuer.Enabled = true
	c.Auth.Operator.TokenRef = "env:CN_OPERATOR_TOKEN"
	c.Bootstrap.Roles = []config.RoleRow{{Tenant: "t1", Person: "ana", Role: "tenant_admin", Scope: "t/t1"}}
	return c
}

type engine struct {
	t        *testing.T
	app      *app.App
	http     string
	operator string
	stop     context.CancelFunc
	done     chan error
}

func boot(t *testing.T, cfg config.Config, adaptersYAML string, env map[string]string) *engine {
	t.Helper()
	file, err := cfg.LoadAdapters([]byte(adaptersYAML), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	a, err := app.New(ctx, cfg, file, app.Options{Lookup: func(k string) string { return env[k] }})
	if err != nil {
		stop()
		t.Fatal(err)
	}
	l, err := a.Listen()
	if err != nil {
		stop()
		t.Fatal(err)
	}
	e := &engine{t: t, app: a, http: "http://" + l.HTTP.Addr().String(), operator: "http://" + l.Operator.Addr().String(), stop: stop, done: make(chan error, 1)}
	go func() { e.done <- a.Run(ctx, l) }()
	t.Cleanup(func() { e.shutdown(); a.Close() })
	return e
}

func (e *engine) shutdown() {
	e.stop()
	select {
	case err := <-e.done:
		if err != nil {
			e.t.Errorf("run: %v", err)
		}
		e.done <- nil
	case <-time.After(10 * time.Second):
		e.t.Error("the engine did not stop within the grace period")
	}
}

func (e *engine) call(method, url, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var in io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		in = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, url, in)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *engine) token(claims map[string]any) string {
	e.t.Helper()
	code, out := e.call(http.MethodPost, e.http+"/local/token", "", claims)
	if code != http.StatusOK {
		e.t.Fatalf("token: %d", code)
	}
	return out["token"].(string)
}

func exercise(t *testing.T, e *engine) {
	t.Helper()
	for _, probe := range []string{"/livez", "/readyz"} {
		if code, _ := e.call(http.MethodGet, e.http+probe, "", nil); code != http.StatusNoContent {
			t.Errorf("%s: %d", probe, code)
		}
	}
	if code, out := e.call(http.MethodPut, e.operator+"/v1/tenants/t1", "wrong", nil); code != http.StatusUnauthorized {
		t.Errorf("a wrong operator token: %d %v", code, out)
	}
	if code, out := e.call(http.MethodPut, e.operator+"/v1/tenants/t1", "op-secret", nil); code != http.StatusOK || out["id"] != "t1" {
		t.Fatalf("createTenant: %d %v", code, out)
	}
	if code, out := e.call(http.MethodGet, e.operator+"/v1/adapters", "op-secret", nil); code != http.StatusOK || len(out["adapters"].([]any)) < 5 {
		t.Errorf("adapters: %d %v", code, out)
	}
	ana := e.token(map[string]any{"sub": "ana", "tenant": "t1"})
	if code, out := e.call(http.MethodPut, e.http+"/v1/roles", ana, map[string]any{"tenant": "t1", "person": "bia", "role": "approver", "scope": "t/t1"}); code != http.StatusOK {
		t.Errorf("a bootstrapped tenant admin writes a role: %d %v", code, out)
	}
	if code, out := e.call(http.MethodGet, e.http+"/v1/roles?tenant=t1&person=bia", ana, nil); code != http.StatusOK || len(out["items"].([]any)) != 1 {
		t.Errorf("roles: %d %v", code, out)
	}
	run := map[string]any{"sub": "x", "tenant": "t1", "workspace": "w1", "agent": "x", "session": "s1"}
	perRun := e.token(run)
	run["aud"] = []string{"http://engine.test"}
	forged := e.token(run)
	for _, path := range []string{"/v1/roles", "/mcp/manage"} {
		if code, _ := e.call(http.MethodGet, e.http+path, perRun, nil); code != http.StatusUnauthorized {
			t.Errorf("a per-run token on %s: %d, want 401 (P2)", path, code)
		}
		if code, out := e.call(http.MethodGet, e.http+path, forged, nil); code != http.StatusForbidden || out["code"] != "agent_principal_refused" {
			t.Errorf("an agent claim on the manage audience at %s: %d %v, want 403 (P1)", path, code, out)
		}
	}
	if code, _ := e.call(http.MethodGet, e.http+"/mcp", ana, nil); code != http.StatusUnauthorized {
		t.Errorf("a person token on the agent endpoint /mcp: %d, want 401", code)
	}
}

func TestBootsWithMemoryAdaptersAndShutsDownGracefully(t *testing.T) {
	cfg := devConfig(t)
	e := boot(t, cfg, fmt.Sprintf(localAdapters, "", "memory", ""), map[string]string{"CN_OPERATOR_TOKEN": "op-secret"})
	exercise(t, e)
	started := time.Now()
	e.shutdown()
	if time.Since(started) > 5*time.Second {
		t.Error("shutdown was slow")
	}
}

func TestBootsOnPostgresAndKeepsRolesAcrossARestart(t *testing.T) {
	if pg == nil {
		t.Skip("postgres container unavailable")
	}
	dsn, err := pg.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CN_OPERATOR_TOKEN": "op-secret", "CN_DATABASE_URL": dsn}
	adaptersYAML := fmt.Sprintf(localAdapters, "", "builtin:postgres", ", config: {databaseUrlRef: env:CN_DATABASE_URL}")
	cfg := devConfig(t)
	e := boot(t, cfg, adaptersYAML, env)
	exercise(t, e)
	e.shutdown()
	cfg.Bootstrap.Roles = nil
	again := boot(t, cfg, adaptersYAML, env)
	ana := again.token(map[string]any{"sub": "ana", "tenant": "t1"})
	if code, out := again.call(http.MethodGet, again.http+"/v1/roles?tenant=t1&person=bia", ana, nil); code != http.StatusOK || len(out["items"].([]any)) != 1 {
		t.Fatalf("roles after a restart: %d %v", code, out)
	}
}

func TestProductionRefusesToBootWithoutIssuersAndWithTheLocalIssuerUnacknowledged(t *testing.T) {
	cfg := config.Default()
	cfg.Server.PublicURL = "https://engine.test"
	base := "apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: production\nallowMemory: true\n%sports:\n  store: {use: memory}\n  secrets: {use: memory}\n"
	if _, err := cfg.LoadAdapters([]byte(fmt.Sprintf(base, "")), nil); err == nil || !strings.Contains(err.Error(), "auth.issuers") {
		t.Fatalf("an empty issuer list booted: %v", err)
	}
	cfg.Auth.LocalIssuer.Enabled = true
	local := base + "  auth: {use: builtin:local}\n"
	if _, err := cfg.LoadAdapters([]byte(fmt.Sprintf(local, "")), nil); err == nil || !strings.Contains(err.Error(), config.AckLocalIssuer) {
		t.Fatalf("an unacknowledged local issuer booted: %v", err)
	}
	if _, err := cfg.LoadAdapters([]byte(fmt.Sprintf(local, "acknowledge: ["+config.AckLocalIssuer+"]\n")), nil); err != nil {
		t.Fatalf("an acknowledged local issuer was refused: %v", err)
	}
}
