package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/agents-control/plugkit/adapters"

	"github.com/agents-control/connector-engine/auth/jwks"
	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/credential"
	filterbuiltin "github.com/agents-control/connector-engine/filter/builtin"
	"github.com/agents-control/connector-engine/index/lexical"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/port"
	rtdeclarative "github.com/agents-control/connector-engine/runtime/declarative"
	rtgrpc "github.com/agents-control/connector-engine/runtime/grpc"
	rtmemory "github.com/agents-control/connector-engine/runtime/memory"
	"github.com/agents-control/connector-engine/runtime/mux"
	rtremote "github.com/agents-control/connector-engine/runtime/remote"
	rtskill "github.com/agents-control/connector-engine/runtime/skill"
	"github.com/agents-control/connector-engine/secrets/hostkey"
	secretslocal "github.com/agents-control/connector-engine/secrets/local"
	secretsmemory "github.com/agents-control/connector-engine/secrets/memory"
	sinklog "github.com/agents-control/connector-engine/sink/log"
	sinkmemory "github.com/agents-control/connector-engine/sink/memory"
	sinkwebhook "github.com/agents-control/connector-engine/sink/webhook"
	"github.com/agents-control/connector-engine/source/bsr"
	"github.com/agents-control/connector-engine/source/docker"
	"github.com/agents-control/connector-engine/source/files"
	"github.com/agents-control/connector-engine/source/openapi"
	"github.com/agents-control/connector-engine/source/reflection"
	"github.com/agents-control/connector-engine/source/registry"
	"github.com/agents-control/connector-engine/source/skillcatalog"
	"github.com/agents-control/connector-engine/store/memory"
	"github.com/agents-control/connector-engine/store/postgres"
)

type builder struct {
	cfg    config.Config
	file   adapters.File
	lookup func(string) string
	clock  port.Clock
	log    *slog.Logger
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

func (b builder) store(ctx context.Context) (port.Store, port.Outbox, func(), error) {
	u, _ := b.one(config.PortStore)
	if u.Tier == adapters.TierMemory {
		s := memory.New()
		return s, s, func() {}, nil
	}
	dsn, err := resolveRef(b.lookup, configString(b.file.Ports[config.PortStore].Config, "databaseUrlRef"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("store: %w", err)
	}
	s, err := postgres.New(ctx, dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("store: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, nil, nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, s, s.Close, nil
}

func (b builder) secrets(ctx context.Context) (port.SecretStore, func(), error) {
	switch u, _ := b.one(config.PortSecrets); {
	case u.Tier == adapters.TierMemory:
		return secretsmemory.New(), func() {}, nil
	case u.Name == "hostkey":
		s, err := hostkey.Open(ctx, b.cfg.Secrets.Dir, b.cfg.Secrets.UnlockSocket, b.log)
		if err != nil {
			return nil, nil, err
		}
		return s, s.Close, nil
	}
	s, err := secretslocal.Open(b.cfg.Secrets.Dir)
	if err != nil {
		return nil, nil, err
	}
	return s, func() {}, nil
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

func (b builder) runtime(store port.Store, secrets port.SecretStore) (port.ConnectorRuntime, error) {
	applier := credential.Applier{Resolve: credential.NewResolver(store, secrets), Injector: credential.Injector{}}
	var runtimes []port.ConnectorRuntime
	for _, u := range b.uses(config.PortRuntime) {
		switch {
		case u.Tier == adapters.TierMemory:
			runtimes = append(runtimes, rtmemory.New())
		case u.Name == "remote":
			runtimes = append(runtimes, rtremote.New(applier, nil, b.guard(nil)))
		case u.Name == "declarative":
			runtimes = append(runtimes, rtdeclarative.New(applier, nil, b.guard(nil)))
		case u.Name == "skill":
			runtimes = append(runtimes, rtskill.New())
		case u.Name == "grpc":
			runtimes = append(runtimes, rtgrpc.New(rtgrpc.Options{Applier: applier, Insecure: b.cfg.GRPC.Insecure, Guard: b.guard(nil)}))
		}
	}
	return mux.New(runtimes...)
}

func (b builder) optionalRef(where, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	v, err := resolveRef(b.lookup, ref)
	if err != nil {
		return "", fmt.Errorf("%s: %w", where, err)
	}
	return v, nil
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

func (b builder) sources() (map[string]port.ListingSource, error) {
	out := map[string]port.ListingSource{}
	add := func(s port.ListingSource) { out[s.Name()] = s }
	for _, u := range b.uses(config.PortSource) {
		switch {
		case u.Tier == adapters.TierMemory:
			continue
		case u.Name == "files":
			add(files.New(b.cfg.Sources.Files.Dir, b.clock.Now))
		case u.Name == "docker":
			add(docker.New(b.cfg.Sources.Docker.Dir, b.clock.Now))
		case u.Name == "official":
			token, err := b.optionalRef("sources.official.tokenRef", b.cfg.Sources.Official.TokenRef)
			if err != nil {
				return nil, err
			}
			add(registry.New(registry.Config{Token: token, BaseURL: b.cfg.Sources.Official.URL, Guard: b.guard(hostsOf(b.cfg.Sources.Official.URL)), NamespaceVerified: true, Now: b.clock.Now}))
		case u.Name == "bsr":
			token, err := b.optionalRef("sources.bsr.tokenRef", b.cfg.Sources.BSR.TokenRef)
			if err != nil {
				return nil, err
			}
			add(bsr.New(bsr.Config{BaseURL: b.cfg.Sources.BSR.URL, Token: token, Owners: b.cfg.Sources.BSR.Owners, Targets: b.cfg.Sources.BSR.Targets,
				Guard: b.guard(hostsOf(b.cfg.Sources.BSR.URL)), Now: b.clock.Now}))
		case u.Name == "openapi":
			add(openapi.New(openapi.Config{Dir: b.cfg.Sources.OpenAPI.Dir, URLs: b.cfg.Sources.OpenAPI.URLs, Guard: b.guard(hostsOf(b.cfg.Sources.OpenAPI.URLs...)), Now: b.clock.Now}))
		case u.Name == "skillcatalog":
			add(skillcatalog.New(skillcatalog.Config{URLs: b.cfg.Sources.SkillCatalog.URLs, Guard: b.guard(nil), Now: b.clock.Now}))
		case u.Name == "reflection":
			add(reflection.New(reflection.Config{Targets: b.cfg.Sources.Reflection.Targets, Guard: b.guard(b.cfg.Sources.Reflection.Targets), Insecure: b.cfg.Sources.Reflection.Insecure, Now: b.clock.Now}))
		}
	}
	return out, nil
}

func (b builder) sink() (port.EventSink, error) {
	u, _ := b.one(config.PortSink)
	switch {
	case u.Tier == adapters.TierMemory:
		return sinkmemory.New(), nil
	case u.Name == "webhook":
		c := b.file.Ports[config.PortSink].Config
		secret := ""
		if ref := configString(c, "secretRef"); ref != "" {
			var err error
			if secret, err = resolveRef(b.lookup, ref); err != nil {
				return nil, fmt.Errorf("sink: %w", err)
			}
		}
		timeout, _ := time.ParseDuration(configString(c, "timeout"))
		return sinkwebhook.New(sinkwebhook.Config{URL: configString(c, "url"), Secret: secret, Timeout: timeout})
	}
	return sinklog.New(b.log), nil
}

func (b builder) filter() port.ResultFilter {
	if _, ok := b.one(config.PortFilter); ok {
		return filterbuiltin.New()
	}
	return nil
}

func (b builder) index() port.ToolIndex { return lexical.New() }

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
