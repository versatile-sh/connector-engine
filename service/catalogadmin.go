package service

import (
	"context"
	"slices"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/authz/facts"
	"github.com/versatile-sh/connector-engine/catalog"
	corecatalog "github.com/versatile-sh/connector-engine/core/catalog"
	"github.com/versatile-sh/connector-engine/port"
)

func (s *Service) blockedReasons(tx port.Tx, tenant port.TenantID, only *port.ListingRef) (map[port.ConnectorID]port.Code, error) {
	conns, err := tx.ListConnectors(port.ConnectorFilter{Listing: only})
	if err != nil {
		return nil, err
	}
	workspaces := make([]port.WorkspaceID, 0, len(conns))
	for _, c := range conns {
		workspaces = append(workspaces, c.Workspace)
	}
	view, err := facts.View(tx, tenant, workspaces, nil)
	if err != nil {
		return nil, err
	}
	out := map[port.ConnectorID]port.Code{}
	for _, c := range conns {
		if !c.RemovedAt.IsZero() {
			continue
		}
		ref := c.Listing.Listing
		switch {
		case view.Status(ref, c.Listing.Version) == port.ListingQuarantined:
			out[c.ID] = port.ReasonListingQuarantined
		case view.Status(ref, c.Listing.Version) == port.ListingDeleted || !view.Resolve(c.Workspace, nil, ref).Visible:
			out[c.ID] = port.ReasonNotInCatalog
		default:
			out[c.ID] = ""
		}
	}
	return out, nil
}

func (s *Service) announceBlocked(tx port.Tx, tenant port.TenantID, after, before map[port.ConnectorID]port.Code) ([]port.ConnectorID, error) {
	var blocked []port.ConnectorID
	for _, id := range slices.Sorted(mapKeys(after)) {
		reason := after[id]
		if reason != "" {
			blocked = append(blocked, id)
		}
		if before != nil && before[id] == reason {
			continue
		}
		ev := catalog.BlockedChanged{Connector: string(id), Blocked: reason != "", Reason: string(reason)}
		if reason == "" {
			ev.Reason = string(port.ReasonNotInCatalog)
		}
		if err := s.emit(tx, tenant, string(id), evBlockedChanged, ev); err != nil {
			return nil, err
		}
		if reason != "" {
			if _, err := s.cancelPending(tx, port.ApprovalFilter{Connector: id}, reason, nil); err != nil {
				return nil, err
			}
		}
	}
	return blocked, nil
}

func mapKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func (s *Service) syncBlocked(tx port.Tx, tenant port.TenantID, only *port.ListingRef) ([]port.ConnectorID, error) {
	after, err := s.blockedReasons(tx, tenant, only)
	if err != nil {
		return nil, err
	}
	return s.announceBlocked(tx, tenant, after, nil)
}

func (s *Service) queueFanOut(tx port.Tx, kind string, ref *port.ListingRef) (port.Operation, error) {
	tenants, err := tx.ListTenants()
	if err != nil {
		return port.Operation{}, err
	}
	var pending []port.TenantID
	for _, t := range tenants {
		if t.State == port.TenantActive {
			pending = append(pending, t.ID)
		}
	}
	op := port.Operation{ID: port.OperationID(s.d.NewID()), Kind: kind, Counts: map[string]int{"tenants_left": len(pending), "tenants_done": 0, "connectors": 0, "approvals_cancelled": 0}, CreatedAt: s.now(), UpdatedAt: s.now(), Done: len(pending) == 0}
	if err := tx.PutOperation(op); err != nil {
		return op, err
	}
	return op, tx.PutFanOut(port.FanOut{ID: op.ID, Kind: kind, Listing: ref, Pending: pending, Done: len(pending) == 0, CreatedAt: s.now()})
}

func (s *Service) putRule(tx port.Tx, by string, r port.CatalogRule) error {
	if r.ID == "" {
		r.ID = port.RuleID(s.d.NewID())
	}
	if err := tx.PutRule(r); err != nil {
		return err
	}
	return s.record(tx, r.Tenant, by, string(r.ID), evRuleChanged, ruleEvent(r, false, by))
}

func ruleEvent(r port.CatalogRule, deleted bool, by string) catalog.RuleChanged {
	aud := r.Audience
	if aud == "" {
		aud = port.AudienceAll
	}
	return catalog.RuleChanged{Rule: string(r.ID), SubjectKind: string(r.SubjectKind), SubjectID: r.SubjectID, TargetKind: string(r.TargetKind), Target: r.Target,
		Show: r.Show, Install: r.Install, Ceiling: string(r.Ceiling), Audience: string(aud), Deleted: deleted, By: by}
}

// PutCatalogRule writes a rule of the caller's tenant; connectors it newly blocks or unblocks are announced in the same transaction.
func (s *Service) PutCatalogRule(ctx context.Context, p port.Principal, r port.CatalogRule) (port.CatalogRule, error) {
	if err := s.authorize(ctx, p, authz.CatalogWrite, s.tenantResource(p)); err != nil {
		return r, err
	}
	r.Tenant, r.CreatedBy = p.Claims.Tenant, actor(p)
	if r.Audience == "" {
		r.Audience = port.AudienceAll
	}
	if err := corecatalog.ValidateRule(r); err != nil {
		return r, err
	}
	if r.ID == "" {
		r.ID = port.RuleID(s.d.NewID())
	}
	return r, s.changeCatalog(ctx, p, "PutCatalogRule", func(tx port.Tx) error { return s.putRule(tx, actor(p), r) })
}

// DeleteCatalogRule deletes a rule of the caller's tenant.
func (s *Service) DeleteCatalogRule(ctx context.Context, p port.Principal, id port.RuleID) error {
	if err := s.authorize(ctx, p, authz.CatalogWrite, s.tenantResource(p)); err != nil {
		return err
	}
	return s.changeCatalog(ctx, p, "DeleteCatalogRule", func(tx port.Tx) error {
		rules, err := tx.ListRules()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(rules, func(r port.CatalogRule) bool { return r.ID == id })
		if i < 0 {
			return port.Errorf(port.CodeNotFound, "rule %s not found", id)
		}
		if err := tx.DeleteRule(id); err != nil {
			return err
		}
		return s.record(tx, p.Claims.Tenant, actor(p), string(id), evRuleChanged, ruleEvent(rules[i], true, actor(p)))
	})
}

func (s *Service) changeCatalog(ctx context.Context, p port.Principal, op string, fn func(port.Tx) error) error {
	tenant := p.Claims.Tenant
	return s.write(ctx, port.Scope{Tenant: tenant}, op, func(tx port.Tx) error {
		before, err := s.blockedReasons(tx, tenant, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		after, err := s.blockedReasons(tx, tenant, nil)
		if err != nil {
			return err
		}
		_, err = s.announceBlocked(tx, tenant, after, before)
		return err
	})
}

// ListCatalogRules returns the rules of the caller's tenant.
func (s *Service) ListCatalogRules(ctx context.Context, p port.Principal) ([]port.CatalogRule, error) {
	if err := s.authorize(ctx, p, authz.CatalogRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.CatalogRule
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListRules()
		return err
	})
	return out, err
}

// PutCatalogPolicy sets the default of a workspace; a policy for the tenant itself is refused.
func (s *Service) PutCatalogPolicy(ctx context.Context, p port.Principal, subject port.SubjectKind, workspace port.WorkspaceID, def port.PolicyDefault) (port.CatalogPolicy, error) {
	pol := port.CatalogPolicy{Tenant: p.Claims.Tenant, Workspace: workspace, Default: def}
	if err := corecatalog.ValidatePolicy(subject); err != nil {
		return pol, err
	}
	if err := s.authorize(ctx, p, authz.CatalogWrite, s.tenantResource(p)); err != nil {
		return pol, err
	}
	if def != port.PolicyVisible && def != port.PolicyHidden {
		return pol, port.Errorf(port.CodeInvalidArgument, "policy default is visible or hidden")
	}
	return pol, s.changeCatalog(ctx, p, "PutCatalogPolicy", func(tx port.Tx) error {
		if err := tx.PutPolicy(pol); err != nil {
			return err
		}
		return s.record(tx, pol.Tenant, actor(p), string(workspace), evPolicyChanged,
			catalog.PolicyChanged{SubjectKind: string(subject), SubjectID: string(workspace), Default: string(def), By: actor(p)})
	})
}

// GetCatalogPolicy returns the policy of a workspace; none stored means visible.
func (s *Service) GetCatalogPolicy(ctx context.Context, p port.Principal, workspace port.WorkspaceID) (port.CatalogPolicy, error) {
	out := port.CatalogPolicy{Tenant: p.Claims.Tenant, Workspace: workspace, Default: port.PolicyVisible}
	if err := s.authorize(ctx, p, authz.CatalogRead, s.tenantResource(p)); err != nil {
		return out, err
	}
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		got, ok, err := tx.GetPolicy(workspace)
		if ok {
			out = got
		}
		return err
	})
	return out, err
}

// PutOffer withholds or offers a source or one installation listing to a tenant and queues the fan-out.
func (s *Service) PutOffer(ctx context.Context, p port.Principal, o port.Offer) (port.Operation, error) {
	var op port.Operation
	if err := s.authorize(ctx, p, authz.OfferPut, authz.Installation); err != nil {
		return op, err
	}
	if o.TargetKind != port.TargetSource && o.TargetKind != port.TargetListing {
		return op, port.Errorf(port.CodeInvalidArgument, "an offer names a source or a listing")
	}
	err := s.write(ctx, port.Scope{Installation: true}, "PutOffer", func(tx port.Tx) error {
		if _, err := tx.GetTenant(o.Tenant); err != nil {
			return err
		}
		if err := tx.PutOffer(o); err != nil {
			return err
		}
		var ref *port.ListingRef
		if o.TargetKind == port.TargetListing {
			ref = &port.ListingRef{Name: o.Target}
			if owner, name, ok := splitRef(o.Target); ok {
				ref = &port.ListingRef{Owner: owner, Name: name}
			}
		}
		var err error
		if op, err = s.queueFanOut(tx, "offer", ref); err != nil {
			return err
		}
		return s.record(tx, port.Installation, actor(p), string(o.Tenant)+"/"+o.Target, evOfferChanged,
			catalog.OfferChanged{Tenant: string(o.Tenant), TargetKind: string(o.TargetKind), Target: o.Target, Withheld: o.Withheld})
	})
	return op, err
}

func splitRef(target string) (port.TenantID, string, bool) {
	for i := 0; i < len(target); i++ {
		if target[i] == '/' {
			return port.TenantID(target[:i]), target[i+1:], true
		}
	}
	return "", "", false
}

// ListOffers returns the offers of one tenant.
func (s *Service) ListOffers(ctx context.Context, p port.Principal, tenant port.TenantID) ([]port.Offer, error) {
	if err := s.authorize(ctx, p, authz.OfferPut, authz.Installation); err != nil {
		return nil, err
	}
	var out []port.Offer
	err := s.d.Store.Read(ctx, port.Scope{Installation: true}, func(tx port.Tx) (err error) {
		out, err = tx.ListOffers(tenant)
		return err
	})
	return out, err
}

// ExplainVisibility answers the levels for (workspace, person, listing); a workspace admin gets not found for a hidden listing.
func (s *Service) ExplainVisibility(ctx context.Context, p port.Principal, workspace port.WorkspaceID, person port.PersonID, ref port.ListingRef) (corecatalog.Answer, error) {
	var out corecatalog.Answer
	tenantAdmin := s.authorize(ctx, p, authz.CatalogRead, s.tenantResource(p)) == nil
	if !tenantAdmin {
		if err := s.authorize(ctx, p, authz.ConnectorManage, authz.Workspace(string(p.Claims.Tenant), string(workspace))); err != nil {
			return out, err
		}
	}
	var people []port.PersonID
	if person != "" {
		people = append(people, person)
	}
	g, err := s.groups(ctx, p.Claims.Tenant, people...)
	if err != nil {
		return out, err
	}
	err = s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		v, err := facts.View(tx, p.Claims.Tenant, []port.WorkspaceID{workspace}, g)
		if err != nil {
			return err
		}
		out = v.Resolve(workspace, people, ref)
		if !tenantAdmin && !v.Resolve(workspace, nil, ref).Visible {
			return port.Errorf(port.CodeNotFound, "%s not found", ref)
		}
		return nil
	})
	return out, err
}
