package config_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
)

func TestTheHeadlessProfileSelectsTheSameAdaptersAsBefore(t *testing.T) {
	cfg, err := config.Load("../deploy/profiles/headless/connector-engine.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../deploy/profiles/headless/adapters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := core(cfg).LoadAdapters(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for name, p := range f.Ports {
		got[name] = p.Use
	}
	want := map[string][]string{"auth": {"builtin:jwks"}, "index": {"builtin:lexical"}, "runtime": {"builtin:remote", "builtin:declarative"},
		"secrets": {"builtin:localfile"}, "sink": {"builtin:log"}, "store": {"builtin:postgres"}}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(f.Acknowledge, []string{"plaintext_root_key"}) {
		t.Fatalf("headless selects %v acknowledging %v", got, f.Acknowledge)
	}
	if cfg.Runtime.AllowPrivateHosts || cfg.Runtime.LoopbackHost != "" {
		t.Fatal("headless reaches private hosts or maps loopback")
	}
	if cfg.Server.Snapshot {
		t.Fatal("headless serves the snapshot routes")
	}
	if r := cfg.Retention; r.DeliveredEvents != 0 || r.IdempotencyKeys != 0 || r.Audit != 0 || r.Calls != 0 {
		t.Fatalf("headless prunes history: %+v", r)
	}
	if cfg.Secrets != (config.Secrets{DataKeyCache: cfg.Secrets.DataKeyCache, Dir: "/var/lib/connector-engine/secrets"}) || cfg.Secrets.DataKeyCache.Seconds() != 300 {
		t.Fatalf("headless secrets: %#v", cfg.Secrets)
	}
}

func TestAConfigThatDoesNotOptInDefaultsToTheLocalFileAndStillNeedsTheAcknowledgement(t *testing.T) {
	cfg := core(config.Default())
	if d := cfg.AdaptersSpec().Ports[config.PortSecrets].Default; !reflect.DeepEqual(d, []string{"builtin:localfile"}) {
		t.Fatalf("default secrets adapter: %v", d)
	}
	if !has(findings(t, cfg, "builtin:localfile"), "port secrets", adapters.LevelError) {
		t.Fatal("localfile without plaintext_root_key is refused in production")
	}
}

func TestTheHeadlessDefaultsStayByteIdentical(t *testing.T) {
	raw, err := json.Marshal(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{"config.Default()": raw}
	for _, name := range []string{"adapters.yaml", "connector-engine.yaml"} {
		if got[name], err = os.ReadFile("../deploy/profiles/headless/" + name); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		"config.Default()":      "299a64166eec422ed0bb163add22f00af9b9918f326abcd934498c88f7b018f5",
		"adapters.yaml":         "69b75fde582e2b044f6afa1bb6cb28f6154d3e64b97b65d3a8cf289229bee2db",
		"connector-engine.yaml": "b5932040f3f2190587b219b81f615d3127f13cb4de3f891093c2a9718701ca72",
	}
	for name, b := range got {
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != want[name] {
			t.Errorf("%s changed: sha256 %x", name, sum)
		}
	}
}
