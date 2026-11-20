package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"time"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"

	"github.com/versatile-sh/connector-engine/auth/jwks"
	"github.com/versatile-sh/connector-engine/auth/local"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/mux"
)

type builder struct {
	cfg    config.Config
	file   adapters.File
	lookup func(string) string
	clock  port.Clock
	log    *slog.Logger
	reg    *Registry
}

func (b builder) uses(name string) []adapters.Use {
	var out []adapters.Use
	for _, s := range b.file.Ports[name].Use {
		u, _ := adapters.ParseUse(s)
		out = append(out, u)
	}
	return out
}

func (b builder) one(name string) (adapters.Use, bool) { return b.file.First(name) }

func (b builder) env(name string) Env {
	return Env{Config: b.cfg, Settings: b.file.Ports[name].Config, Lookup: b.lookup, Clock: b.clock, Log: b.log}
}

func (b builder) store(ctx context.Context) (port.Store, port.Outbox, func(), error) {
	u, _ := b.one(config.PortStore)
	s, closer, err := openAdapter(ctx, b.reg, Stores, u, b.env(config.PortStore))
	if err != nil {
		return nil, nil, nil, err
	}
	outbox, ok := s.(port.Outbox)
	if !ok {
		closeAll(closer)()
		return nil, nil, nil, fmt.Errorf("store: %s keeps no outbox", u.Name)
	}
	return s, outbox, closeAll(closer), nil
}

func (b builder) localIssuer(ctx context.Context, secrets port.SecretStore) (*local.Issuer, error) {
	c := b.cfg.Auth.LocalIssuer
	if u, _ := b.one(config.PortSecrets); u.Tier == adapters.TierMemory {
		return local.New(c.Name, b.clock.Now)
	}
	return local.Open(ctx, c.Name, b.clock.Now, local.Keyring{Path: filepath.Join(b.cfg.Secrets.Dir, "local-issuer.json"), Secrets: secrets,
		RotateEvery: c.RotateEvery, Keep: max(time.Hour, b.cfg.Auth.AgentTokenMaxLifetime)})
}

func (b builder) secrets(ctx context.Context) (port.SecretStore, func(), error) {
	u, _ := b.one(config.PortSecrets)
	s, closer, err := openAdapter(ctx, b.reg, Secrets, u, b.env(config.PortSecrets))
	return s, closeAll(closer), err
}

func (b builder) clients() ([]credential.Registered, error) {
	var out []credential.Registered
	for _, c := range b.cfg.Credentials.Clients {
		secret := ""
		if c.SecretRef != "" {
			var err error
			if secret, err = resolveRef(b.lookup, c.SecretRef); err != nil {
				return nil, fmt.Errorf("credentials.clients[%s]: %w", c.Issuer, err)
			}
		}
		out = append(out, credential.Registered{Tenant: port.TenantID(c.Tenant), Client: port.OAuthClient{Issuer: c.Issuer, ClientID: c.ClientID, ClientSecret: []byte(secret), RedirectURL: c.RedirectURL}})
	}
	return out, nil
}

func (b builder) issuerHosts() []string {
	var issuers []string
	for _, c := range b.cfg.Credentials.Clients {
		issuers = append(issuers, c.Issuer)
	}
	return hostsOf(issuers...)
}

func (b builder) runtime(ctx context.Context, store port.Store, secrets port.SecretStore) (port.ConnectorRuntime, func(), error) {
	env := b.env(config.PortRuntime)
	env.Store, env.Secrets, env.Applier = store, secrets, credential.Applier{Resolve: credential.NewResolver(store, secrets), Injector: credential.Injector{}}
	var runtimes []port.ConnectorRuntime
	var closers []func()
	for _, u := range b.uses(config.PortRuntime) {
		rt, closer, err := openAdapter(ctx, b.reg, Runtimes, u, env)
		closers = append(closers, closer)
		if err != nil {
			closeAll(closers...)()
			return nil, nil, err
		}
		runtimes = append(runtimes, rt)
	}
	m, err := mux.New(runtimes...)
	if err != nil {
		closeAll(closers...)()
		return nil, nil, err
	}
	return m, closeAll(closers...), nil
}

func (b builder) guard(operatorHosts []string) egress.Guard {
	return egress.Guard{AllowPrivate: b.cfg.Runtime.AllowPrivateHosts, Hosts: operatorHosts}
}

func hostsOf(addresses ...string) []string {
	var out []string
	for _, a := range addresses {
		if u, err := url.Parse(a); err == nil && u.Host != "" {
			out = append(out, u.Host)
		} else {
			out = append(out, a)
		}
	}
	return out
}

func (b builder) sources(ctx context.Context) (map[string]port.ListingSource, func(), error) {
	out := map[string]port.ListingSource{}
	var closers []func()
	for _, u := range b.uses(config.PortSource) {
		s, closer, err := openAdapter(ctx, b.reg, Sources, u, b.env(config.PortSource))
		closers = append(closers, closer)
		if err != nil {
			closeAll(closers...)()
			return nil, nil, err
		}
		if s != nil {
			out[s.Name()] = s
		}
	}
	return out, closeAll(closers...), nil
}

func (b builder) sink(ctx context.Context) (port.EventSink, func(), error) {
	u, ok := b.one(config.PortSink)
	if !ok {
		u = adapters.Use{Tier: adapters.TierBuiltin, Name: "log"}
	}
	s, closer, err := openAdapter(ctx, b.reg, Sinks, u, b.env(config.PortSink))
	return s, closeAll(closer), err
}

func (b builder) filter(ctx context.Context) (port.ResultFilter, func(), error) {
	u, ok := b.one(config.PortFilter)
	if !ok {
		return nil, func() {}, nil
	}
	f, closer, err := openAdapter(ctx, b.reg, Filters, u, b.env(config.PortFilter))
	return f, closeAll(closer), err
}

func (b builder) index(ctx context.Context) (port.ToolIndex, func(), error) {
	u, ok := b.one(config.PortIndex)
	if !ok {
		u = adapters.Use{Tier: adapters.TierBuiltin, Name: "lexical"}
	}
	i, closer, err := openAdapter(ctx, b.reg, Indexes, u, b.env(config.PortIndex))
	return i, closeAll(closer), err
}

func closeAll(closers ...func()) func() {
	return func() {
		for i := len(closers) - 1; i >= 0; i-- {
			if closers[i] != nil {
				closers[i]()
			}
		}
	}
}

type localTransport struct{ issuer http.Handler }

func (t localTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != localIssuerHost {
		return http.DefaultTransport.RoundTrip(r)
	}
	rec := httptest.NewRecorder()
	t.issuer.ServeHTTP(rec, r)
	return rec.Result(), nil
}

const localIssuerHost = "local-issuer.invalid"

func (b builder) verifier(local http.Handler, localName string) (port.PrincipalVerifier, error) {
	c := jwks.Config{MaxLifetime: b.cfg.Auth.AgentTokenMaxLifetime, Now: b.clock.Now}
	for _, i := range b.cfg.Auth.Issuers {
		c.Issuers = append(c.Issuers, jwks.Issuer{Name: i.Name, URL: i.URL})
	}
	if local != nil {
		c.Issuers = append(c.Issuers, jwks.Issuer{Name: localName, URL: "http://" + localIssuerHost + "/jwks"})
		c.Client = &http.Client{Transport: localTransport{local}, Timeout: 10 * time.Second}
	}
	return jwks.New(c)
}
