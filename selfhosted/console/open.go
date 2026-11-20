package console

import (
	"context"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"github.com/versatile-sh/connector-engine/selfhosted/console/web"
)

// Options is what one console composes: its public URL, the engine it drives, its state file, how its key store opens and how it asks to restart.
type Options struct {
	PublicURL    string
	EngineURL    string
	OperatorURL  string
	Operator     string
	StatePath    string
	Unlock       func(ctx context.Context) (port.SecretStore, error)
	Tenant       string
	Workspace    string
	Person       string
	LoopbackHost string
	AgentTTL     time.Duration
	Restart      func()
	Log          *slog.Logger
}

// Open opens the state and activity of o, unlocks the key store and starts the tenant and notification loops on ctx; the closer flushes the activity.
func Open(ctx context.Context, o Options) (*Console, func(), error) {
	public, err := url.Parse(strings.TrimRight(o.PublicURL, "/"))
	if err != nil || public.Host == "" || public.Port() == "" {
		return nil, nil, invalid("the public URL %q is not an absolute URL with a port", o.PublicURL)
	}
	state, err := OpenState(o.StatePath, o.Person)
	if err != nil {
		return nil, nil, err
	}
	activity, err := OpenActivity(filepath.Join(filepath.Dir(o.StatePath), "activity"), o.Log)
	if err != nil {
		return nil, nil, err
	}
	box := &sealed.Box{}
	go unlockBox(ctx, box, o.Unlock, o.Log)
	eng := &Engine{PublicURL: o.EngineURL, OperatorURL: o.OperatorURL, Operator: o.Operator, Tenant: o.Tenant, Workspace: o.Workspace, Person: o.Person}
	c := &Console{Engine: eng, State: state, Sessions: NewSessions(state), Guard: Guard{Hosts: LoopbackHosts(public)}, Web: web.App(), Secrets: box,
		MCPURL: public.String() + "/mcp", OAuthRedirect: public.String() + "/oauth/callback", MaxAgentTTL: o.AgentTTL, Log: o.Log, Activity: activity, Restart: o.Restart, LoopbackHost: o.LoopbackHost}
	go ensureTenant(ctx, eng, o.Log)
	go c.RunNotifications(ctx)
	return c, activity.Close, nil
}

func unlockBox(ctx context.Context, box *sealed.Box, unlock func(context.Context) (port.SecretStore, error), log *slog.Logger) {
	store, err := unlock(ctx)
	if err != nil {
		log.Error("the console's secrets stay locked", "err", err)
		return
	}
	box.Unlock(store)
	log.Info("the console's secrets are unlocked", "protection", box.Protection())
}

func ensureTenant(ctx context.Context, eng *Engine, log *slog.Logger) {
	for {
		err := eng.EnsureTenant(ctx)
		if err == nil {
			log.Info("tenant ready", "tenant", eng.Tenant)
			return
		}
		log.Warn("waiting for the engine", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
