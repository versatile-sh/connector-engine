package config

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/agents-control/plugkit/adapters"
)

// Engine and ports of the adapters file; each port also reads CN_<PORT> from the environment.
const (
	Engine      = "connector-engine"
	EnvPrefix   = "CN"
	PortStore   = "store"
	PortSecrets = "secrets"
	PortAuth    = "auth"
	PortRuntime = "runtime"
	PortSource  = "source"
	PortSink    = "sink"
	PortFilter  = "filter"
	PortIndex   = "index"
)

func use(tier string, names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = tier + ":" + n
	}
	return out
}

func (c Config) runtimeDefault() []string { return use(adapters.TierBuiltin, c.Runtime.Adapters...) }

func (c Config) sourceDefault() []string {
	var out []string
	for name, on := range map[string]bool{"files": c.Sources.Files.Enabled, "official": c.Sources.Official.Enabled, "docker": c.Sources.Docker.Enabled,
		"openapi": c.Sources.OpenAPI.Enabled, "reflection": c.Sources.Reflection.Enabled} {
		if on {
			out = append(out, adapters.TierBuiltin+":"+name)
		}
	}
	slices.Sort(out)
	return out
}

func (c Config) secretsDefault() []string {
	if c.Secrets.Adapter == "memory" {
		return []string{adapters.TierMemory}
	}
	return use(adapters.TierBuiltin, "localfile")
}

func (c Config) filterDefault() []string {
	if c.ResultFilter.Adapter == "builtin" {
		return use(adapters.TierBuiltin, "builtin")
	}
	return nil
}

// AdaptersSpec declares the ports of the engine; the defaults of a port follow the keys of connector-engine.yaml that name its adapter.
// Plugin selections are refused: no port of this build speaks a plugin protocol yet.
func (c Config) AdaptersSpec() adapters.Spec {
	return adapters.Spec{Engine: Engine, EnvPrefix: EnvPrefix, Acknowledgements: Acknowledgements, Ports: map[string]adapters.PortSpec{
		PortStore: {Default: use(adapters.TierBuiltin, "postgres"), Builtins: []string{"postgres"}, Memory: true, Stateful: true,
			Capabilities:  map[string][]string{"memory": {"store.volatile"}, "builtin:postgres": {"store.transactional_outbox", "store.row_level_security"}},
			BuiltinConfig: map[string]map[string]adapters.ConfigKind{"builtin:postgres": {"databaseUrlRef": adapters.ConfigRef}}},
		PortSecrets: {Default: c.secretsDefault(), Builtins: []string{"localfile"}, Memory: true, Stateful: true, Sensitive: true,
			Capabilities: map[string][]string{"builtin:localfile": {"secrets.envelope", "secrets.crypto_shredding"}, "memory": {"secrets.volatile"}}},
		PortAuth:    {Default: use(adapters.TierBuiltin, "jwks"), Builtins: []string{"jwks", "local"}, Sensitive: true, Weak: []string{"builtin:local"}},
		PortRuntime: {Default: c.runtimeDefault(), Builtins: []string{"remote", "declarative", "grpc"}, Memory: true, Multi: true},
		PortSource:  {Default: c.sourceDefault(), Builtins: []string{"files", "official", "docker", "openapi", "reflection"}, Memory: true, Multi: true},
		PortSink: {Default: use(adapters.TierBuiltin, "log"), Builtins: []string{"log", "webhook"}, Memory: true,
			BuiltinConfig: map[string]map[string]adapters.ConfigKind{"builtin:webhook": {"url": adapters.ConfigString, "secretRef": adapters.ConfigRef, "timeout": adapters.ConfigString}}},
		PortFilter: {Default: c.filterDefault(), Builtins: []string{"builtin"}},
		PortIndex:  {Default: use(adapters.TierBuiltin, "lexical"), Builtins: []string{"lexical"}},
	}, Validate: c.validateAdapters}
}

func (c Config) validateAdapters(f adapters.File) adapters.Findings {
	fs := c.Check(f.Acknowledged)
	if f.Mode != c.Mode {
		fs = append(fs, adapters.Finding{Level: adapters.LevelError, Where: "mode", Message: fmt.Sprintf("the adapters file says %q and connector-engine.yaml %q", f.Mode, c.Mode)})
	}
	if u, ok := f.First(PortAuth); ok && u.Name == "local" && !c.Auth.LocalIssuer.Enabled {
		fs = append(fs, adapters.Finding{Level: adapters.LevelError, Where: "port auth", Message: "builtin:local needs auth.localIssuer.enabled in connector-engine.yaml"})
	}
	if c.Auth.LocalIssuer.Enabled {
		if u, _ := f.First(PortAuth); u.Name != "local" {
			fs = append(fs, adapters.Finding{Level: adapters.LevelError, Where: "port auth", Message: "auth.localIssuer.enabled needs the port auth to select builtin:local"})
		}
	}
	return fs
}

// LoadAdapters reads adapters.yaml (nil data is the env form and the defaults) and checks it with the rules of §14.
func (c Config) LoadAdapters(data []byte, environ []string) (adapters.File, error) {
	spec := c.AdaptersSpec()
	f, err := adapters.Load(spec, data, environ)
	if err != nil {
		return f, err
	}
	if f.Mode == "" || (data == nil && !slices.ContainsFunc(environ, func(kv string) bool { return strings.HasPrefix(kv, EnvPrefix+"_MODE=") })) {
		f.Mode = c.Mode
	}
	return f, adapters.Check(spec, f).Err()
}

// ReadAdapters loads the file at path, or the defaults when path is empty.
func (c Config) ReadAdapters(path string, environ []string) (adapters.File, error) {
	var data []byte
	if path != "" {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return adapters.File{}, fmt.Errorf("adapters.yaml: %w", err)
		}
	}
	return c.LoadAdapters(data, environ)
}

// AdaptersMain runs "adapters check" or "adapters plan" with the same rules boot applies.
func (c Config) AdaptersMain(args, environ []string, stdout, stderr io.Writer) int {
	return adapters.Main(c.AdaptersSpec(), args, environ, stdout, stderr)
}
