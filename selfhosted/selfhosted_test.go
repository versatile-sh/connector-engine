package selfhosted_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/selfhosted"
)

const profile = "../deploy/profiles/person/"

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
	cfg.Secrets.Adapter = "hostkey"
	found := false
	for _, f := range cfg.Check(func(string) bool { return true }) {
		found = found || f.Where == "secrets.unlockSocket"
	}
	if !found {
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
