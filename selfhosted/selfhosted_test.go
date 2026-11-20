package selfhosted_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted"
)

const profile = "deploy/profiles/person/"

func TestThePersonProfileNeedsTheSelfHostedBuild(t *testing.T) {
	cfg, err := config.Load(profile + "connector-engine.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile + "adapters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.With(app.Core().Builtins()...).LoadAdapters(data, nil); err == nil || !strings.Contains(err.Error(), "hostkey") {
		t.Fatalf("the core alone accepts the person profile: %v", err)
	}
	r := app.Core()
	selfhosted.Register(r)
	f, err := cfg.With(r.Builtins()...).LoadAdapters(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string(f.Ports[config.PortSecrets].Use); !reflect.DeepEqual(got, []string{"builtin:hostkey"}) {
		t.Fatalf("secrets: %v", got)
	}
}

func TestHostkeyNeedsItsUnlockSocket(t *testing.T) {
	r := app.Core()
	selfhosted.Register(r)
	cfg := config.Default().With(r.Builtins()...)
	_, err := cfg.LoadAdapters([]byte("apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: production\nports:\n  secrets: {use: builtin:hostkey}\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "secrets.unlockSocket") {
		t.Fatal("hostkey without secrets.unlockSocket is accepted")
	}
}

func TestThePersonProfileReachesLocalServersAndKeepsItsRetention(t *testing.T) {
	cfg, err := config.Load(profile + "connector-engine.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Runtime.AllowPrivateHosts {
		t.Fatal("the person profile refuses the local servers its console imports")
	}
	week, quarter := 7*24*time.Hour, 90*24*time.Hour
	if want := (config.Retention{Every: time.Hour, DeliveredEvents: week, IdempotencyKeys: week, Audit: quarter, Calls: quarter}); cfg.Retention != want {
		t.Fatalf("person retention %+v, want %+v", cfg.Retention, want)
	}
}

func TestThePersonProfileRelaysServerGuidance(t *testing.T) {
	c, err := config.Load(profile + "connector-engine.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := [3]bool{c.Service().RelayInstructions, c.Service().UpstreamContent, c.Gateway.Skills == "tools+prompts"}; got != [3]bool{true, true, true} {
		t.Errorf("person: instructions, content and skill prompts %v", got)
	}
}

func TestTheHostkeyAdapterOpensThroughTheRegistryAndReportsHowItsRootKeyRests(t *testing.T) {
	dir, err := os.MkdirTemp("", "hk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg := config.Default()
	cfg.Mode = config.ModeDev
	cfg.Server.Listen, cfg.Server.GRPCListen, cfg.Server.OperatorListen = "127.0.0.1:0", "127.0.0.1:0", "127.0.0.1:0"
	cfg.Server.PublicURL = "http://engine.test"
	cfg.Auth.LocalIssuer.Enabled = true
	cfg.Auth.Operator.TokenRef = "env:CN_OPERATOR_TOKEN"
	cfg.Secrets.Dir, cfg.Secrets.UnlockSocket = filepath.Join(dir, "keys"), filepath.Join(dir, "unlock.sock")
	if _, err := secretslocal.Open(cfg.Secrets.Dir); err != nil {
		t.Fatal(err)
	}
	r := app.Core()
	selfhosted.Register(r)
	file, err := cfg.With(r.Builtins()...).LoadAdapters([]byte("apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: dev\nports:\n  store: {use: memory}\n  secrets: {use: builtin:hostkey}\n  auth: {use: builtin:local}\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	a, err := app.New(ctx, cfg, file, app.Options{Lookup: func(k string) string { return map[string]string{"CN_OPERATOR_TOKEN": "op"}[k] }, Adapters: r})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	l, err := a.Listen()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx, l) }()
	req, _ := http.NewRequest(http.MethodGet, "http://"+l.Operator.Addr().String()+"/v1/adapters", nil)
	req.Header.Set("Authorization", "Bearer op")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "secrets.root_key_plaintext") || !strings.Contains(string(body), `"hostkey"`) {
		t.Fatalf("/v1/adapters: %d %s", res.StatusCode, body)
	}
	stop()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the engine did not stop")
	}
}
