package config

import (
	"maps"
	"slices"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
)

// Builtin declares an adapter of a port: its name (selected as builtin:<Name>, or the memory tier when Name is memory), its capabilities, the settings it accepts and the checks it adds to connector-engine.yaml.
type Builtin struct {
	Port         string
	Name         string
	Capabilities []string
	Settings     map[string]adapters.ConfigKind
	Check        func(c Config, acknowledged func(string) bool) adapters.Findings
}

// Selection is how adapters.yaml selects the adapter.
func (b Builtin) Selection() string {
	if b.Name == adapters.TierMemory {
		return adapters.TierMemory
	}
	return adapters.TierBuiltin + ":" + b.Name
}

// With returns c reading the adapters a build composes; it replaces those of an earlier call.
func (c Config) With(builtins ...Builtin) Config {
	c.builtins = slices.Clone(builtins)
	return c
}

func (c Config) has(port, name string) bool {
	return slices.ContainsFunc(c.builtins, func(b Builtin) bool { return b.Port == port && b.Name == name })
}

func (c Config) names(port string) []string {
	var out []string
	for _, b := range c.builtins {
		if b.Port == port && b.Name != adapters.TierMemory {
			out = append(out, b.Name)
		}
	}
	return out
}

func (c Config) declare(ports map[string]adapters.PortSpec) {
	for _, b := range c.builtins {
		p, ok := ports[b.Port]
		if !ok {
			continue
		}
		p.Capabilities, p.BuiltinConfig = maps.Clone(p.Capabilities), maps.Clone(p.BuiltinConfig)
		if p.Capabilities == nil {
			p.Capabilities = map[string][]string{}
		}
		if len(b.Capabilities) > 0 {
			p.Capabilities[b.Selection()] = b.Capabilities
		}
		if b.Name == adapters.TierMemory {
			p.Memory = true
		} else {
			p.Builtins = append(slices.Clone(p.Builtins), b.Name)
		}
		if len(b.Settings) > 0 {
			if p.BuiltinConfig == nil {
				p.BuiltinConfig = map[string]map[string]adapters.ConfigKind{}
			}
			p.BuiltinConfig[b.Selection()] = b.Settings
		}
		ports[b.Port] = p
	}
}

func (c Config) builtinFindings(acknowledged func(string) bool) adapters.Findings {
	var fs adapters.Findings
	for _, b := range c.builtins {
		if b.Check != nil {
			fs = append(fs, b.Check(c, acknowledged)...)
		}
	}
	return fs
}
