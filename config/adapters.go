package config

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
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

func (c Config) runtimeDefault() []string {
	out := use(adapters.TierBuiltin, c.Runtime.Adapters...)
	if c.Gateway.Skills != "off" && !slices.Contains(c.Runtime.Adapters, "skill") {
		out = append(out, adapters.TierBuiltin+":skill")
	}
	return out
}

func (c Config) sourceDefault() []string {
	var out []string
	for name, on := range map[string]bool{"files": c.Sources.Files.Enabled, "official": c.Sources.Official.Enabled, "docker": c.Sources.Docker.Enabled,
		"openapi": c.Sources.OpenAPI.Enabled, "reflection": c.Sources.Reflection.Enabled, "bsr": c.Sources.BSR.Enabled, "skillcatalog": c.Sources.SkillCatalog.Enabled} {
		if on {
			out = append(out, adapters.TierBuiltin+":"+name)
		}
	}
	slices.Sort(out)
	return out
}

func (c Config) secretsDefault() []string {
	switch {
	case c.Secrets.Adapter == "memory":
		return []string{adapters.TierMemory}
	case c.has(PortSecrets, c.Secrets.Adapter):
		return use(adapters.TierBuiltin, c.Secrets.Adapter)
	}
	return use(adapters.TierBuiltin, "localfile")
}

func (c Config) filterDefault() []string {
	if c.ResultFilter.Adapter == "builtin" {
		return use(adapters.TierBuiltin, "builtin")
	}
	return nil
}

// AdaptersSpec declares the ports of the engine: their defaults follow the keys of connector-engine.yaml that name an adapter, and their adapters are those of With.
// The auth port is the core's own. Plugin selections are refused: no port of this build speaks a plugin protocol yet.
func (c Config) AdaptersSpec() adapters.Spec {
	ports := map[string]adapters.PortSpec{
		PortStore:   {Default: use(adapters.TierBuiltin, "postgres"), Stateful: true},
		PortSecrets: {Default: c.secretsDefault(), Stateful: true, Sensitive: true},
		PortAuth:    {Default: use(adapters.TierBuiltin, "jwks"), Builtins: []string{"jwks", "local"}, Sensitive: true, Weak: []string{"builtin:local"}},
		PortRuntime: {Default: c.runtimeDefault(), Multi: true},
		PortSource:  {Default: c.sourceDefault(), Multi: true},
		PortSink:    {Default: use(adapters.TierBuiltin, "log")},
		PortFilter:  {Default: c.filterDefault()},
		PortIndex:   {Default: use(adapters.TierBuiltin, "lexical")},
	}
	c.declare(ports)
	return adapters.Spec{Engine: Engine, EnvPrefix: EnvPrefix, Acknowledgements: Acknowledgements, Ports: ports, Validate: c.validateAdapters}
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

// LoadAdapters reads adapters.yaml (nil data is the env form and the defaults) and checks it with the rules.
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
