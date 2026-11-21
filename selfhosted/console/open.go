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

// Options is what one console composes: its public URL, the engine it drives, its state file, how its key store opens, its first Owner's and service persons, its managers group and how it asks to restart.
type Options struct {
	PublicURL     string
	EngineURL     string
	OperatorURL   string
	Operator      string
	StatePath     string
	Unlock        func(ctx context.Context) (port.SecretStore, error)
	Tenant        string
	Workspace     string
	Person        string
	ServicePerson string
	ManagersGroup string
	SetupToken    string
	LoopbackHost  string
	AgentTTL      time.Duration
	Restart       func()
	Log           *slog.Logger
}

// Open opens the state and activity of o, unlocks the key store and, on ctx, prepares the tenant and the users' engine roles and runs the notification loop; the closer flushes the activity.
func Open(ctx context.Context, o Options) (*Console, func(), error) {
	public, err := url.Parse(strings.TrimRight(o.PublicURL, "/"))
	if err != nil || public.Host == "" {
		return nil, nil, invalid("the public URL %q is not an absolute URL", o.PublicURL)
	}
	if firstOf(o.ServicePerson, defaultServicePerson) == firstOf(o.Person, defaultPerson) {
		return nil, nil, invalid("the service person %q is also the first Owner's person", firstOf(o.Person, defaultPerson))
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
	eng := &Engine{PublicURL: o.EngineURL, OperatorURL: o.OperatorURL, Operator: o.Operator, Tenant: o.Tenant, Workspace: o.Workspace, ServicePerson: o.ServicePerson}
	c := &Console{Engine: eng, State: state, Sessions: &Sessions{state: state, Secure: public.Scheme == "https"}, Guard: Guard{Hosts: LoopbackHosts(public)}, Web: web.App(), Secrets: box,
		PublicURL: public.String(), MCPURL: public.String() + "/mcp", OAuthRedirect: public.String() + "/oauth/callback", MaxAgentTTL: o.AgentTTL, Log: o.Log, Activity: activity, Restart: o.Restart, LoopbackHost: o.LoopbackHost,
		ManagersGroup: o.ManagersGroup, SetupToken: o.SetupToken}
	go c.prepareEngine(ctx)
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

func (c *Console) prepareEngine(ctx context.Context) {
	if c.untilReady(ctx, "tenant", c.Engine.EnsureTenant) {
		c.untilReady(ctx, "roles", func(ctx context.Context) error { return c.ReconcileRoles(ctx) })
		c.untilReady(ctx, "classes", func(ctx context.Context) error { return c.RepairClasses(WithActor(ctx, c.Engine.service())) })
	}
}

func (c *Console) untilReady(ctx context.Context, step string, run func(context.Context) error) bool {
	for {
		err := run(ctx)
		if err == nil {
			c.logger().Info(step+" ready", "tenant", c.Engine.Tenant)
			return true
		}
		c.logger().Warn("waiting for the engine", "step", step, "error", err)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
}
