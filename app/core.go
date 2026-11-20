package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"

	"github.com/versatile-sh/connector-engine/config"
	filterbuiltin "github.com/versatile-sh/connector-engine/filter/builtin"
	"github.com/versatile-sh/connector-engine/index/lexical"
	"github.com/versatile-sh/connector-engine/port"
	rtdeclarative "github.com/versatile-sh/connector-engine/runtime/declarative"
	rtgrpc "github.com/versatile-sh/connector-engine/runtime/grpc"
	rtmemory "github.com/versatile-sh/connector-engine/runtime/memory"
	rtremote "github.com/versatile-sh/connector-engine/runtime/remote"
	rtskill "github.com/versatile-sh/connector-engine/runtime/skill"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	secretsmemory "github.com/versatile-sh/connector-engine/secrets/memory"
	"github.com/versatile-sh/connector-engine/secrets/onepassword"
	sinklog "github.com/versatile-sh/connector-engine/sink/log"
	sinkmemory "github.com/versatile-sh/connector-engine/sink/memory"
	sinkwebhook "github.com/versatile-sh/connector-engine/sink/webhook"
	"github.com/versatile-sh/connector-engine/source/bsr"
	"github.com/versatile-sh/connector-engine/source/docker"
	"github.com/versatile-sh/connector-engine/source/files"
	"github.com/versatile-sh/connector-engine/source/openapi"
	"github.com/versatile-sh/connector-engine/source/reflection"
	"github.com/versatile-sh/connector-engine/source/registry"
	"github.com/versatile-sh/connector-engine/source/skillcatalog"
	storememory "github.com/versatile-sh/connector-engine/store/memory"
	"github.com/versatile-sh/connector-engine/store/postgres"
)

const memory = adapters.TierMemory

var envelope = []string{"secrets.envelope", "secrets.crypto_shredding"}

// Core returns a registry holding the engine's own adapters; a build adds its own with Register.
func Core() *Registry {
	r := NewRegistry()
	Register(r, Stores, Adapter{Name: memory, Capabilities: []string{"store.volatile"}}, func(context.Context, Env) (port.Store, func(), error) {
		return storememory.New(), nil, nil
	})
	Register(r, Stores, Adapter{Name: "postgres", Capabilities: []string{"store.transactional_outbox", "store.row_level_security"},
		Settings: map[string]adapters.ConfigKind{"databaseUrlRef": adapters.ConfigRef}}, openPostgres)

	Register(r, Secrets, Adapter{Name: memory, Capabilities: []string{"secrets.volatile"}, Check: plaintextRootKey(memory)}, func(context.Context, Env) (port.SecretStore, func(), error) {
		return secretsmemory.New(), nil, nil
	})
	Register(r, Secrets, Adapter{Name: "localfile", Capabilities: envelope, Check: plaintextRootKey("localfile")}, func(_ context.Context, env Env) (port.SecretStore, func(), error) {
		s, err := secretslocal.Open(env.Config.Secrets.Dir)
		return s, nil, err
	})
	Register(r, Secrets, Adapter{Name: "onepassword", Capabilities: append(envelope[:2:2], "secrets.root_key_wrapped"), Check: checkOnePassword}, openOnePassword)

	Register(r, Runtimes, Adapter{Name: memory}, func(context.Context, Env) (port.ConnectorRuntime, func(), error) {
		return rtmemory.New(), nil, nil
	})
	Register(r, Runtimes, Adapter{Name: "remote"}, func(_ context.Context, env Env) (port.ConnectorRuntime, func(), error) {
		return rtremote.New(env.Applier, nil, env.Guard()), nil, nil
	})
	Register(r, Runtimes, Adapter{Name: "declarative"}, func(_ context.Context, env Env) (port.ConnectorRuntime, func(), error) {
		return rtdeclarative.New(env.Applier, nil, env.Guard()), nil, nil
	})
	Register(r, Runtimes, Adapter{Name: "grpc"}, func(_ context.Context, env Env) (port.ConnectorRuntime, func(), error) {
		return rtgrpc.New(rtgrpc.Options{Applier: env.Applier, Insecure: env.Config.GRPC.Insecure, Guard: env.Guard()}), nil, nil
	})
	Register(r, Runtimes, Adapter{Name: "skill"}, func(context.Context, Env) (port.ConnectorRuntime, func(), error) {
		return rtskill.New(), nil, nil
	})

	Register(r, Sources, Adapter{Name: memory}, func(context.Context, Env) (port.ListingSource, func(), error) { return nil, nil, nil })
	Register(r, Sources, Adapter{Name: "files"}, func(_ context.Context, env Env) (port.ListingSource, func(), error) {
		return files.New(env.Config.Sources.Files.Dir, env.Clock.Now), nil, nil
	})
	Register(r, Sources, Adapter{Name: "official"}, openOfficial)
	Register(r, Sources, Adapter{Name: "docker"}, func(_ context.Context, env Env) (port.ListingSource, func(), error) {
		return docker.New(env.Config.Sources.Docker.Dir, env.Clock.Now), nil, nil
	})
	Register(r, Sources, Adapter{Name: "openapi"}, func(_ context.Context, env Env) (port.ListingSource, func(), error) {
		c := env.Config.Sources.OpenAPI
		return openapi.New(openapi.Config{Dir: c.Dir, URLs: c.URLs, Guard: env.Guard(hostsOf(c.URLs...)...), Now: env.Clock.Now}), nil, nil
	})
	Register(r, Sources, Adapter{Name: "reflection"}, func(_ context.Context, env Env) (port.ListingSource, func(), error) {
		c := env.Config.Sources.Reflection
		return reflection.New(reflection.Config{Targets: c.Targets, Guard: env.Guard(c.Targets...), Insecure: c.Insecure, Now: env.Clock.Now}), nil, nil
	})
	Register(r, Sources, Adapter{Name: "bsr"}, openBSR)
	Register(r, Sources, Adapter{Name: "skillcatalog"}, func(_ context.Context, env Env) (port.ListingSource, func(), error) {
		return skillcatalog.New(skillcatalog.Config{URLs: env.Config.Sources.SkillCatalog.URLs, Guard: env.Guard(), Now: env.Clock.Now}), nil, nil
	})

	Register(r, Sinks, Adapter{Name: memory}, func(context.Context, Env) (port.EventSink, func(), error) { return sinkmemory.New(), nil, nil })
	Register(r, Sinks, Adapter{Name: "log"}, func(_ context.Context, env Env) (port.EventSink, func(), error) {
		return sinklog.New(env.Log), nil, nil
	})
	Register(r, Sinks, Adapter{Name: "webhook", Settings: map[string]adapters.ConfigKind{"url": adapters.ConfigString, "secretRef": adapters.ConfigRef, "timeout": adapters.ConfigString}}, openWebhook)

	Register(r, Filters, Adapter{Name: "builtin"}, func(_ context.Context, env Env) (port.ResultFilter, func(), error) {
		f, err := filterbuiltin.New(env.Config.ResultFilter.Locales...)
		if err != nil {
			return nil, nil, fmt.Errorf("resultFilter.locales: %w", err)
		}
		return f, nil, nil
	})
	Register(r, Indexes, Adapter{Name: "lexical"}, func(context.Context, Env) (port.ToolIndex, func(), error) { return lexical.New(), nil, nil })
	return r
}

func openPostgres(ctx context.Context, env Env) (port.Store, func(), error) {
	dsn, err := resolveRef(env.Lookup, env.Setting("databaseUrlRef"))
	if err != nil {
		return nil, nil, fmt.Errorf("store: %w", err)
	}
	s, err := postgres.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("store: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, s.Close, nil
}

var vaultHTTP = &http.Client{Timeout: 30 * time.Second}

func plaintextRootKey(name string) func(config.Config, func(string) bool) config.Findings {
	return func(c config.Config, acknowledged func(string) bool) config.Findings {
		if acknowledged(config.AckPlaintextRootKey) {
			return nil
		}
		return config.Findings{{Level: c.RefusalLevel(), Where: "port secrets", Message: name + " keeps the root key in plaintext; acknowledge " + config.AckPlaintextRootKey + " to run with it"}}
	}
}

func checkOnePassword(c config.Config, _ func(string) bool) config.Findings {
	if op := c.Secrets.OnePassword; op.ConnectURL == "" || op.TokenRef == "" || op.Vault == "" || op.Item == "" || op.Field == "" {
		return config.Findings{{Level: config.LevelError, Where: "secrets.onePassword", Message: "the onepassword adapter needs connectUrl, tokenRef, vault, item and field"}}
	}
	return nil
}

func openOnePassword(ctx context.Context, env Env) (port.SecretStore, func(), error) {
	c := env.Config.Secrets.OnePassword
	token, err := resolveRef(env.Lookup, c.TokenRef)
	if err != nil {
		return nil, nil, fmt.Errorf("secrets.onePassword.tokenRef: %w", err)
	}
	s, err := onepassword.Open(ctx, env.Config.Secrets.Dir, onepassword.Config{ConnectURL: c.ConnectURL, Token: token, Vault: c.Vault, Item: c.Item, Field: c.Field}, vaultHTTP)
	return s, nil, err
}

func openOfficial(_ context.Context, env Env) (port.ListingSource, func(), error) {
	c := env.Config.Sources.Official
	token, err := env.Ref("sources.official.tokenRef", c.TokenRef)
	if err != nil {
		return nil, nil, err
	}
	return registry.New(registry.Config{Token: token, BaseURL: c.URL, Guard: env.Guard(hostsOf(c.URL)...), NamespaceVerified: true, Now: env.Clock.Now}), nil, nil
}

func openBSR(_ context.Context, env Env) (port.ListingSource, func(), error) {
	c := env.Config.Sources.BSR
	token, err := env.Ref("sources.bsr.tokenRef", c.TokenRef)
	if err != nil {
		return nil, nil, err
	}
	return bsr.New(bsr.Config{BaseURL: c.URL, Token: token, Owners: c.Owners, Targets: c.Targets, Guard: env.Guard(hostsOf(c.URL)...), Now: env.Clock.Now}), nil, nil
}

func openWebhook(_ context.Context, env Env) (port.EventSink, func(), error) {
	secret, err := env.Ref("sink", env.Setting("secretRef"))
	if err != nil {
		return nil, nil, err
	}
	timeout, _ := time.ParseDuration(env.Setting("timeout"))
	s, err := sinkwebhook.New(sinkwebhook.Config{URL: env.Setting("url"), Secret: secret, Timeout: timeout})
	return s, nil, err
}
