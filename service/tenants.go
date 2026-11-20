package service

import (
	"context"
	"slices"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/catalog"
	corecatalog "github.com/versatile-sh/connector-engine/core/catalog"
	"github.com/versatile-sh/connector-engine/port"
)

// RemoveTenantKind is the kind of the fan-out row of a tenant removal.
const RemoveTenantKind = "remove_tenant"

// CreateTenant registers a tenant with its key and, under catalog.tenantDefault = starter, the starter rules; a second call writes nothing.
func (s *Service) CreateTenant(ctx context.Context, p port.Principal, tenant port.TenantID) (port.Tenant, error) {
	var out port.Tenant
	if err := s.authorize(ctx, p, authz.TenantCreate, authz.Installation); err != nil {
		return out, err
	}
	if tenant == port.Installation {
		return out, port.Errorf(port.CodeInvalidArgument, "a tenant has an id")
	}
	if err := port.CheckIDs("tenant", string(tenant)); err != nil {
		return out, err
	}
	sealed, err := s.d.Secrets.Seal(ctx, tenant, []byte("tenant-key"))
	if err != nil {
		return out, err
	}
	err = s.write(ctx, port.Scope{Tenant: tenant, Installation: true}, "CreateTenant", func(tx port.Tx) error {
		if t, err := tx.GetTenant(tenant); err == nil {
			out = t
			return nil
		}
		out = port.Tenant{ID: tenant, State: port.TenantActive, KeyRef: sealed.KeyRef, CreatedBy: actor(p), CreatedAt: s.now()}
		if err := tx.PutTenant(out); err != nil {
			return err
		}
		starters := 0
		if s.cfg.TenantDefault == "starter" {
			listings, err := tx.ListListings()
			if err != nil {
				return err
			}
			for _, r := range corecatalog.StarterRules(tenant, listings) {
				if err := s.putRule(tx, actor(p), r); err != nil {
					return err
				}
				starters++
			}
		}
		return s.record(tx, port.Installation, actor(p), string(tenant), evTenantCreated, catalog.TenantCreated{Tenant: string(tenant), By: actor(p), StarterRules: starters})
	})
	return out, err
}

// RemoveTenant marks a tenant removing, which refuses every token of it at once, and queues the worker's steps 2 to 5.
func (s *Service) RemoveTenant(ctx context.Context, p port.Principal, tenant port.TenantID) (port.Operation, error) {
	var op port.Operation
	if err := s.authorize(ctx, p, authz.TenantRemove, authz.Installation); err != nil {
		return op, err
	}
	err := s.write(ctx, port.Scope{Installation: true}, "RemoveTenant", func(tx port.Tx) error {
		t, err := tx.GetTenant(tenant)
		if err != nil {
			return err
		}
		if t.State != port.TenantActive {
			return port.Errorf(port.CodeConflict, "tenant %s is %s", tenant, t.State)
		}
		t.State = port.TenantRemoving
		if err := tx.PutTenant(t); err != nil {
			return err
		}
		op = port.Operation{ID: port.OperationID(s.d.NewID()), Kind: RemoveTenantKind, Counts: map[string]int{}, CreatedAt: s.now(), UpdatedAt: s.now()}
		if err := tx.PutOperation(op); err != nil {
			return err
		}
		return tx.PutFanOut(port.FanOut{ID: op.ID, Kind: RemoveTenantKind, Tenant: tenant, Pending: []port.TenantID{tenant}, CreatedAt: s.now()})
	})
	return op, err
}

// GetOperation returns the progress of an operation of the caller's tenant, or of the installation for the operator.
func (s *Service) GetOperation(ctx context.Context, p port.Principal, id port.OperationID) (port.Operation, error) {
	var out port.Operation
	action, resource := authz.ConnectorRead, s.tenantResource(p)
	if p.Kind == port.PrincipalSystem {
		action, resource = authz.TenantCreate, authz.Installation
	}
	if err := s.authorize(ctx, p, action, resource); err != nil {
		return out, err
	}
	scope := port.Scope{Tenant: p.Claims.Tenant, Installation: p.Kind == port.PrincipalSystem}
	err := s.d.Store.Read(ctx, scope, func(tx port.Tx) (err error) {
		out, err = tx.GetOperation(id)
		return err
	})
	return out, err
}

// FanOutTenant is the worker's step of an operator's status change or offer for one tenant: events 36 and A7 inside that tenant.
func (s *Service) FanOutTenant(ctx context.Context, f port.FanOut, tenant port.TenantID) error {
	var blocked, connectors int
	err := s.write(ctx, port.Scope{Tenant: tenant}, "FanOutTenant:"+string(f.ID), func(tx port.Tx) error {
		reasons, err := s.blockedReasons(tx, tenant, f.Listing)
		if err != nil {
			return err
		}
		ids, err := s.announceBlocked(tx, tenant, reasons, nil)
		blocked, connectors = len(ids), len(reasons)
		return err
	})
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Installation: true}, "FanOutDone:"+string(f.ID)+string(tenant), func(tx port.Tx) error {
		cur, err := s.fanOut(tx, f.ID)
		if err != nil {
			return err
		}
		cur.Pending = slices.DeleteFunc(cur.Pending, func(t port.TenantID) bool { return t == tenant })
		cur.Done = len(cur.Pending) == 0
		if err := tx.PutFanOut(cur); err != nil {
			return err
		}
		op, err := tx.GetOperation(f.ID)
		if err != nil {
			return err
		}
		op.Counts["tenants_left"], op.Counts["tenants_done"] = len(cur.Pending), op.Counts["tenants_done"]+1
		op.Counts["connectors"] += connectors
		op.Counts["connectors_blocked"] += blocked
		op.Done, op.UpdatedAt = cur.Done, s.now()
		return tx.PutOperation(op)
	})
}

func (s *Service) fanOut(tx port.Tx, id port.OperationID) (port.FanOut, error) {
	all, err := tx.ListFanOuts()
	if err != nil {
		return port.FanOut{}, err
	}
	i := slices.IndexFunc(all, func(f port.FanOut) bool { return f.ID == id })
	if i < 0 {
		return port.FanOut{}, port.Errorf(port.CodeNotFound, "fan-out %s not found", id)
	}
	return all[i], nil
}

// RemoveTenantData is the worker's steps 2 to 5: cancel open approvals, shred the key, delete the rows, publish event 38.
func (s *Service) RemoveTenantData(ctx context.Context, f port.FanOut) error {
	tenant := f.Tenant
	var creds []port.Credential
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) (err error) {
		creds, err = tx.ListCredentials(port.CredentialFilter{})
		return err
	}); err != nil {
		return err
	}
	s.revokeUpstream(ctx, creds)
	counts := map[string]int{}
	err := s.write(ctx, port.Scope{Tenant: tenant}, "RemoveTenantData:"+string(f.ID), func(tx port.Tx) error {
		conns, err := tx.ListConnectors(port.ConnectorFilter{})
		if err != nil {
			return err
		}
		approvals, err := tx.ListApprovals(port.ApprovalFilter{})
		if err != nil {
			return err
		}
		counts["connectors"], counts["credentials"], counts["approvals"] = len(conns), len(creds), len(approvals)
		if _, err := s.cancelPending(tx, port.ApprovalFilter{}, port.CodeTenantRemoved, nil); err != nil {
			return err
		}
		sessions, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeSession})
		if err != nil {
			return err
		}
		for _, g := range sessions {
			if err := tx.DeleteGrant(g.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.d.Secrets.Destroy(ctx, tenant); err != nil {
		return err
	}
	s.stopHandles(ctx, tenant)
	if err := s.purge(ctx, tenant); err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Installation: true}, "RemoveTenantDone:"+string(f.ID), func(tx port.Tx) error {
		t, err := tx.GetTenant(tenant)
		if err != nil {
			return err
		}
		t.State, t.RemovedAt, t.Counts, t.KeyRef = port.TenantRemoved, s.now(), counts, ""
		if err := tx.PutTenant(t); err != nil {
			return err
		}
		f.Pending, f.Done = nil, true
		if err := tx.PutFanOut(f); err != nil {
			return err
		}
		op, err := tx.GetOperation(f.ID)
		if err != nil {
			return err
		}
		op.Counts, op.Done, op.UpdatedAt = counts, true, s.now()
		if err := tx.PutOperation(op); err != nil {
			return err
		}
		return s.record(tx, port.Installation, "worker", string(tenant), evTenantRemoved, catalog.TenantRemoved{
			Tenant: string(tenant), By: "worker", Connectors: counts["connectors"], Credentials: counts["credentials"], Approvals: counts["approvals"], Key: s.cfg.KeyOutcome})
	})
}

func (s *Service) purge(ctx context.Context, tenant port.TenantID) error {
	purger, ok := s.d.Store.(port.TenantPurger)
	if !ok {
		return port.Errorf(port.CodeInternal, "the store cannot purge a removed tenant")
	}
	_, err := purger.PurgeTenant(ctx, tenant)
	return err
}
