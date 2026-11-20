package console

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

// ProfileMode is how one access key reaches one connector: off when it is not bound, follow when the connector's own choices decide, custom when it narrows some of them.
type ProfileMode string

// The three modes of a profile.
const (
	ProfileOff    ProfileMode = "off"
	ProfileFollow ProfileMode = "follow"
	ProfileCustom ProfileMode = "custom"
)

// Profile is one access key's permissions on one connector: its mode and the functions and items it holds below the connector's choice, which can only narrow it since the stricter row wins.
type Profile struct {
	Agent     string            `yaml:"key" json:"agent"`
	Mode      ProfileMode       `yaml:"mode" json:"mode"`
	Overrides map[string]Access `yaml:"overrides,omitempty" json:"overrides,omitempty"`
	Expired   bool              `yaml:"-" json:"expired,omitempty"`
}

// Effect is the engine's answer for one access key on one function or item: the access it gets and, when the engine narrows or refuses it, why.
type Effect struct {
	Name   string `json:"name"`
	Access Access `json:"access"`
	Reason string `json:"reason,omitempty"`
}

func isOverride(g port.Grant) bool { return grant.Specificity(g.Selector) == 2 }

func (c *Console) boundKeys(ctx context.Context, conn string) (map[string]bool, error) {
	binds, err := c.Engine.bindings(ctx, conn)
	out := map[string]bool{}
	for _, b := range binds {
		out[string(b.Agent)] = true
	}
	return out, err
}

func (c *Console) profiles(ctx context.Context, conn string) ([]Profile, error) {
	bound, err := c.boundKeys(ctx, conn)
	if err != nil {
		return nil, err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, "", conn)
	if err != nil {
		return nil, err
	}
	out := []Profile{}
	for _, a := range c.knownAgents() {
		p := Profile{Agent: a.Name, Mode: ProfileOff, Expired: !a.ExpiresAt.After(time.Now())}
		if bound[a.Name] {
			p.Mode = ProfileFollow
			for _, g := range rows {
				if g.ScopeID == a.Name && isOverride(g) {
					access, _ := accessFor(g.Mode)
					put(&p.Overrides, strings.TrimPrefix(g.Selector, port.SelectorFunction), access)
					p.Mode = ProfileCustom
				}
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (c *Console) plainConnector(ctx context.Context, conn string) error {
	v, err := c.Engine.connector(ctx, conn)
	if err != nil {
		return err
	}
	skill, err := c.isSkill(ctx, v)
	if err == nil && (skill || !v.RemovedAt.IsZero()) {
		err = &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no connector is installed as " + conn}
	}
	return err
}

// SetProfileMode sets the named access keys, or every key made so far when none is named, expired ones included, to a mode on a connector (off unbinds, follow binds without overrides, custom binds and keeps them) and answers each key's result in order.
func (c *Console) SetProfileMode(ctx context.Context, conn string, names []string, mode ProfileMode) ([]KeyResult, error) {
	set, ok := map[ProfileMode]func(context.Context, string, Agent) error{ProfileOff: c.unbind, ProfileFollow: c.follow, ProfileCustom: c.bind}[mode]
	if !ok {
		return nil, invalid("mode %q is not off, follow or custom", mode)
	}
	if err := c.plainConnector(ctx, conn); err != nil {
		return nil, err
	}
	return eachKey(c.knownAgents(), names, func(a Agent) error { return set(ctx, conn, a) }), nil
}

func (c *Console) unbind(ctx context.Context, conn string, a Agent) error {
	return c.Engine.unbind(ctx, conn, a.Name)
}

func (c *Console) follow(ctx context.Context, conn string, a Agent) error {
	if err := c.bind(ctx, conn, a); err != nil {
		return err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, a.Name, conn)
	if err != nil {
		return err
	}
	return c.Engine.deleteGrants(ctx, slices.DeleteFunc(rows, func(g port.Grant) bool { return !isOverride(g) }))
}

// SetProfileAccess narrows one function or item for one access key below the connector's choice, binding a key that was off, or clears the narrowing when access is the connector's own choice; a wider access is refused.
func (c *Console) SetProfileAccess(ctx context.Context, conn, agent, name string, access Access) error {
	kind, item := itemKinds[port.ItemKind(name)]
	if !access.Valid() || item && !validFor(kind, access) {
		return invalid("access %q is not a choice for %s", access, name)
	}
	if err := c.plainConnector(ctx, conn); err != nil {
		return err
	}
	a, err := agentNamed(c.knownAgents(), agent)
	if err != nil {
		return err
	}
	return c.narrow(ctx, conn, a, name, access)
}

func (c *Console) narrow(ctx context.Context, conn string, a Agent, name string, access Access) error {
	f, err := c.function(ctx, conn, name)
	if err != nil {
		return err
	}
	rows, err := c.workspaceGrants(ctx, conn)
	if err != nil {
		return err
	}
	base := functionAccess(f, rows)
	if access.mode().Rank() > base.mode().Rank() {
		return invalid("%s is %s on this connector, and a key can only narrow it", name, base)
	}
	bound, err := c.boundKeys(ctx, conn)
	if err != nil {
		return err
	}
	if !bound[a.Name] {
		if err := c.bind(ctx, conn, a); err != nil {
			return err
		}
	}
	if access != base {
		return c.writeOverride(ctx, conn, a.Name, f, access)
	}
	keyRows, err := c.Engine.grants(ctx, port.ScopeAgent, a.Name, conn)
	if err != nil {
		return err
	}
	return c.Engine.deleteGrants(ctx, slices.DeleteFunc(keyRows, func(g port.Grant) bool { return g.Selector != port.SelectorOf(name) }))
}

func (c *Console) writeOverride(ctx context.Context, conn, agent string, f port.Function, access Access) error {
	return c.Engine.createGrant(ctx, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: agent, Agent: port.AgentID(agent), Connector: port.ConnectorID(conn), Selector: port.SelectorOf(f.Name), Mode: access.mode(), DefinitionHash: f.DefinitionHash})
}

func (c *Console) applyProfile(ctx context.Context, conn string, a Agent, p Profile) error {
	if p.Mode == ProfileOff {
		return c.unbind(ctx, conn, a)
	}
	if err := c.follow(ctx, conn, a); err != nil {
		return err
	}
	fns, err := c.Engine.functions(ctx, conn)
	if err != nil {
		return err
	}
	for _, f := range fns {
		if access, ok := p.Overrides[f.Name]; ok && access.Valid() && access != AccessOn && f.State != port.FunctionRemoved {
			if err := c.writeOverride(ctx, conn, a.Name, f, access); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Console) confirmOverrides(ctx context.Context, conn string, f port.Function) error {
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, "", conn)
	if err != nil {
		return err
	}
	for _, g := range rows {
		if g.Selector == port.SelectorOf(f.Name) && grant.Stale(g, f) {
			if err := c.Engine.confirmGrant(ctx, g.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProfileEffects asks the engine what one access key gets for each function and item of a connector, as tools/list and every call decide it.
func (c *Console) ProfileEffects(ctx context.Context, conn, agent string) ([]Effect, error) {
	if err := c.plainConnector(ctx, conn); err != nil {
		return nil, err
	}
	if _, err := agentNamed(c.knownAgents(), agent); err != nil {
		return nil, err
	}
	fns, err := c.Engine.functions(ctx, conn)
	if err != nil {
		return nil, err
	}
	out := []Effect{}
	for _, f := range fns {
		if f.State == port.FunctionRemoved {
			continue
		}
		d, err := c.Engine.explain(ctx, conn, agent, f.Name)
		if err != nil {
			return nil, err
		}
		e := Effect{Name: f.Name, Reason: string(d.Reason)}
		e.Access, _ = accessFor(d.Mode)
		if e.Reason == "" && len(d.Notes) > 0 {
			e.Reason = string(d.Notes[0])
		}
		out = append(out, e)
	}
	return out, nil
}

// SetNewKeysOff records whether access keys made from now on start off on a connector instead of following its choices; keys already made keep theirs.
func (c *Console) SetNewKeysOff(ctx context.Context, conn string, off bool) error {
	if err := c.plainConnector(ctx, conn); err != nil {
		return err
	}
	return c.State.SetNewKeysOff(conn, off)
}

func (c *Console) join(ctx context.Context, conn port.Connector, a Agent, known bool) error {
	id := string(conn.ID)
	if skill, err := c.isSkill(ctx, conn); err != nil || skill {
		if err != nil {
			return err
		}
		return c.rebindSkill(ctx, id, a)
	}
	bound, err := c.boundKeys(ctx, id)
	switch {
	case err != nil:
		return err
	case bound[a.Name]:
		return c.bind(ctx, id, a)
	case known:
		return nil
	}
	if p, ok := c.State.PendingProfile(id, a.Name); ok {
		return c.applyProfile(ctx, id, a, p)
	}
	if c.State.NewKeysOff(id) {
		return nil
	}
	return c.bind(ctx, id, a)
}
