package config_test

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/plugkit/adapters"
	"gopkg.in/yaml.v3"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/service"
)

var update = flag.Bool("update", false, "rewrite connector-engine.yaml from the defaults")

var keysOfSection14 = strings.Fields(`mode auth.issuers auth.audience auth.localIssuer.enabled gateway.protocolVersions gateway.relayInstructions
gateway.scanArgumentsForToken apps.appOnlyFunctions auth.agentTokenMaxLifetime auth.senderConstraint auth.manageAudience auth.trustedProxy.listen
auth.trustedProxy.cidrs manage.mcp.operations catalog.tenantDefault catalog.offerDefault catalog.defaultCeiling catalog.privateListings
gateway.listTtl gateway.toolNameMax gateway.separator gateway.denyAs permission.defaultMode permission.levelWrite.write permission.levelWrite.destructive
permission.destructiveMax permission.sessionGrantMax permission.unclassified permission.install.levelReviewed permission.install.levelOther
permission.alwaysMaxDuration permission.external.endpoint approval.holdWindow approval.timeout approval.pollAfter approval.resultRetention
approval.denialBrake.consecutive approval.denialBrake.perSession approval.maxPendingPerSession approval.executeWithin approval.sweepInterval
approval.executeGrace approval.proofOfReading approval.inChat.classes approval.noteMaxChars approval.changedWarningDays pinning.relistMin
pinning.relistMax pinning.hashIcons pinning.autoAcceptReviewed pinning.relistBeforeCall exposure.onTrifecta search.thresholdTokens
search.thresholdFunctions search.resultMax projection.default projection.maxPaths resultFilter.adapter overlay.suggester cli.defaultForCodingAgents
grpc.unaryOnly sources.official.enabled sources.official.url sources.docker.enabled sources.syncInterval listing.requireDigest listing.requireSignature
credentials.refreshMargin credentials.registrationOrder credentials.fallback secrets.adapter secrets.dataKeyCache runtime.adapters runtime.callTimeout
runtime.container.cpu runtime.container.memory runtime.container.idleStop runtime.container.network audit.arguments audit.hashChain outbox.signing`)

func TestDefaultsRoundTripAndTheCheckedInFileIsTheDefaults(t *testing.T) {
	data, err := config.Default().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile("connector-engine.yaml", data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("connector-engine.yaml")
	if err != nil || string(got) != string(data) {
		t.Fatalf("connector-engine.yaml is stale or missing: go test ./config -run Defaults -update (%v)", err)
	}
	back, err := config.Parse(data)
	if err != nil || !reflect.DeepEqual(back, config.Default()) {
		t.Fatalf("the defaults do not survive a round trip: %v", err)
	}
}

func TestEveryKeyOfSection14IsInTheFile(t *testing.T) {
	data, _ := config.Default().Marshal()
	var tree map[string]any
	if err := yaml.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	for _, key := range keysOfSection14 {
		var at any = tree
		for _, part := range strings.Split(key, ".") {
			m, ok := at.(map[string]any)
			if at = m[part]; !ok {
				t.Errorf("%s is not in connector-engine.yaml", key)
				break
			}
		}
	}
}

func TestTheServiceDefaultsAreTheConfigDefaults(t *testing.T) {
	if got, want := config.Default().Service(), service.DefaultConfig(); !reflect.DeepEqual(got, want) {
		t.Fatalf("service config from the file defaults\n%+v\nservice.DefaultConfig\n%+v", got, want)
	}
}

func TestParseKeepsDefaultsAndRefusesUnknownKeys(t *testing.T) {
	c, err := config.Parse([]byte("mode: dev\napproval:\n  timeout: 90s\n"))
	if err != nil || c.Approval.Timeout != 90*time.Second || c.Approval.PollAfter != 15*time.Second || c.Mode != "dev" {
		t.Fatalf("%+v %v", c.Approval, err)
	}
	if _, err := config.Parse([]byte("aproval: {}\n")); err == nil {
		t.Fatal("a misspelt key was accepted")
	}
}

func none(string) bool { return false }

func has(fs adapters.Findings, where, level string) bool {
	for _, f := range fs {
		if f.Where == where && f.Level == level {
			return true
		}
	}
	return false
}

func TestProductionRefusesWhatSection14Lists(t *testing.T) {
	base := config.Default()
	base.Server.PublicURL = "https://engine.test"
	base.Auth.Issuers = []config.Issuer{{Name: "idp", URL: "https://idp.test/jwks"}}
	if err := base.Check(none).Err(); err != nil {
		t.Fatalf("a sound production configuration is refused: %v", err)
	}
	for name, c := range map[string]struct {
		edit  func(*config.Config)
		where string
		ack   string
	}{
		"empty issuer list": {func(c *config.Config) { c.Auth.Issuers = nil }, "auth.issuers", ""},
		"local issuer":      {func(c *config.Config) { c.Auth.Issuers, c.Auth.LocalIssuer.Enabled = nil, true }, "auth.localIssuer.enabled", config.AckLocalIssuer},
		"private egress":    {func(c *config.Config) { c.Runtime.AllowPrivateHosts = true }, "runtime.allowPrivateHosts", config.AckPrivateEgress},
		"long agent tokens": {func(c *config.Config) { c.Auth.AgentTokenMaxLifetime = 48 * time.Hour }, "auth.agentTokenMaxLifetime", config.AckLongLivedTokens},
		"trusted proxy":     {func(c *config.Config) { c.Auth.TrustedProxy.Listen = ":8081" }, "auth.trustedProxy.listen", config.AckTrustedProxy},
		"manage mutation":   {func(c *config.Config) { c.Manage.MCP.Operations = []string{"read_only", "putRole"} }, "manage.mcp.operations", config.AckManageMutations},
		"no public address": {func(c *config.Config) { c.Server.PublicURL = "" }, "auth.audience", ""},
	} {
		cfg := base
		c.edit(&cfg)
		if !has(cfg.Check(none), c.where, adapters.LevelError) {
			t.Errorf("%s: production accepted it", name)
		}
		if c.ack != "" && cfg.Check(func(a string) bool { return a == c.ack }).Err() != nil && name != "empty issuer list" {
			t.Errorf("%s: the acknowledgement %s does not lift it", name, c.ack)
		}
		cfg.Mode = config.ModeDev
		if cfg.Check(none).Err() != nil {
			t.Errorf("%s: dev refused it", name)
		}
	}
}

func TestAdaptersFileFollowsTheDefaultsAndTheAcknowledgements(t *testing.T) {
	c := config.Default()
	c.Mode = config.ModeDev
	c.Auth.Issuers = []config.Issuer{{Name: "idp", URL: "https://idp.test/jwks"}}
	f, err := c.LoadAdapters(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Ports[config.PortStore].Use; !reflect.DeepEqual([]string(got), []string{"builtin:postgres"}) {
		t.Fatalf("store %v", got)
	}
	if got := f.Ports[config.PortRuntime].Use; !reflect.DeepEqual([]string(got), []string{"builtin:remote", "builtin:declarative"}) {
		t.Fatalf("runtime follows runtime.adapters: %v", got)
	}
	local := []byte("apiVersion: acplug/v1\nkind: Adapters\nengine: connector-engine\nmode: dev\nports:\n  store: {use: memory}\n  secrets: {use: memory}\n  auth: {use: builtin:local}\n")
	c.Auth.LocalIssuer.Enabled = true
	c.Auth.Issuers = nil
	if _, err := c.LoadAdapters(local, nil); err != nil {
		t.Fatalf("dev local issuer: %v", err)
	}
	c.Mode = config.ModeProduction
	prod := strings.Replace(string(local), "mode: dev", "mode: production\nallowMemory: true", 1)
	c.Server.PublicURL = "https://engine.test"
	if _, err := c.LoadAdapters([]byte(prod), nil); err == nil || !strings.Contains(err.Error(), config.AckLocalIssuer) {
		t.Fatalf("production accepted the local issuer unacknowledged: %v", err)
	}
	ack := strings.Replace(prod, "ports:", "acknowledge: ["+config.AckLocalIssuer+"]\nports:", 1)
	if _, err := c.LoadAdapters([]byte(ack), nil); err != nil {
		t.Fatalf("acknowledged local issuer: %v", err)
	}
	c.Auth.LocalIssuer.Enabled = false
	if _, err := c.LoadAdapters([]byte(ack), nil); err == nil {
		t.Fatal("builtin:local was selected without auth.localIssuer.enabled")
	}
}

func TestKeysThisBuildDoesNotActOnAcceptOnlyTheirDefault(t *testing.T) {
	for where, edit := range map[string]func(*config.Config){
		"gateway.relayInstructions":     func(c *config.Config) { c.Gateway.RelayInstructions = "pinned" },
		"gateway.denyAs":                func(c *config.Config) { c.Gateway.DenyAs = "error" },
		"audit.arguments":               func(c *config.Config) { c.Audit.Arguments = "redacted" },
		"audit.hashChain":               func(c *config.Config) { c.Audit.HashChain = true },
		"pinning.hashIcons":             func(c *config.Config) { c.Pinning.HashIcons = true },
		"approval.inChat.classes":       func(c *config.Config) { c.Approval.InChat.Classes = []string{"read"} },
		"listing.requireSignature":      func(c *config.Config) { c.Listing.RequireSignature = true },
		"secrets.dataKeyCache":          func(c *config.Config) { c.Secrets.DataKeyCache = time.Minute },
		"credentials.registrationOrder": func(c *config.Config) { c.Credentials.RegistrationOrder = []string{"dcr"} },
		"runtime.container":             func(c *config.Config) { c.Runtime.Container.CPU = 2 },
		"grpc.unaryOnly":                func(c *config.Config) { c.GRPC.UnaryOnly = false },
	} {
		cfg := config.Default()
		cfg.Mode = config.ModeDev
		edit(&cfg)
		if !has(cfg.Check(none), where, adapters.LevelError) {
			t.Errorf("%s: a value this build ignores is accepted", where)
		}
	}
}
