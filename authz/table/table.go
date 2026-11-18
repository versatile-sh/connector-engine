// Package table is the built-in port.Authorizer: the table evaluator of §7.1 over the store, and the role table of §4.2.
package table

import (
	"context"
	"slices"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/port"
)

// Authorizer evaluates calls with core/evaluate over the store's rows and answers role questions from Roles.
type Authorizer struct {
	store    port.Store
	clock    port.Clock
	roles    *Roles
	settings Settings
}

// New returns the table authorizer.
func New(store port.Store, clock port.Clock, roles *Roles, settings Settings) *Authorizer {
	return &Authorizer{store: store, clock: clock, roles: roles, settings: settings}
}

var _ port.Authorizer = (*Authorizer)(nil)

func subjectOf(q port.Query) port.Subject {
	return port.Subject{Tenant: q.Tenant, Workspace: q.Workspace, Agent: q.Agent, AgentOwner: q.AgentOwner, OnBehalfOf: q.OnBehalfOf, Session: q.Session, Caller: q.Caller}
}

func (a *Authorizer) facts(ctx context.Context, s port.Subject, connector port.ConnectorID, fn func(evaluate.Facts) error) error {
	groups := map[port.PersonID][]port.GroupID{}
	for _, p := range evaluate.People(s) {
		groups[p] = a.roles.groupsOf(s.Tenant, p)
	}
	return a.store.Read(ctx, port.Scope{Tenant: s.Tenant}, func(tx port.Tx) error {
		f, err := LoadFacts(tx, a.clock.Now(), s, connector, groups, a.settings)
		if err != nil {
			return err
		}
		return fn(f)
	})
}

// Evaluate runs §7.1 steps 1 to 14 for one function; step 15(b) has no external point here.
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
func (a *Authorizer) Groups(_ context.Context, tenant port.TenantID, person port.PersonID) ([]port.GroupID, error) {
	return a.roles.groupsOf(tenant, person), nil
}

var operatorOnly = []Role{Operator}
var wsAdmin = []Role{WorkspaceAdmin, TenantAdmin}

var actionRoles = map[string][]Role{
	authz.SourceSync: operatorOnly, authz.ListingImport: operatorOnly, authz.ListingCreateInst: operatorOnly, authz.ListingStatusInst: operatorOnly,
	authz.ListingStarter: operatorOnly, authz.OfferPut: operatorOnly, authz.TenantCreate: operatorOnly, authz.TenantRemove: operatorOnly,
	authz.ListingReview:     {Reviewer, Operator, TenantAdmin},
	authz.ListingCreatePriv: {TenantAdmin}, authz.ListingStatusPriv: {TenantAdmin}, authz.CatalogRead: {TenantAdmin}, authz.CatalogWrite: {TenantAdmin},
	authz.GrantTenant:      {TenantAdmin},
	authz.ConnectorInstall: wsAdmin, authz.ConnectorManage: wsAdmin, authz.FunctionManage: wsAdmin, authz.OverlayManage: wsAdmin,
	authz.CredentialWorkspace: wsAdmin, authz.PauseClear: append([]Role{Approver}, wsAdmin...), authz.GrantWorkspace: wsAdmin,
	authz.BindingWrite: append([]Role{AgentConfigurer}, wsAdmin...), authz.GrantAgent: append([]Role{AgentConfigurer}, wsAdmin...),
	authz.ApprovalCancel: append([]Role{Approver}, wsAdmin...), authz.ApprovalRead: append([]Role{Approver, Auditor}, wsAdmin...),
	authz.SessionEnd: wsAdmin, authz.AgentRemove: wsAdmin, authz.PersonRemove: wsAdmin, authz.AuditRead: append([]Role{Auditor}, wsAdmin...),
	authz.GrantUser: {TenantAdmin}, authz.CredentialOwn: {TenantAdmin},
}

var selfActions = []string{authz.GrantUser, authz.CredentialOwn}
var tenantActions = []string{authz.ConnectorRead, authz.ApprovalDecide}
var systemActions = []string{authz.SessionEnd, authz.AgentRemove, authz.PersonRemove}

// Allowed answers a role question of §4.2 from the assignments: an agent is never allowed, the operator acts as system.
func (a *Authorizer) Allowed(_ context.Context, p port.Permission) (bool, error) {
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
	for _, g := range a.roles.of(tenant, person) {
		if slices.Contains(roles, g.Role) && authz.Covers(g.Scope, p.Resource) {
			return true, nil
		}
	}
	return false, nil
}
