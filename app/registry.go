package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"github.com/versatile-sh/connector-engine/port"
)

// Port names a port of the engine and the Go type its adapters provide.
type Port[T any] struct{ name string }

// The ports a build registers adapters on; auth is the core's own.
var (
	Stores   = Port[port.Store]{config.PortStore}
	Secrets  = Port[port.SecretStore]{config.PortSecrets}
	Runtimes = Port[port.ConnectorRuntime]{config.PortRuntime}
	Sources  = Port[port.ListingSource]{config.PortSource}
	Sinks    = Port[port.EventSink]{config.PortSink}
	Filters  = Port[port.ResultFilter]{config.PortFilter}
	Indexes  = Port[port.ToolIndex]{config.PortIndex}
)

// Adapter declares an adapter of a port; Register sets its Port.
type Adapter = config.Builtin

// Factory opens an adapter once its port is selected; the closer may be nil.
type Factory[T any] func(ctx context.Context, env Env) (T, func(), error)

// Registry holds the adapters one binary composes, by port and name; Core returns the engine's own.
type Registry struct {
	decls []config.Builtin
	open  map[string]func(context.Context, Env) (any, func(), error)
}

// NewRegistry returns a registry with no adapter.
func NewRegistry() *Registry {
	return &Registry{open: map[string]func(context.Context, Env) (any, func(), error){}}
}

// Register adds the adapter a on the port p; a second adapter of the same name on a port panics, as a composition error.
func Register[T any](r *Registry, p Port[T], a Adapter, open Factory[T]) {
	a.Port = p.name
	key := p.name + "/" + a.Selection()
	if _, dup := r.open[key]; dup {
		panic(fmt.Sprintf("app: adapter %s registered twice on port %s", a.Selection(), p.name))
	}
	r.decls = append(r.decls, a)
	r.open[key] = func(ctx context.Context, env Env) (any, func(), error) { return open(ctx, env) }
}

// Builtins returns the declarations config needs, in registration order.
func (r *Registry) Builtins() []config.Builtin { return append([]config.Builtin(nil), r.decls...) }

func openAdapter[T any](ctx context.Context, r *Registry, p Port[T], u adapters.Use, env Env) (T, func(), error) {
	var zero T
	selection := u.Tier
	if u.Tier != adapters.TierMemory {
		selection = u.Tier + ":" + u.Name
	}
	open, ok := r.open[p.name+"/"+selection]
	if !ok {
		return zero, nil, fmt.Errorf("port %s: %s is not in this build", p.name, selection)
	}
	v, closer, err := open(ctx, env)
	if err != nil || v == nil {
		return zero, closer, err
	}
	return v.(T), closer, nil
}

// Env is what a factory may read: the configuration, the settings of its port in adapters.yaml, references, the clock, the log, and the ports opened before it.
type Env struct {
	Config   config.Config
	Settings map[string]any
	Lookup   func(string) string
	Clock    port.Clock
	Log      *slog.Logger
	Store    port.Store
	Secrets  port.SecretStore
	Applier  credential.Applier
}

// Setting returns the string setting key of the port, or "".
func (e Env) Setting(key string) string { return configString(e.Settings, key) }

// Ref resolves an env:NAME or file:PATH reference; an empty reference is "", where names the key in errors.
func (e Env) Ref(where, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	v, err := resolveRef(e.Lookup, ref)
	if err != nil {
		return "", fmt.Errorf("%s: %w", where, err)
	}
	return v, nil
}

// Guard is the egress guard of the engine, allowing the operator-configured hosts.
func (e Env) Guard(hosts ...string) egress.Guard {
	return egress.Guard{AllowPrivate: e.Config.Runtime.AllowPrivateHosts, Hosts: hosts, LoopbackHost: e.Config.Runtime.LoopbackHost}
}

// reporter is a port adapter that reports capabilities it knows only once it is open.
type reporter interface{ Capabilities() []string }
