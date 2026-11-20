package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
)

type items[T any] struct {
	Items []T `json:"items"`
}

type connectorView struct {
	Connector port.Connector  `json:"connector"`
	Functions []port.Function `json:"functions,omitempty"`
	Blocked   string          `json:"blocked"`
}

// FunctionView is one function of a connector as the console shows it: its class and the owner's choice read back from the engine's grants.
type FunctionView struct {
	Name        string     `json:"name"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Class       port.Class `json:"class"`
	State       string     `json:"state"`
	Access      Access     `json:"access"`
}

// ConnectorSummary is one installed connector with its listing's presentation and its functions.
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
	Auth        []string       `json:"auth,omitempty"`
	Credentials []string       `json:"credentials,omitempty"`
	Functions   []FunctionView `json:"functions"`
}

func q(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return "?" + v.Encode()
}

func esc(s string) string { return url.PathEscape(s) }

// IsCode reports whether err is an engine refusal with this code.
func IsCode(err error, code string) bool {
	var ee *EngineError
	return errors.As(err, &ee) && ee.Code == code
}

func (c *Console) workspaceGrants(ctx context.Context, conn string) ([]port.Grant, error) {
	var g items[port.Grant]
	err := c.Engine.Call(ctx, http.MethodGet, "/v1/grants"+q("scope_kind", "workspace", "scope_id", c.Engine.Workspace, "connector", conn), nil, &g)
	return g.Items, err
}

func accessOf(f port.Function, rows []port.Grant) Access {
	for _, g := range rows {
		if g.Selector != port.SelectorFunction+f.Name {
			continue
		}
		switch g.Mode {
		case port.ModeAllow:
			if f.Class == port.ClassRead {
				return AccessRead
			}
			return AccessWrite
		case port.ModeAsk, port.ModeAskOnce:
			return AccessAsk
		}
	}
	return AccessOff
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
	var list items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &list); err != nil {
		return nil, err
	}
	out := make([]ConnectorSummary, 0, len(list.Items))
	for _, v := range list.Items {
		conn := v.Connector
		if !conn.RemovedAt.IsZero() {
			continue
		}
		s := ConnectorSummary{ID: string(conn.ID), Alias: conn.Alias, Enabled: conn.Enabled, Blocked: v.Blocked, Listing: conn.Listing.Listing.Name, Version: conn.Listing.Version, Title: conn.Alias}
		var lv port.ListingVersion
		if err := c.Engine.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", c.Engine.Workspace, "owner", string(conn.Listing.Listing.Owner), "name", conn.Listing.Listing.Name, "version", conn.Listing.Version), nil, &lv); err == nil {
			m := lv.Manifest
			s.Title, s.Description, s.Icon, s.Publisher = firstOf(m.Title, m.Name), m.Description, m.Icon, m.Publisher.Name
			if m.Remote != nil {
				s.RemoteURL = m.Remote.URL
			}
			for _, a := range m.Auth {
				s.Auth = append(s.Auth, string(a.Kind))
			}
		}
		var fns items[port.Function]
		if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(string(conn.ID))+"/functions", nil, &fns); err != nil {
			return nil, err
		}
		rows, err := c.workspaceGrants(ctx, string(conn.ID))
		if err != nil {
			return nil, err
		}
		for _, f := range fns.Items {
			if f.State == port.FunctionRemoved {
				continue
			}
			title, desc := describe(f.Definition)
			s.Functions = append(s.Functions, FunctionView{Name: f.Name, Title: title, Description: desc, Class: f.Class, State: string(f.State), Access: accessOf(f, rows)})
		}
		var creds items[port.Credential]
		if err := c.Engine.Call(ctx, http.MethodGet, "/v1/credentials"+q("connector", string(conn.ID)), nil, &creds); err == nil {
			for _, cr := range creds.Items {
				s.Credentials = append(s.Credentials, string(cr.Kind)+":"+string(cr.Status))
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Console) function(ctx context.Context, conn, name string) (port.Function, error) {
	var fns items[port.Function]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(conn)+"/functions", nil, &fns); err != nil {
		return port.Function{}, err
	}
	for _, f := range fns.Items {
		if f.Name == name {
			return f, nil
		}
	}
	return port.Function{}, &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "function " + name + " not found"}
}

// SetAccess writes the owner's choice for one function: it accepts a quarantined definition the owner now sees, classes the function, then replaces its workspace row (none for off).
func (c *Console) SetAccess(ctx context.Context, conn, name string, a Access) error {
	if !a.Valid() {
		return fmt.Errorf("access %q is not off, read, write or ask", a)
	}
	f, err := c.function(ctx, conn, name)
	if err != nil {
		return err
	}
	if f.State == port.FunctionQuarantined && a != AccessOff {
		if err := c.Engine.Call(ctx, http.MethodPost, "/v1/connectors/"+esc(conn)+"/functions/"+esc(name)+":accept", map[string]any{"seen": f.DefinitionHash}, nil); err != nil {
			return err
		}
		if f, err = c.function(ctx, conn, name); err != nil {
			return err
		}
	}
	class := f.Class
	switch {
	case a == AccessRead:
		class = port.ClassRead
	case a == AccessWrite:
		class = port.ClassWrite
	case class == port.ClassNone:
		class = port.ClassWrite
	}
	if class != f.Class {
		if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/functions/"+esc(name)+"/class", map[string]any{"class": class}, &f); err != nil {
			return err
		}
	}
	rows, err := c.workspaceGrants(ctx, conn)
	if err != nil {
		return err
	}
	for _, g := range rows {
		if g.Selector == port.SelectorFunction+name {
			if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/grants/"+esc(string(g.ID)), nil, nil); err != nil {
				return err
			}
		}
	}
	mode := map[Access]port.Mode{AccessRead: port.ModeAllow, AccessWrite: port.ModeAllow, AccessAsk: port.ModeAsk}[a]
	if mode == "" {
		return nil
	}
	g := port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: c.Engine.Workspace, Connector: port.ConnectorID(conn), Selector: port.SelectorFunction + name, Mode: mode, DefinitionHash: f.DefinitionHash}
	return c.Engine.Call(ctx, http.MethodPost, "/v1/grants", g, nil)
}

func (c *Console) createListing(ctx context.Context, m port.Manifest) (port.ListingVersion, error) {
	var lv port.ListingVersion
	err := c.Engine.Call(ctx, http.MethodPost, "/v1/listings", map[string]any{"manifest": m}, &lv)
	if IsCode(err, "conflict") {
		err = c.Engine.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", c.Engine.Workspace, "owner", c.Engine.Tenant, "name", m.Name, "version", m.Version), nil, &lv)
	}
	return lv, err
}

func (c *Console) freeAlias(ctx context.Context, base string) (string, error) {
	var list items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &list); err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, v := range list.Items {
		if v.Connector.RemovedAt.IsZero() {
			taken[v.Connector.Alias] = true
		}
	}
	base = Slug(base)
	if len(base) > 24 {
		base = strings.Trim(base[:24], "-")
	}
	if base == "" {
		base = "mcp"
	}
	alias := base
	for i := 2; taken[alias]; i++ {
		alias = fmt.Sprintf("%s-%d", base, i)
	}
	return alias, nil
}

// Install creates the private listing, opens it to the workspace, installs it, accepts its captured functions, applies the owner's choices and binds every connected agent.
func (c *Console) Install(ctx context.Context, m port.Manifest, choices map[string]Access) (string, error) {
	lv, err := c.createListing(ctx, m)
	if err != nil {
		return "", err
	}
	ref := lv.Ref.Listing
	target := string(ref.Owner) + "/" + ref.Name
	if err := c.openToWorkspace(ctx, target); err != nil {
		return "", err
	}
	alias, err := c.freeAlias(ctx, strings.TrimPrefix(ref.Name[strings.LastIndex(ref.Name, "/")+1:], "mcp-"))
	if err != nil {
		return "", err
	}
	var view connectorView
	if err := c.Engine.Call(ctx, http.MethodPost, "/v1/connectors", map[string]any{"workspace": c.Engine.Workspace, "alias": alias, "listing": ref, "version": lv.Ref.Version}, &view); err != nil {
		return "", err
	}
	conn := string(view.Connector.ID)
	var fns items[port.Function]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(conn)+"/functions", nil, &fns); err != nil {
		return conn, err
	}
	seen := map[string]port.Hash{}
	for _, f := range fns.Items {
		if f.State == port.FunctionQuarantined {
			seen[f.Name] = f.DefinitionHash
		}
	}
	if len(seen) > 0 {
		if err := c.Engine.Call(ctx, http.MethodPost, "/v1/connectors/"+esc(conn)+"/functions:accept", map[string]any{"seen": seen}, nil); err != nil {
			return conn, err
		}
	}
	if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/level", map[string]any{"level": port.LevelCustom}, nil); err != nil {
		return conn, err
	}
	for _, f := range fns.Items {
		a, ok := choices[f.Name]
		if !ok {
			a = AccessAsk
		}
		if err := c.SetAccess(ctx, conn, f.Name, a); err != nil {
			return conn, fmt.Errorf("function %s: %w", f.Name, err)
		}
	}
	for _, a := range c.State.Agents() {
		if a.ExpiresAt.After(time.Now()) {
			if err := c.bind(ctx, conn, a); err != nil {
				return conn, err
			}
		}
	}
	return conn, nil
}

func (c *Console) openToWorkspace(ctx context.Context, target string) error {
	var existing items[port.CatalogRule]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/catalog/rules", nil, &existing); err != nil {
		return err
	}
	for _, want := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: c.Engine.Tenant, TargetKind: port.TargetListing, Target: target, Show: true, Install: true, Ceiling: port.CeilingFull},
		{SubjectKind: port.SubjectWorkspace, SubjectID: c.Engine.Workspace, TargetKind: port.TargetListing, Target: target, Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		for _, r := range existing.Items {
			if r.Target == want.Target && r.SubjectKind == want.SubjectKind && r.SubjectID == want.SubjectID && r.TargetKind == want.TargetKind {
				want.ID, want.Audience = r.ID, r.Audience
			}
		}
		if err := c.Engine.Call(ctx, http.MethodPut, "/v1/catalog/rules", want, nil); err != nil {
			return err
		}
	}
	return nil
}

// Remove uninstalls a connector.
func (c *Console) Remove(ctx context.Context, conn string) error {
	return c.Engine.Call(ctx, http.MethodDelete, "/v1/connectors/"+esc(conn), nil, nil)
}

func (c *Console) bind(ctx context.Context, conn string, a Agent) error {
	if err := c.Engine.Call(ctx, http.MethodPut, "/v1/connectors/"+esc(conn)+"/bindings/"+esc(a.Name), map[string]any{"credential_policy": port.CredentialShared, "ask_requester": true}, nil); err != nil {
		return err
	}
	var rows items[port.Grant]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/grants"+q("scope_kind", "agent", "scope_id", a.Name, "connector", conn), nil, &rows); err != nil {
		return err
	}
	for _, g := range rows.Items {
		if g.Selector == port.SelectorAll {
			if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/grants/"+esc(string(g.ID)), nil, nil); err != nil {
				return err
			}
		}
	}
	g := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: a.Name, Agent: port.AgentID(a.Name), Connector: port.ConnectorID(conn), Selector: port.SelectorAll, Mode: port.ModeAllow, ExpiresAt: a.ExpiresAt}
	return c.Engine.Call(ctx, http.MethodPost, "/v1/grants", g, nil)
}

// RevokeAgent refuses every token minted so far for the agent, then unbinds it from every connector and deletes the grants scoped to it.
func (c *Console) RevokeAgent(ctx context.Context, name string) error {
	ok, err := c.State.RevokeAgent(name, time.Now())
	if err != nil {
		return err
	}
	if !ok {
		return &EngineError{Status: http.StatusNotFound, Code: "not_found", Detail: "no access key is named " + name}
	}
	var list items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &list); err != nil {
		return err
	}
	for _, v := range list.Items {
		if !v.Connector.RemovedAt.IsZero() {
			continue
		}
		if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/connectors/"+esc(string(v.Connector.ID))+"/bindings/"+esc(name), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	var rows items[port.Grant]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/grants"+q("scope_kind", string(port.ScopeAgent), "scope_id", name), nil, &rows); err != nil {
		return err
	}
	for _, g := range rows.Items {
		if err := c.Engine.Call(ctx, http.MethodDelete, "/v1/grants/"+esc(string(g.ID)), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	return nil
}

// AgentToken is a minted agent token with where to use it.
type AgentToken struct {
	Agent     string    `json:"agent"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	MCPURL    string    `json:"mcp_url"`
}

// BindAgent binds the agent to every connector and records it, letting the workspace rows decide for it until ttl from now, at most the console's longest.
func (c *Console) BindAgent(ctx context.Context, name string, ttl time.Duration) (Agent, error) {
	name = Slug(name)
	if name == "" {
		return Agent{}, fmt.Errorf("the agent name has letters or digits")
	}
	if ttl <= 0 || ttl > c.MaxAgentTTL {
		ttl = c.MaxAgentTTL
	}
	a := Agent{Name: name, ExpiresAt: time.Now().Add(ttl).UTC().Truncate(time.Second)}
	var list items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &list); err != nil {
		return Agent{}, err
	}
	for _, v := range list.Items {
		if v.Connector.RemovedAt.IsZero() {
			if err := c.bind(ctx, string(v.Connector.ID), a); err != nil {
				return Agent{}, err
			}
		}
	}
	return a, c.State.PutAgent(a)
}

func (c *Console) agentToken(ctx context.Context, name string, ttl time.Duration) (string, error) {
	return c.Engine.Mint(ctx, map[string]any{"sub": name, "tenant": c.Engine.Tenant, "workspace": c.Engine.Workspace, "agent": name, "session": name + "-" + randomID()[:8], "on_behalf_of": c.Engine.Person}, ttl)
}

// MintAgent binds the agent to every connector, lets the workspace rows decide for it until the token expires, and mints its token.
func (c *Console) MintAgent(ctx context.Context, name string, ttl time.Duration) (AgentToken, error) {
	a, err := c.BindAgent(ctx, name, ttl)
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
	var out items[port.Approval]
	err := c.Engine.Call(ctx, http.MethodGet, "/v1/approvals"+q("state", string(port.ApprovalPending)), nil, &out)
	for i := range out.Items {
		out.Items[i].Arguments = nil
	}
	return out.Items, err
}

// Decide approves or denies one waiting call as the owner.
func (c *Console) Decide(ctx context.Context, id string, approve bool) error {
	d := port.DecisionDeny
	if approve {
		d = port.DecisionApprove
	}
	return c.Engine.Call(ctx, http.MethodPost, "/v1/approvals/"+esc(id)+":decide", map[string]any{"decision": d, "channel": "console"}, nil)
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
	err = c.Engine.Call(ctx, http.MethodPost, "/v1/connectors/"+esc(conn)+"/credentials:connect", map[string]any{"owner": port.OwnerWorkspace}, &out)
	return out.State, out.URL, err
}

// CompleteOAuth hands the code back to the engine, which exchanges it with the PKCE verifier and seals the tokens.
func (c *Console) CompleteOAuth(ctx context.Context, state, code string) error {
	return c.Engine.Call(ctx, http.MethodPost, "/v1/credentials:complete", map[string]any{"state": state, "code": code}, nil)
}

// OAuthIssuer returns the issuer the listing of a connector names for oauth2_user.
func (c *Console) OAuthIssuer(ctx context.Context, conn string) (string, error) {
	var v connectorView
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors/"+esc(conn), nil, &v); err != nil {
		return "", err
	}
	ref := v.Connector.Listing
	var lv port.ListingVersion
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", c.Engine.Workspace, "owner", string(ref.Listing.Owner), "name", ref.Listing.Name, "version", ref.Version), nil, &lv); err != nil {
		return "", err
	}
	for _, a := range lv.Manifest.Auth {
		if a.Kind == port.CredentialOAuth2User && a.Issuer != "" {
			return a.Issuer, nil
		}
	}
	return "", fmt.Errorf("the listing of this connector declares no OAuth issuer")
}
