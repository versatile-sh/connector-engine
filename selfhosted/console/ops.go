package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

// FunctionView is one function of a connector as the console shows it: its class, what it does by nature, and the owner's choice read back from the engine's grants.
type FunctionView struct {
	Name        string     `json:"name"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Class       port.Class `json:"class"`
	Kind        port.Class `json:"kind"`
	State       string     `json:"state"`
	Access      Access     `json:"access"`
}

// ConnectorSummary is one installed connector with its listing's presentation, its functions and items, the profile of each access key made so far and the permission sets that hold a profile for it.
type ConnectorSummary struct {
	ID          string         `json:"id"`
	Alias       string         `json:"alias"`
	Enabled     bool           `json:"enabled"`
	Blocked     string         `json:"blocked,omitempty"`
	Listing     string         `json:"listing"`
	Version     string         `json:"version"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Publisher   string         `json:"publisher,omitempty"`
	RemoteURL   string         `json:"remote_url,omitempty"`
	Transport   string         `json:"transport,omitempty"`
	Auth        []string       `json:"auth,omitempty"`
	Credentials []string       `json:"credentials,omitempty"`
	Functions   []FunctionView `json:"functions"`
	Items       []Item         `json:"items"`
	Profiles    []Profile      `json:"profiles"`
	Slots       []SlotProfile  `json:"slots"`
	NewKeysOff  bool           `json:"new_keys_off,omitempty"`
}

func (c *Console) workspaceGrants(ctx context.Context, conn string) ([]port.Grant, error) {
	return c.Engine.grants(ctx, port.ScopeWorkspace, c.Engine.Workspace, conn)
}

func accessOf(rows []port.Grant, match func(port.Grant) bool) Access {
	for _, g := range rows {
		if a, ok := accessFor(g.Mode); ok && match(g) {
			return a
		}
	}
	return AccessOff
}

func functionAccess(f port.Function, rows []port.Grant) Access {
	return accessOf(rows, func(g port.Grant) bool { return g.Selector == port.SelectorOf(f.Name) })
}

func describe(def json.RawMessage) (title, description string) {
	var d struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(def, &d)
	return d.Title, d.Description
}

// Connectors lists the workspace's connectors with their listing, functions, credentials and the owner's choices.
func (c *Console) Connectors(ctx context.Context) ([]ConnectorSummary, error) {
	views, err := c.Engine.connectors(ctx)
	if err != nil {
		return nil, err
	}
	agentRows, err := c.Engine.grants(ctx, port.ScopeAgent, "", "")
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorSummary, 0, len(views))
	for _, v := range views {
		conn := v.Connector
		if !conn.RemovedAt.IsZero() {
			continue
		}
		s := ConnectorSummary{ID: string(conn.ID), Alias: conn.Alias, Enabled: conn.Enabled, Blocked: v.Blocked, Listing: conn.Listing.Listing.Name, Version: conn.Listing.Version, Title: conn.Alias}
		if m, err := c.Engine.manifest(ctx, conn); err == nil {
			if m.Kind == port.KindSkill {
				continue
			}
			s.Title, s.Description, s.Icon, s.Publisher = firstOf(m.Title, m.Name), m.Description, m.Icon, m.Publisher.Name
			if m.Remote != nil {
				s.RemoteURL, s.Transport = m.Remote.URL, m.Remote.Transport
			}
			for _, a := range m.Auth {
				s.Auth = append(s.Auth, string(a.Kind))
			}
		}
		fns, err := c.Engine.functions(ctx, string(conn.ID))
		if err != nil {
			return nil, err
		}
		rows, err := c.workspaceGrants(ctx, string(conn.ID))
		if err != nil {
			return nil, err
		}
		s.Items = []Item{}
		for _, f := range fns {
			if f.State == port.FunctionRemoved {
				continue
			}
			if it, ok := ItemOf(f.Name, f.Definition); ok {
				it.State, it.Access = string(f.State), functionAccess(f, rows)
				s.Items = append(s.Items, it)
				continue
			}
			title, desc := describe(f.Definition)
			s.Functions = append(s.Functions, FunctionView{Name: f.Name, Title: title, Description: desc, Class: f.Class, Kind: Nature(f), State: string(f.State), Access: functionAccess(f, rows)})
		}
		if s.Profiles, err = c.profiles(ctx, string(conn.ID), agentRows); err != nil {
			return nil, err
		}
		s.NewKeysOff, s.Slots = c.State.NewKeysOff(string(conn.ID)), c.slotsOn(string(conn.ID), conn.Alias)
		if creds, err := c.Engine.credentials(ctx, string(conn.ID)); err == nil {
			for _, cr := range creds {
				s.Credentials = append(s.Credentials, string(cr.Kind)+":"+string(cr.Status))
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Console) function(ctx context.Context, conn, name string) (port.Function, error) {
	fns, err := c.Engine.functions(ctx, conn)
	if err != nil {
		return port.Function{}, err
	}
	if i := slices.IndexFunc(fns, func(f port.Function) bool { return f.Name == name }); i >= 0 {
		return fns[i], nil
	}
	return port.Function{}, &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "function " + name + " not found"}
}

// SetAccess writes the owner's choice for one function or item: it accepts a quarantined definition the owner now sees, classes a function read or write by its nature (write for destructive, so on is never capped to ask), replaces its workspace row (none for off; ask is ask_once so a session answer is possible), and gives every bound key the item rows of the connector.
func (c *Console) SetAccess(ctx context.Context, conn, name string, a Access) error {
	results, err := c.SetAccessAll(ctx, conn, []string{name}, a)
	if err != nil {
		return err
	}
	for _, r := range results {
		if r.Err != nil {
			return r.Err
		}
	}
	return nil
}

// SetAccessAll writes one choice for many functions or items of a connector, as SetAccess does for one, lifts the new-key limits of the names written, and answers each name's result in order, then one entry per bound key it failed to give the item rows.
func (c *Console) SetAccessAll(ctx context.Context, conn string, names []string, a Access) ([]Result, error) {
	switch {
	case len(names) == 0:
		return nil, invalid("name at least one function or item")
	case !a.Valid():
		return nil, invalid("access %q is not off, on or ask", a)
	}
	if err := checkBatch(names); err != nil {
		return nil, err
	}
	if err := c.plainConnector(ctx, conn); err != nil {
		return nil, err
	}
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer unlock()
	results := make([]Result, 0, len(names))
	items, written := false, []string{}
	for _, name := range names {
		err := c.writeAccess(ctx, conn, name, a)
		results = append(results, Result{Name: name, Err: err})
		items = items || err == nil && port.ItemKind(name) != ""
		if err == nil {
			written = append(written, name)
		}
	}
	if err := c.State.LiftNewKeyLimits(conn, written); err != nil {
		results = append(results, Result{Name: "keys", Err: err})
	}
	if !items || a == AccessOff {
		return results, nil
	}
	bound, err := c.boundKeys(ctx, conn)
	if err != nil {
		return append(results, Result{Name: "keys", Err: err}), nil
	}
	for _, k := range c.liveAgents() {
		if !bound[k.Name] {
			continue
		}
		if err := c.bind(ctx, conn, k); err != nil {
			results = append(results, Result{Name: k.Name, Err: fmt.Errorf("key %s keeps its old item rows: %w", k.Name, err)})
		}
	}
	return results, nil
}

func (c *Console) bindAll(ctx context.Context, conn string, bind func(context.Context, string, Agent) error) error {
	for _, a := range c.liveAgents() {
		if err := bind(ctx, conn, a); err != nil {
			return err
		}
	}
	return nil
}

func (c *Console) writeAccess(ctx context.Context, conn, name string, a Access) error {
	kind, item := itemKinds[port.ItemKind(name)]
	if !a.Valid() || item && !validFor(kind, a) {
		return invalid("access %q is not a choice for %s", a, name)
	}
	f, err := c.function(ctx, conn, name)
	if err != nil {
		return err
	}
	if f.State == port.FunctionQuarantined && a != AccessOff {
		if err := c.Engine.accept(ctx, conn, map[string]port.Hash{name: f.DefinitionHash}); err != nil {
			return err
		}
		if f, err = c.function(ctx, conn, name); err != nil {
			return err
		}
		if err := c.confirmOverrides(ctx, conn, f); err != nil {
			return err
		}
	}
	class := port.ClassWrite
	if Nature(f) == port.ClassRead {
		class = port.ClassRead
	}
	if !item && class != f.Class {
		if f, err = c.Engine.setClass(ctx, conn, name, class); err != nil {
			return err
		}
	}
	rows, err := c.workspaceGrants(ctx, conn)
	if err != nil {
		return err
	}
	if err := c.Engine.deleteGrants(ctx, slices.DeleteFunc(rows, func(g port.Grant) bool { return g.Selector != port.SelectorOf(name) })); err != nil {
		return err
	}
	if a == AccessOff {
		return nil
	}
	return c.Engine.createGrant(ctx, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: c.Engine.Workspace, Connector: port.ConnectorID(conn), Selector: port.SelectorOf(name), Mode: a.mode(), DefinitionHash: f.DefinitionHash})
}

func (c *Console) freeAlias(ctx context.Context, base string) (string, error) {
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, conn := range live {
		taken[conn.Alias] = true
	}
	base = Slug(base)
	if len(base) > 24 {
		base = strings.Trim(base[:24], "-")
	}
	if base == "" || base[0] < 'a' {
		base = strings.Trim("mcp-"+base, "-")
	}
	alias := base
	for i := 2; taken[alias]; i++ {
		alias = fmt.Sprintf("%s-%d", base, i)
	}
	return alias, nil
}

func (c *Console) installListing(ctx context.Context, m port.Manifest, aliasBase func(port.ListingRef) string) (string, []port.Function, error) {
	lv, err := c.Engine.createListing(ctx, m)
	if err != nil {
		return "", nil, err
	}
	ref := lv.Ref.Listing
	if err := c.openToWorkspace(ctx, string(ref.Owner)+"/"+ref.Name); err != nil {
		return "", nil, err
	}
	alias, err := c.freeAlias(ctx, aliasBase(ref))
	if err != nil {
		return "", nil, err
	}
	conn, err := c.Engine.install(ctx, alias, lv.Ref)
	if err != nil {
		return "", nil, err
	}
	fns, err := c.Engine.functions(ctx, conn)
	if err != nil {
		return conn, nil, err
	}
	seen := map[string]port.Hash{}
	for _, f := range fns {
		if f.State == port.FunctionQuarantined && port.ItemKind(f.Name) == "" {
			seen[f.Name] = f.DefinitionHash
		}
	}
	if len(seen) > 0 {
		if err := c.Engine.accept(ctx, conn, seen); err != nil {
			return conn, nil, err
		}
	}
	return conn, fns, c.Engine.setLevel(ctx, conn, port.LevelCustom)
}

// AliasBase is the alias a listing name proposes: its last segment without an mcp- prefix, or the namespace's last label when that is only server or mcp.
func AliasBase(name string) string {
	ns, last := "", name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		ns, last = name[:i], name[i+1:]
	}
	if base := strings.TrimPrefix(last, "mcp-"); ns == "" || (base != "server" && base != "mcp" && base != "") {
		return base
	}
	return ns[strings.LastIndex(ns, ".")+1:]
}

// Install creates the private listing, opens it to the workspace, installs it, accepts its captured functions, applies the owner's choices and, with bindKeys, binds every key made so far, expired ones included so that a renewal follows it; with a key already made it also finishes the first-run guide.
func (c *Console) Install(ctx context.Context, m port.Manifest, choices map[string]Access, bindKeys bool) (string, error) {
	conn, fns, err := c.installListing(ctx, m, func(ref port.ListingRef) string { return AliasBase(ref.Name) })
	if err != nil {
		return conn, err
	}
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return conn, err
	}
	defer unlock()
	for _, f := range fns {
		a, ok := choices[f.Name]
		if kind, item := itemKinds[port.ItemKind(f.Name)]; !ok && item {
			a = Proposed(kind)
		} else if !ok {
			a = AccessAsk
		}
		if err := c.writeAccess(ctx, conn, f.Name, a); err != nil {
			return conn, fmt.Errorf("function %s: %w", f.Name, err)
		}
	}
	keys := c.State.Agents()
	if !bindKeys {
		keys = nil
	}
	for _, a := range keys {
		if err := c.bind(ctx, conn, a); err != nil {
			return conn, err
		}
	}
	if len(c.State.Agents()) > 0 {
		if err := c.State.DismissOnboarding(); err != nil {
			return conn, err
		}
	}
	return conn, nil
}

func (c *Console) openToWorkspace(ctx context.Context, target string) error {
	existing, err := c.Engine.catalogRules(ctx)
	if err != nil {
		return err
	}
	for _, want := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: c.Engine.Tenant, TargetKind: port.TargetListing, Target: target, Show: true, Install: true, Ceiling: port.CeilingFull},
		{SubjectKind: port.SubjectWorkspace, SubjectID: c.Engine.Workspace, TargetKind: port.TargetListing, Target: target, Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		for _, r := range existing {
			if r.Target == want.Target && r.SubjectKind == want.SubjectKind && r.SubjectID == want.SubjectID && r.TargetKind == want.TargetKind {
				want.ID, want.Audience = r.ID, r.Audience
			}
		}
		if err := c.Engine.putCatalogRule(ctx, want); err != nil {
			return err
		}
	}
	return nil
}

// Remove uninstalls a connector under its lock and forgets its new-key default and limits, its profiles in permission sets and the lock; with a key made, the first-run guide was finished and stays so.
func (c *Console) Remove(ctx context.Context, conn string) error {
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	defer c.writing.Delete(conn)
	if len(c.State.Agents()) > 0 {
		if err := c.State.DismissOnboarding(); err != nil {
			return err
		}
	}
	if err := c.Engine.removeConnector(ctx, conn); err != nil {
		return err
	}
	return c.State.ForgetConnector(conn)
}

func (c *Console) bind(ctx context.Context, conn string, a Agent) error {
	if err := c.Engine.bind(ctx, conn, a.Name, a.Owner); err != nil {
		return err
	}
	for _, sel := range agentSelectors {
		g := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: a.Name, Agent: port.AgentID(a.Name), Connector: port.ConnectorID(conn), Selector: sel, Mode: port.ModeAllow, ExpiresAt: a.ExpiresAt}
		if err := c.Engine.createGrant(ctx, g); err != nil {
			return err
		}
	}
	return nil
}

var agentSelectors = []string{port.SelectorAll, port.ItemInstructions + port.SelectorAll, port.ItemPrompt + port.SelectorAll, port.ItemResource + port.SelectorAll}

// RevokeAgent refuses every token minted so far for the agent, then under each connector's lock unbinds it and deletes its rows there, and last deletes the rows left; a key and a connector existed, so the first-run guide stays finished.
func (c *Console) RevokeAgent(ctx context.Context, name string) error {
	ok, err := c.State.RevokeAgent(name, time.Now())
	if err != nil {
		return err
	}
	if !ok {
		return noKey(name)
	}
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return err
	}
	for _, conn := range live {
		if err := c.State.DismissOnboarding(); err != nil {
			return err
		}
		if err := c.revokeOn(ctx, string(conn.ID), name); err != nil {
			return err
		}
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, name, "")
	if err != nil {
		return err
	}
	return c.Engine.deleteGrants(ctx, rows)
}

func (c *Console) revokeOn(ctx context.Context, conn, name string) error {
	unlock, err := c.lockConnector(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.Engine.unbind(ctx, conn, name); err != nil {
		return err
	}
	rows, err := c.Engine.grants(ctx, port.ScopeAgent, name, conn)
	if err != nil {
		return err
	}
	return c.Engine.deleteGrants(ctx, rows)
}

// AgentToken is a minted agent token with where to use it.
type AgentToken struct {
	Agent     string    `json:"agent"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	MCPURL    string    `json:"mcp_url"`
}

// BindAgent renews the agent where it is bound and keeps it off elsewhere; a new key follows every connector except those whose new keys start off, within each connector's new-key limits, and starts off on skills. A named permission set gives the key its profile on the set's connectors and is forgotten once the key is recorded. It records the agent, owned by the context's actor, until ttl from now, at most the console's longest, and with a connector installed it also finishes the first-run guide; a name another person's key has is refused before any engine write.
func (c *Console) BindAgent(ctx context.Context, name string, ttl time.Duration, slotName string) (Agent, error) {
	name = Slug(name)
	if name == "" {
		return Agent{}, invalid("the agent name has letters or digits")
	}
	slot, ok := c.State.Slot(slotName)
	if slotName != "" && !ok {
		return Agent{}, noSlot(slotName)
	}
	if ttl <= 0 || ttl > c.MaxAgentTTL {
		ttl = c.MaxAgentTTL
	}
	owner := actorOf(ctx)
	if owner == "" {
		return Agent{}, ErrNoActor
	}
	if old, ok := c.State.Agent(name); ok && old.Owner != owner {
		return Agent{}, ErrNameTaken
	}
	a := Agent{Name: name, Owner: owner, ExpiresAt: time.Now().Add(ttl).UTC().Truncate(time.Second)}
	live, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return Agent{}, err
	}
	known := slices.ContainsFunc(c.State.Agents(), func(k Agent) bool { return k.Name == name })
	for _, conn := range live {
		if p, ok := slot.Profiles[string(conn.ID)]; ok {
			err = c.claimOn(ctx, string(conn.ID), a, p)
		} else {
			err = c.join(ctx, conn, a, known)
		}
		if err != nil {
			return Agent{}, err
		}
	}
	if err := c.State.PutAgent(a); err != nil {
		return Agent{}, err
	}
	if err := c.State.DropSlot(slotName); err != nil {
		return Agent{}, err
	}
	if len(live) > 0 {
		if err := c.State.DismissOnboarding(); err != nil {
			return Agent{}, err
		}
	}
	return a, nil
}

func (c *Console) agentToken(ctx context.Context, name string, ttl time.Duration) (string, error) {
	owner := c.State.KeyOwner(name)
	return c.Engine.Mint(ctx, map[string]any{"sub": name, "tenant": c.Engine.Tenant, "workspace": c.Engine.Workspace, "agent": name, "session": name + "-" + randomID()[:8], "on_behalf_of": owner, "agent_owner": owner}, ttl)
}

// MintAgent binds the agent as BindAgent does, with the named permission set when one is named, and mints its token, living until the key expires.
func (c *Console) MintAgent(ctx context.Context, name string, ttl time.Duration, slot string) (AgentToken, error) {
	a, err := c.BindAgent(ctx, name, ttl, slot)
	if err != nil {
		return AgentToken{}, err
	}
	tok, err := c.agentToken(ctx, a.Name, time.Until(a.ExpiresAt))
	if err != nil {
		return AgentToken{}, err
	}
	return AgentToken{Agent: a.Name, Token: tok, ExpiresAt: a.ExpiresAt, MCPURL: c.MCPURL}, nil
}

// Approvals lists the calls waiting for the owner.
func (c *Console) Approvals(ctx context.Context) ([]port.Approval, error) {
	out, err := c.Engine.pendingApprovals(ctx, "", "")
	for i := range out {
		out[i].Arguments = nil
	}
	return out, err
}

// Decide approves that one waiting call, or denies it, as the context's actor.
func (c *Console) Decide(ctx context.Context, id string, approve bool) error {
	d := port.DecisionDeny
	if approve {
		d = port.DecisionApprove
	}
	_, err := c.Engine.decide(ctx, id, map[string]any{"decision": d, "channel": "console"})
	return err
}

// SetHeaderSecret stores the headers the connector's calls carry, sealed by the engine for the workspace.
func (c *Console) SetHeaderSecret(ctx context.Context, conn string, headers map[string]string) error {
	secret, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	in := map[string]any{"connector": conn, "owner_kind": port.OwnerWorkspace, "owner_id": c.Engine.Workspace, "kind": port.CredentialCustomHeaders, "secret": secret}
	return c.Engine.Call(ctx, http.MethodPost, "/v1/credentials", in, nil)
}

// SetOAuthClient registers the pre-registered OAuth client of an issuer with the console's loopback redirect.
func (c *Console) SetOAuthClient(ctx context.Context, issuer, clientID, secret string) error {
	in := map[string]any{"issuer": issuer, "client_id": clientID, "secret": secret, "redirect_url": c.OAuthRedirect}
	return c.Engine.Call(ctx, http.MethodPut, "/v1/issuers/client", in, nil)
}

// BeginOAuth asks the engine for the provider address of a workspace connection.
func (c *Console) BeginOAuth(ctx context.Context, conn string) (state, to string, err error) {
	var out struct {
		State string `json:"state"`
		URL   string `json:"url"`
	}
	err = c.Engine.Call(ctx, http.MethodPost, connectorPath(conn, "/credentials:connect"), map[string]any{"owner": port.OwnerWorkspace}, &out)
	return out.State, out.URL, err
}

// CompleteOAuth hands the code back to the engine, which exchanges it with the PKCE verifier and seals the tokens.
func (c *Console) CompleteOAuth(ctx context.Context, state, code string) error {
	return c.Engine.Call(ctx, http.MethodPost, "/v1/credentials:complete", map[string]any{"state": state, "code": code}, nil)
}

// OAuthIssuer returns the issuer the listing of a connector names for oauth2_user.
func (c *Console) OAuthIssuer(ctx context.Context, conn string) (string, error) {
	v, err := c.Engine.connector(ctx, conn)
	if err != nil {
		return "", err
	}
	m, err := c.Engine.manifest(ctx, v)
	if err != nil {
		return "", err
	}
	for _, a := range m.Auth {
		if a.Kind == port.CredentialOAuth2User && a.Issuer != "" {
			return a.Issuer, nil
		}
	}
	return "", invalid("the listing of this connector declares no OAuth issuer")
}
