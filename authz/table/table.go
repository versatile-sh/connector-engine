// Package table is the built-in port.Authorizer: the table evaluator over the store, and the role table.
package table

import (
	"context"
	"slices"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/authz/facts"
	"github.com/versatile-sh/connector-engine/core/evaluate"
	"github.com/versatile-sh/connector-engine/port"
)

// Authorizer evaluates calls with core/evaluate over the store's rows and answers role questions from the role rows of the store.
type Authorizer struct {
	store    port.Store
	clock    port.Clock
	settings Settings
}

// New returns the table authorizer.
func New(store port.Store, clock port.Clock, settings Settings) *Authorizer {
	return &Authorizer{store: store, clock: clock, settings: settings}
}

var (
	_ port.Authorizer    = (*Authorizer)(nil)
	_ port.BatchSearcher = (*Authorizer)(nil)
)

func subjectOf(q port.Query) port.Subject {
	return port.Subject{Tenant: q.Tenant, Workspace: q.Workspace, Agent: q.Agent, AgentOwner: q.AgentOwner, OnBehalfOf: q.OnBehalfOf, Session: q.Session, Caller: q.Caller}
}

func (a *Authorizer) facts(ctx context.Context, s port.Subject, connector port.ConnectorID, fn func(evaluate.Facts) error) error {
	return a.store.Read(ctx, port.Scope{Tenant: s.Tenant}, func(tx port.Tx) error {
		groups, err := peopleGroups(tx, s, map[port.PersonID][]port.GroupID{})
		if err != nil {
			return err
		}
		f, err := facts.LoadFacts(tx, a.clock.Now(), s, connector, groups, a.settings)
		if err != nil {
			return err
		}
		return fn(f)
	})
}

func peopleGroups(tx port.Tx, s port.Subject, known map[port.PersonID][]port.GroupID) (map[port.PersonID][]port.GroupID, error) {
	groups := map[port.PersonID][]port.GroupID{}
	for _, p := range evaluate.People(s) {
		g, ok := known[p]
		if !ok {
			var err error
			if g, err = groupsOf(tx, p); err != nil {
				return nil, err
			}
			known[p] = g
		}
		groups[p] = g
	}
	return groups, nil
}

// SearchBatch answers Search for each query in one read of the tenant, loading the grant table and each person's groups once.
func (a *Authorizer) SearchBatch(ctx context.Context, tenant port.TenantID, qs []port.SearchQuery) ([][]port.FunctionDecision, error) {
	out := make([][]port.FunctionDecision, len(qs))
	err := a.store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		grants, err := tx.ListGrants(port.GrantFilter{})
		if err != nil {
			return err
		}
		known := map[port.PersonID][]port.GroupID{}
		now := a.clock.Now()
		for i, q := range qs {
			groups, err := peopleGroups(tx, q.Subject, known)
			if err != nil {
				return err
			}
			f, err := facts.LoadFactsWith(tx, now, q.Subject, q.Connector, groups, a.settings, grants)
			if err != nil {
				return err
			}
			out[i] = evaluate.Search(f, q.Subject, now)
		}
		return nil
	})
	return out, err
}

// Evaluate runs steps 1 to 14 for one function; this Authorizer has no external decision point for step 15.
func (a *Authorizer) Evaluate(ctx context.Context, q port.Query) (port.AccessDecision, error) {
	var d evaluate.Decision
	err := a.facts(ctx, subjectOf(q), q.Connector, func(f evaluate.Facts) error {
		d = evaluate.Evaluate(f, evaluate.Request{Subject: subjectOf(q), Function: q.Function, Now: a.clock.Now(), Arguments: q.Arguments, Call: q.Arguments != nil})
		return nil
	})
	return d.AccessDecision, err
}

// Search evaluates every function of the connector without arguments.
func (a *Authorizer) Search(ctx context.Context, s port.Subject, connector port.ConnectorID) ([]port.FunctionDecision, error) {
	var out []port.FunctionDecision
	err := a.facts(ctx, s, connector, func(f evaluate.Facts) error {
		out = evaluate.Search(f, s, a.clock.Now())
		return nil
	})
	return out, err
}

// Groups returns the groups of a person.
func (a *Authorizer) Groups(ctx context.Context, tenant port.TenantID, person port.PersonID) ([]port.GroupID, error) {
	var out []port.GroupID
	err := a.store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		out, err = groupsOf(tx, person)
		return err
	})
	return out, err
}

func groupsOf(tx port.Tx, person port.PersonID) ([]port.GroupID, error) {
	rows, err := tx.ListMembers(port.MemberFilter{Person: person})
	out := make([]port.GroupID, len(rows))
	for i, m := range rows {
		out[i] = m.Group
	}
	return out, err
}

var operatorOnly = []Role{Operator}
var wsAdmin = []Role{WorkspaceAdmin, TenantAdmin}

var actionRoles = map[string][]Role{
	authz.SourceSync: operatorOnly, authz.KeyRotate: operatorOnly, authz.ListingImport: operatorOnly, authz.ListingCreateInst: operatorOnly, authz.ListingStatusInst: operatorOnly,
	authz.ListingStarter: operatorOnly, authz.OfferPut: operatorOnly, authz.TenantCreate: operatorOnly, authz.TenantRemove: operatorOnly,
	authz.ListingReview:     {Reviewer, Operator, TenantAdmin},
	authz.ListingCreatePriv: {TenantAdmin}, authz.ListingStatusPriv: {TenantAdmin}, authz.CatalogRead: {TenantAdmin}, authz.CatalogWrite: {TenantAdmin},
	authz.GrantTenant: {TenantAdmin}, authz.SourceRead: {TenantAdmin, Operator}, authz.ClientRegister: {TenantAdmin},
	authz.ConnectorInstall: wsAdmin, authz.ConnectorManage: wsAdmin, authz.FunctionManage: wsAdmin, authz.OverlayManage: wsAdmin,
	authz.CredentialWorkspace: wsAdmin, authz.PauseClear: append([]Role{Approver}, wsAdmin...), authz.GrantWorkspace: wsAdmin,
	authz.BindingWrite: append([]Role{AgentConfigurer}, wsAdmin...), authz.GrantAgent: append([]Role{AgentConfigurer}, wsAdmin...),
	authz.ApprovalCancel: append([]Role{Approver}, wsAdmin...), authz.ApprovalRead: append([]Role{Approver, Auditor}, wsAdmin...),
	authz.SessionEnd: wsAdmin, authz.AgentRemove: wsAdmin, authz.PersonRemove: wsAdmin, authz.AuditRead: append([]Role{Auditor}, wsAdmin...),
	authz.GrantUser: {TenantAdmin}, authz.CredentialOwn: {TenantAdmin}, authz.RoleInstallation: operatorOnly, authz.RoleManage: {TenantAdmin},
}

var selfActions = []string{authz.GrantUser, authz.CredentialOwn}
var tenantActions = []string{authz.ConnectorRead, authz.ApprovalDecide}
var systemActions = []string{authz.SessionEnd, authz.AgentRemove, authz.PersonRemove}

// Allowed answers a role question from the assignments: an agent is never allowed, the operator acts as system.
func (a *Authorizer) Allowed(ctx context.Context, p port.Permission) (bool, error) {
	if p.Principal.Kind == port.PrincipalAgent || p.Principal.Kind == port.PrincipalView || p.Principal.Claims.Agent != "" {
		return false, nil
	}
	roles := actionRoles[p.Action]
	if p.Principal.Kind == port.PrincipalSystem || p.Principal.Kind == port.PrincipalWorker {
		return slices.Contains(roles, Operator) || slices.Contains(systemActions, p.Action), nil
	}
	tenant, person := p.Principal.Claims.Tenant, p.Principal.Person
	if person == "" {
		return false, nil
	}
	if slices.Contains(tenantActions, p.Action) && authz.Covers(authz.Tenant(string(tenant)), p.Resource) {
		return true, nil
	}
	if slices.Contains(selfActions, p.Action) && p.Resource == authz.Person(string(tenant), string(person)) {
		return true, nil
	}
	var held []port.RoleAssignment
	err := a.store.Read(ctx, port.Scope{Tenant: tenant, Installation: true}, func(tx port.Tx) (err error) {
		held, err = tx.ListRoles(port.RoleFilter{Person: person})
		return err
	})
	for _, g := range held {
		if slices.Contains(roles, Role(g.Role)) && authz.Covers(g.Scope, p.Resource) && sameIssuer(g, p.Principal.Claims.Issuer) {
			return true, err
		}
	}
	return false, err
}

func sameIssuer(g port.RoleAssignment, issuer string) bool {
	return g.Issuer == issuer || (g.Issuer == "" && g.Tenant != port.Installation)
}
