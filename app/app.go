// Package app is the only composition root: it builds every port from the configuration and serves the surfaces.
package app

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/agents-control/plugkit/adapters"
	"github.com/agents-control/plugkit/discovery"

	"github.com/agents-control/connector-engine/artifact/digest"
	"github.com/agents-control/connector-engine/auth/local"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/credential"
	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/secrets/hostkey"
	"github.com/agents-control/connector-engine/server"
	"github.com/agents-control/connector-engine/service"
	"github.com/agents-control/connector-engine/service/worker"
	"github.com/agents-control/connector-engine/telemetry"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Options adjust a boot; the zero value reads the process environment and logs to the default logger.
type Options struct {
	Lookup func(string) string
	Clock  port.Clock
	Log    *slog.Logger
	Build  string
}

// App is a composed engine: the service and its adapters, ready to serve.
type App struct {
	Config   config.Config
	Adapters adapters.File
	Service  *service.Service
	Store    port.Store
	Index    port.ToolIndex
	Filter   port.ResultFilter
	Verifier port.PrincipalVerifier
	Clock    port.Clock

	log      *slog.Logger
	outbox   port.Outbox
	sink     port.EventSink
	sources  map[string]port.ListingSource
	worker   *worker.Worker
	metrics  *telemetry.Metrics
	issuer   *local.Issuer
	operator string
	env      *server.Env
	agent    http.Handler
	spec     adapters.Spec
	secrets  port.SecretStore
	build    string
	closers  []func()
}

// New builds the engine: it checks the configuration against the adapters file, opens the store, writes the bootstrap roles and wires the service.
func New(ctx context.Context, cfg config.Config, file adapters.File, o Options) (*App, error) {
	if err := adapters.Check(cfg.AdaptersSpec(), file).Err(); err != nil {
		return nil, err
	}
	if o.Lookup == nil {
		o.Lookup = func(string) string { return "" }
	}
	if o.Clock == nil {
		o.Clock = systemClock{}
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	a := &App{Config: cfg, Adapters: file, Clock: o.Clock, log: o.Log, spec: cfg.AdaptersSpec(), build: o.Build}
	if err := a.compose(ctx, builder{cfg: cfg, file: file, lookup: o.Lookup, clock: o.Clock, log: o.Log}); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) compose(ctx context.Context, b builder) error {
	store, outbox, closeStore, err := b.store(ctx)
	if err != nil {
		return err
	}
	a.Store, a.outbox = store, outbox
	a.closers = append(a.closers, closeStore)
	secrets, closeSecrets, err := b.secrets(ctx)
	if err != nil {
		return fmt.Errorf("secrets: %w", err)
	}
	a.secrets = secrets
	a.closers = append(a.closers, closeSecrets)
	runtime, err := b.runtime(store, secrets)
	if err != nil {
		return fmt.Errorf("runtime: %w", err)
	}
	if a.sources, err = b.sources(); err != nil {
		return err
	}
	if a.sink, err = b.sink(); err != nil {
		return err
	}
	clients, err := b.clients()
	if err != nil {
		return err
	}
	if a.Config.Auth.LocalIssuer.Enabled {
		if a.issuer, err = local.New(a.Config.Auth.LocalIssuer.Name, a.Clock.Now); err != nil {
			return fmt.Errorf("local issuer: %w", err)
		}
	}
	var localKeys http.Handler
	if a.issuer != nil {
		localKeys = a.issuer.Handler()
	}
	if a.Verifier, err = b.verifier(localKeys, a.Config.Auth.LocalIssuer.Name); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if ref := a.Config.Auth.Operator.TokenRef; ref != "" {
		if a.operator, err = resolveRef(b.lookup, ref); err != nil {
			return fmt.Errorf("auth.operator.tokenRef: %w", err)
		}
	}
	svcCfg := a.Config.Service()
	if a.Filter, err = b.filter(); err != nil {
		return err
	}
	a.Index = b.index()
	a.Service, err = service.New(service.Deps{
		Store: store, Clock: a.Clock, Secrets: secrets, Runtime: runtime, Sources: a.sources, Verifier: digest.Verifier{}, Filter: a.Filter,
		Authorizer: table.New(store, a.Clock, table.Settings{DestructiveMax: svcCfg.DestructiveMax, ForceAsk: svcCfg.ForceAsk}),
		Broker:     &credential.Broker{HTTP: b.guard(b.issuerHosts()).Client()}, Registrar: credential.NewRegistrar(clients...).WithStore(store, secrets),
	}, svcCfg)
	if err != nil {
		return err
	}
	a.metrics = telemetry.New(a.outbox, a.Clock.Now)
	a.worker = worker.New(a.Service, store, a.Clock, worker.Config{Interval: a.Config.Approval.SweepInterval, Grace: a.Config.Server.ShutdownGrace, Log: a.log, Failed: a.metrics.WorkerFailed})
	a.env = &server.Env{Svc: a.Service, Adapters: a.document}
	gcfg := gateway.DefaultConfig(a.Config.Audience())
	gcfg.ViewTokens = a.Config.Apps.AppOnlyFunctions == "view_token"
	gcfg.ResourceMetadataURL = a.Config.ResourceMetadataURL()
	gcfg.HoldWindow, gcfg.Logger, gcfg.Skills, gcfg.SkillPrompts = a.Config.Approval.HoldWindow, a.log, a.Config.Gateway.Skills != "off", a.Config.Gateway.Skills == "tools+prompts"
	gcfg.Instructions, gcfg.Content = a.Config.Gateway.RelayInstructions == "pinned", a.Config.Gateway.UpstreamContent == "granted"
	gcfg.ThresholdTokens, gcfg.ThresholdFunctions, gcfg.SearchResultMax = a.Config.Search.ThresholdTokens, a.Config.Search.ThresholdFunctions, a.Config.Search.ResultMax
	agent, err := gateway.New(a.metrics.Core(a.Service), a.Verifier, a.Index, gcfg)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	a.agent = agent.Handler()
	a.sink = a.metrics.Sink(listChanged{next: a.sink, notify: agent.ToolsChanged})
	return a.bootstrap(ctx)
}

func (a *App) bootstrap(ctx context.Context) error {
	for _, r := range a.Config.Bootstrap.Roles {
		row := port.RoleAssignment{Tenant: port.TenantID(r.Tenant), Issuer: r.Issuer, Person: port.PersonID(r.Person), Role: r.Role, Scope: r.Scope}
		scope := port.Scope{Tenant: row.Tenant, Installation: row.Tenant == port.Installation}
		if err := a.Store.Write(ctx, scope, func(tx port.Tx) error { return tx.PutRole(row) }); err != nil {
			return fmt.Errorf("bootstrap.roles: %w", err)
		}
	}
	return nil
}

func (a *App) document() discovery.Document {
	d := discovery.Document{Engine: config.Engine, Build: a.build, Mode: a.Adapters.Mode, Acknowledge: a.Adapters.Acknowledge}
	for name, p := range a.Adapters.Ports {
		for _, s := range p.Use {
			u, _ := adapters.ParseUse(s)
			row := discovery.EmbeddedRow(name, u.Name)
			row.Tier, row.Capabilities = u.Tier, a.spec.Ports[name].Capabilities[s]
			if hk, ok := a.secrets.(*hostkey.Store); ok && name == config.PortSecrets {
				row.Capabilities = append(slices.Clone(row.Capabilities), "secrets.root_key_"+string(hk.Form()))
			}
			if u.Tier == adapters.TierMemory {
				row.Name = adapters.TierMemory
			}
			d.Adapters = append(d.Adapters, row)
		}
	}
	return d
}

func (a *App) personAuthn() server.Authenticator {
	audience := a.Config.ManageAudience()
	return func(ctx context.Context, token string) (port.Principal, error) {
		return a.Verifier.Verify(ctx, token, audience)
	}
}

func (a *App) operatorAuthn() server.Authenticator {
	return func(_ context.Context, token string) (port.Principal, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.operator)) != 1 {
			return port.Principal{}, port.Errorf(port.CodeUnauthenticated, "not the operator token")
		}
		return port.Principal{Kind: port.PrincipalSystem, Token: token}, nil
	}
}

// Close releases what New opened, last first.
func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
	a.closers = nil
}
