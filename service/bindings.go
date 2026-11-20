package service

import (
	"context"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/catalog"
	"github.com/versatile-sh/connector-engine/core/approval"
	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

func (s *Service) removeBinding(tx port.Tx, b port.Binding, by string) error {
	if err := tx.DeleteBinding(b.Connector, b.Agent); err != nil {
		return err
	}
	return s.record(tx, b.Tenant, by, string(b.Connector)+"/"+string(b.Agent), evBindingChanged,
		catalog.BindingChanged{Agent: string(b.Agent), Connector: string(b.Connector), CredentialPolicy: string(b.CredentialPolicy), Deleted: true})
}

func defaultPolicy(m port.Manifest) port.CredentialPolicy {
	for _, a := range m.Auth {
		if len(a.OwnerKindsAllowed) > 0 && !slices.Contains(a.OwnerKindsAllowed, port.OwnerUser) {
			return port.CredentialPerAgent
		}
	}
	return port.CredentialPerUser
}

// PutBinding gives an agent a connector; a binding for an agent whose owner may not see the listing is not found.
func (s *Service) PutBinding(ctx context.Context, p port.Principal, b port.Binding) (port.Binding, error) {
	if err := port.CheckIDs("agent", string(b.Agent), "agent_owner", string(b.AgentOwner)); err != nil {
		return b, err
	}
	c, err := s.connectorOf(ctx, p, b.Connector, authz.BindingWrite)
	if err != nil {
		return b, err
	}
	b.Tenant, b.Workspace, b.OwnerRemoved = c.Tenant, c.Workspace, false
	var people []port.PersonID
	if b.AgentOwner != "" {
		people = append(people, b.AgentOwner)
	}
	g, err := s.groups(ctx, c.Tenant, people...)
	if err != nil {
		return b, err
	}
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "PutBinding", func(tx port.Tx) error {
		v, err := s.view(tx, c.Tenant, c.Workspace, g)
		if err != nil {
			return err
		}
		if !v.Resolve(c.Workspace, people, c.Listing.Listing).Visible {
			return port.Errorf(port.CodeNotFound, "connector %s not found", b.Connector)
		}
		if b.CredentialPolicy == "" {
			lv, err := tx.GetVersion(c.Listing)
			if err != nil {
				return err
			}
			b.CredentialPolicy = defaultPolicy(lv.Manifest)
		}
		if err := tx.PutBinding(b); err != nil {
			return err
		}
		return s.record(tx, c.Tenant, actor(p), string(b.Connector)+"/"+string(b.Agent), evBindingChanged,
			catalog.BindingChanged{Agent: string(b.Agent), Connector: string(b.Connector), CredentialPolicy: string(b.CredentialPolicy)})
	})
	return b, err
}

// DeleteBinding removes a binding with the agent's rows on the connector and cancels the agent's pending approvals there.
func (s *Service) DeleteBinding(ctx context.Context, p port.Principal, connector port.ConnectorID, agent port.AgentID) error {
	c, err := s.connectorOf(ctx, p, connector, authz.BindingWrite)
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: c.Tenant}, "DeleteBinding", func(tx port.Tx) error {
		b, err := tx.GetBinding(connector, agent)
		if err != nil {
			return err
		}
		return s.unbind(tx, b, actor(p))
	})
}

func (s *Service) unbind(tx port.Tx, b port.Binding, by string) error {
	if err := s.removeBinding(tx, b, by); err != nil {
		return err
	}
	rows, err := tx.ListGrants(port.GrantFilter{Connector: b.Connector, Agent: b.Agent})
	if err != nil {
		return err
	}
	agentRows, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeAgent, ScopeID: string(b.Agent), Connector: b.Connector})
	if err != nil {
		return err
	}
	for _, g := range append(rows, agentRows...) {
		if err := s.revokeGrant(tx, g, by, "binding_deleted"); err != nil && !port.IsCode(err, port.CodeNotFound) {
			return err
		}
	}
	_, err = s.cancelPending(tx, port.ApprovalFilter{Connector: b.Connector, Agent: b.Agent}, port.ReasonNoBinding, nil)
	return err
}

// ListBindings returns the bindings of a connector.
func (s *Service) ListBindings(ctx context.Context, p port.Principal, f port.BindingFilter) ([]port.Binding, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Binding
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListBindings(f)
		return err
	})
	return out, err
}

// SetAgentLevel replaces the level rows of an agent on a connector and returns the rows it wrote.
func (s *Service) SetAgentLevel(ctx context.Context, p port.Principal, connector port.ConnectorID, agent port.AgentID, level port.Level) ([]port.Grant, error) {
	c, err := s.connectorOf(ctx, p, connector, authz.GrantAgent)
	if err != nil {
		return nil, err
	}
	var written []port.Grant
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "SetAgentLevel", func(tx port.Tx) error {
		layer, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeAgent, ScopeID: string(agent), Connector: connector})
		if err != nil {
			return err
		}
		base := port.Grant{Tenant: c.Tenant, ScopeKind: port.ScopeAgent, ScopeID: string(agent), Connector: connector, CreatedBy: actor(p), Origin: port.OriginLevel}
		return s.applyLevel(tx, layer, base, level, actor(p), &written)
	})
	return written, err
}

// GrantResult is a written row, the functions the destructive cap applies to today and the row it replaced.
type GrantResult struct {
	Grant    port.Grant  `json:"grant"`
	Capped   []string    `json:"capped,omitempty"`
	Replaced *port.Grant `json:"replaced,omitempty"`
}

func (s *Service) grantAuth(ctx context.Context, p port.Principal, g port.Grant) error {
	tenant := string(p.Claims.Tenant)
	switch g.ScopeKind {
	case port.ScopeTenant:
		return s.authorize(ctx, p, authz.GrantTenant, authz.Tenant(tenant))
	case port.ScopeUser:
		return s.authorize(ctx, p, authz.GrantUser, authz.Person(tenant, g.ScopeID))
	}
	c, err := s.connectorOf(ctx, p, g.Connector, authz.GrantWorkspace)
	if err == nil || g.ScopeKind != port.ScopeAgent || !port.IsCode(err, port.CodePermissionDenied) {
		return err
	}
	return s.authorize(ctx, p, authz.GrantAgent, authz.Agent(tenant, string(c.Workspace), g.ScopeID))
}

// CreateGrant writes a settings row under the rules G1 to G10; a session row is never written here.
func (s *Service) CreateGrant(ctx context.Context, p port.Principal, g port.Grant) (GrantResult, error) {
	out := GrantResult{Grant: g}
	if g.ScopeKind == port.ScopeSession {
		return out, port.Errorf(port.CodeRowRefused, "a session row is written by an approval only (G5)")
	}
	g.Tenant, g.Origin, g.Uses = p.Claims.Tenant, port.OriginSettings, 0
	if g.ScopeKind == port.ScopeTenant && g.ScopeID == "" {
		g.ScopeID = string(g.Tenant)
	}
	if err := port.CheckIDs("scope", g.ScopeID, "agent", string(g.Agent)); err != nil {
		return out, err
	}
	if err := s.grantAuth(ctx, p, g); err != nil {
		return out, err
	}
	if err := grant.Validate(g, grant.Policy{Now: s.now(), AlwaysMax: s.cfg.AlwaysMax}); err != nil {
		return out, err
	}
	err := s.write(ctx, port.Scope{Tenant: g.Tenant}, "CreateGrant", func(tx port.Tx) error {
		rows, err := tx.ListGrants(port.GrantFilter{ScopeKind: g.ScopeKind, ScopeID: g.ScopeID})
		if err != nil {
			return err
		}
		for _, old := range rows {
			if old.Key() == g.Key() {
				old := old
				out.Replaced = &old
				if err := s.revokeGrant(tx, old, actor(p), "replaced"); err != nil {
					return err
				}
			}
		}
		g = s.newGrant(g)
		g.CreatedBy = actor(p)
		if err := s.putGrant(tx, g, actor(p)); err != nil {
			return err
		}
		out.Grant = g
		if g.Connector == "" {
			return nil
		}
		fns, err := tx.ListFunctions(g.Connector)
		out.Capped = grant.Capped(g, fns)
		return err
	})
	return out, err
}

// DeleteGrant revokes a row and cancels the pending approvals it covered.
func (s *Service) DeleteGrant(ctx context.Context, p port.Principal, id port.GrantID) error {
	g, err := s.grantOf(ctx, p, id)
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "DeleteGrant", func(tx port.Tx) error {
		if err := s.revokeGrant(tx, g, actor(p), "revoked"); err != nil {
			return err
		}
		if g.Connector == "" {
			return nil
		}
		_, err := s.cancelPending(tx, port.ApprovalFilter{Connector: g.Connector}, port.CodeGrantRevoked, func(a port.Approval) bool {
			f, err := tx.GetFunction(a.Connector, a.Function)
			return err == nil && grant.Covers(g.Selector, f)
		})
		return err
	})
}

func (s *Service) grantOf(ctx context.Context, p port.Principal, id port.GrantID) (port.Grant, error) {
	var g port.Grant
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		g, err = tx.GetGrant(id)
		return err
	})
	if err != nil {
		return g, err
	}
	return g, s.grantAuth(ctx, p, g)
}

// ConfirmGrant moves a stale fn: row to the function's current hash.
func (s *Service) ConfirmGrant(ctx context.Context, p port.Principal, id port.GrantID) (port.Grant, error) {
	g, err := s.grantOf(ctx, p, id)
	if err != nil {
		return g, err
	}
	err = s.write(ctx, port.Scope{Tenant: g.Tenant}, "ConfirmGrant", func(tx port.Tx) error {
		name := strings.TrimPrefix(g.Selector, port.SelectorFunction)
		f, err := tx.GetFunction(g.Connector, name)
		if err != nil {
			return err
		}
		old := g.DefinitionHash
		if g, err = grant.Confirm(g, f); err != nil {
			return err
		}
		if err := tx.PutGrant(g); err != nil {
			return err
		}
		return s.record(tx, g.Tenant, actor(p), string(g.ID), evGrantConfirmed, catalog.GrantConfirmed{Grant: string(g.ID), OldHash: string(old), NewHash: string(g.DefinitionHash), By: actor(p)})
	})
	return g, err
}

// ListGrants returns the rows of the caller's tenant that match the filter.
func (s *Service) ListGrants(ctx context.Context, p port.Principal, f port.GrantFilter) ([]port.Grant, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Grant
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListGrants(f)
		return err
	})
	return out, err
}

// ListPauses returns the denial-brake rows of the caller's tenant.
func (s *Service) ListPauses(ctx context.Context, p port.Principal) ([]port.Pause, error) {
	if err := s.authorize(ctx, p, authz.ApprovalRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Pause
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListPauses()
		return err
	})
	return out, err
}

// ClearPause ends a pause and resets the consecutive count.
func (s *Service) ClearPause(ctx context.Context, p port.Principal, id port.PauseID) error {
	var pause port.Pause
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		all, err := tx.ListPauses()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(all, func(x port.Pause) bool { return x.ID == id })
		if i < 0 {
			return port.Errorf(port.CodeNotFound, "pause %s not found", id)
		}
		pause = all[i]
		return nil
	}); err != nil {
		return err
	}
	if _, err := s.connectorOf(ctx, p, pause.Connector, authz.PauseClear); err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: pause.Tenant}, "ClearPause", func(tx port.Tx) error {
		if err := tx.PutPause(approval.Clear(pause)); err != nil {
			return err
		}
		return s.record(tx, pause.Tenant, actor(p), string(id), evBrakeCleared,
			catalog.BrakeCleared{Agent: string(pause.Agent), Session: string(pause.Session), Connector: string(pause.Connector), By: actor(p)})
	})
}

// EndSession ends a session: its rows go, its pending approvals are cancelled and a tombstone refuses its token.
func (s *Service) EndSession(ctx context.Context, p port.Principal, session port.SessionID) error {
	if err := s.authorize(ctx, p, authz.SessionEnd, s.tenantResource(p)); err != nil {
		return err
	}
	tenant := p.Claims.Tenant
	return s.write(ctx, port.Scope{Tenant: tenant}, "EndSession", func(tx port.Tx) error {
		if err := tx.PutTombstone(port.Tombstone{Tenant: tenant, Kind: port.TombstoneSession, Subject: string(session), At: s.now(), ExpiresAt: s.now().Add(s.cfg.AgentTokenMax)}); err != nil {
			return err
		}
		rows, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeSession, ScopeID: string(session)})
		if err != nil {
			return err
		}
		for _, g := range rows {
			if err := s.revokeGrant(tx, g, actor(p), "session_ended"); err != nil {
				return err
			}
		}
		pauses, err := tx.ListPauses()
		if err != nil {
			return err
		}
		for _, pz := range pauses {
			if pz.Session == session {
				if err := tx.DeletePause(pz.ID); err != nil {
					return err
				}
			}
		}
		_, err = s.cancelPending(tx, port.ApprovalFilter{Session: session}, port.ReasonSessionEnded, nil)
		return err
	})
}

// RemoveAgent revokes the bindings and rows of an agent, destroys its credentials and cancels its pending approvals.
func (s *Service) RemoveAgent(ctx context.Context, p port.Principal, agent port.AgentID) error {
	if err := s.authorize(ctx, p, authz.AgentRemove, s.tenantResource(p)); err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "RemoveAgent", func(tx port.Tx) error {
		binds, err := tx.ListBindings(port.BindingFilter{Agent: agent})
		if err != nil {
			return err
		}
		for _, b := range binds {
			if err := s.unbind(tx, b, actor(p)); err != nil {
				return err
			}
		}
		creds, err := tx.ListCredentials(port.CredentialFilter{OwnerKind: port.OwnerAgent, OwnerID: string(agent)})
		if err != nil {
			return err
		}
		for _, c := range creds {
			if err := s.revokeCredentialRow(tx, c, actor(p)); err != nil {
				return err
			}
		}
		_, err = s.cancelPending(tx, port.ApprovalFilter{Agent: agent}, port.ReasonNoBinding, nil)
		return err
	})
}

// RemovePerson revokes a person's rows, roles and credentials in a workspace, or everywhere with the groups when empty, and tombstones the person.
func (s *Service) RemovePerson(ctx context.Context, p port.Principal, person port.PersonID, workspace port.WorkspaceID) error {
	if err := s.authorize(ctx, p, authz.PersonRemove, s.tenantResource(p)); err != nil {
		return err
	}
	tenant := p.Claims.Tenant
	return s.write(ctx, port.Scope{Tenant: tenant}, "RemovePerson", func(tx port.Tx) error {
		if err := tx.PutTombstone(port.Tombstone{Tenant: tenant, Kind: port.TombstonePerson, Subject: string(person), Workspace: workspace, At: s.now(), ExpiresAt: s.now().Add(s.cfg.AgentTokenMax)}); err != nil {
			return err
		}
		conns, err := tx.ListConnectors(port.ConnectorFilter{Workspace: workspace})
		if err != nil {
			return err
		}
		inScope := map[port.ConnectorID]bool{}
		for _, c := range conns {
			inScope[c.ID] = true
		}
		if err := s.forgetPerson(tx, person, inScope, actor(p)); err != nil {
			return err
		}
		if err := dropRoles(tx, tenant, person, workspace); err != nil {
			return err
		}
		_, err = s.cancelPending(tx, port.ApprovalFilter{}, port.ReasonPersonRemoved, func(a port.Approval) bool {
			return inScope[a.Connector] && (a.OnBehalfOf == person || slices.Equal(a.Approvers, []string{string(person)}))
		})
		return err
	})
}

func dropRoles(tx port.Tx, tenant port.TenantID, person port.PersonID, workspace port.WorkspaceID) error {
	roles, err := tx.ListRoles(port.RoleFilter{Person: person})
	if err != nil {
		return err
	}
	for _, r := range roles {
		if r.Tenant == tenant && (workspace == "" || authz.Covers(authz.Workspace(string(tenant), string(workspace)), r.Scope)) {
			if err := tx.DeleteRole(r); err != nil {
				return err
			}
		}
	}
	if workspace != "" {
		return nil
	}
	members, err := tx.ListMembers(port.MemberFilter{Person: person})
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := tx.DeleteMember(m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) forgetPerson(tx port.Tx, person port.PersonID, inScope map[port.ConnectorID]bool, by string) error {
	grants, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeUser, ScopeID: string(person)})
	if err != nil {
		return err
	}
	for _, g := range grants {
		if inScope[g.Connector] {
			if err := s.revokeGrant(tx, g, by, "person_removed"); err != nil {
				return err
			}
		}
	}
	creds, err := tx.ListCredentials(port.CredentialFilter{OwnerKind: port.OwnerUser, OwnerID: string(person)})
	if err != nil {
		return err
	}
	for _, c := range creds {
		if inScope[c.Connector] {
			if err := s.revokeCredentialRow(tx, c, by); err != nil {
				return err
			}
		}
	}
	binds, err := tx.ListBindings(port.BindingFilter{})
	if err != nil {
		return err
	}
	for _, b := range binds {
		if !inScope[b.Connector] {
			continue
		}
		b.Approvers = slices.DeleteFunc(slices.Clone(b.Approvers), func(a string) bool { return a == string(person) })
		b.OwnerRemoved = b.OwnerRemoved || b.AgentOwner == person
		if err := tx.PutBinding(b); err != nil {
			return err
		}
	}
	return nil
}

// RestorePerson deletes the tombstone of a person and nothing else.
func (s *Service) RestorePerson(ctx context.Context, p port.Principal, person port.PersonID, workspace port.WorkspaceID) error {
	if err := s.authorize(ctx, p, authz.PersonRemove, s.tenantResource(p)); err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "RestorePerson", func(tx port.Tx) error {
		return tx.DeleteTombstone(port.TombstonePerson, string(person), workspace)
	})
}
