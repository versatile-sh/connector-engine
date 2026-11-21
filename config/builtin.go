package config

import (
	"maps"
	"slices"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
)

// Finding is one result of a check of the configuration.
type Finding = adapters.Finding

// Findings are what a Builtin's Check returns.
type Findings = adapters.Findings

// SettingKind is the value type of a setting an adapter accepts in adapters.yaml.
type SettingKind = adapters.ConfigKind

// Finding levels and the kinds of a setting in adapters.yaml; SettingRef values are references such as env:NAME.
const (
	LevelError    = adapters.LevelError
	LevelWarning  = adapters.LevelWarning
	SettingRef    = adapters.ConfigRef
	SettingString = adapters.ConfigString
	SettingBool   = adapters.ConfigBool
	SettingNumber = adapters.ConfigNumber
)

// Builtin declares an adapter of a port: its name (selected as builtin:<Name>, or the memory tier when Name is memory), its capabilities, the settings it accepts and the checks it adds to connector-engine.yaml once selected.
type Builtin struct {
	Port         string
	Name         string
	Capabilities []string
	Settings     map[string]SettingKind
	Check        func(c Config, acknowledged func(string) bool) Findings
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

func (c Config) hasBuiltinBeyond(port, core string) bool {
	return slices.ContainsFunc(c.builtins, func(b Builtin) bool { return b.Port == port && b.Name != core })
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

func (c Config) builtinFindings(f adapters.File) Findings {
	var fs Findings
	for _, b := range c.builtins {
		if b.Check != nil && slices.ContainsFunc(f.Ports[b.Port].Use, func(s string) bool {
			u, err := adapters.ParseUse(s)
			return err == nil && selection(u) == b.Selection()
		}) {
			fs = append(fs, b.Check(c, f.Acknowledged)...)
		}
	}
	return fs
}

func selection(u adapters.Use) string {
	if u.Tier == adapters.TierMemory {
		return adapters.TierMemory
	}
	return u.Tier + ":" + u.Name
}
