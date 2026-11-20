package config_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/agents-control/connector-engine/config"
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
	f, err := cfg.LoadAdapters(data, nil)
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
	if cfg.Secrets != (config.Secrets{Adapter: "localfile", DataKeyCache: cfg.Secrets.DataKeyCache, Dir: "/var/lib/connector-engine/secrets"}) || cfg.Secrets.DataKeyCache.Seconds() != 300 {
		t.Fatalf("headless secrets: %#v", cfg.Secrets)
	}
}

func TestAConfigThatDoesNotOptInDefaultsToTheLocalFileAndStillNeedsTheAcknowledgement(t *testing.T) {
	cfg := config.Default()
	if d := cfg.AdaptersSpec().Ports[config.PortSecrets].Default; !reflect.DeepEqual(d, []string{"builtin:localfile"}) {
		t.Fatalf("default secrets adapter: %v", d)
	}
	found := false
	for _, f := range cfg.Check(func(string) bool { return false }) {
		found = found || f.Where == "secrets.adapter"
	}
	if !found {
		t.Fatal("localfile without plaintext_root_key is refused in production")
	}
}
