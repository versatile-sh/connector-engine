package service

import (
	"context"
	"encoding/json"

	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/port"
)

// ExpireApproval is A6: it acts only when the record is pending and its expiry passed.
func (s *Service) ExpireApproval(ctx context.Context, tenant port.TenantID, id port.ApprovalID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		exp, err := approval.Expire(a, s.now())
		if err != nil {
			return nil
		}
		if err := tx.UpdateApproval(exp, port.ApprovalPending); err != nil {
			return err
		}
		return s.emit(tx, tenant, string(id), evApprovalExpired, catalog.ApprovalRef{Approval: string(id)})
	})
}

// RetainApproval is A13: it destroys the stored result and the sealed arguments once retention ended.
func (s *Service) RetainApproval(ctx context.Context, tenant port.TenantID, id port.ApprovalID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		kept, err := approval.Retain(a, s.now())
		if err != nil {
			return nil
		}
		return tx.UpdateApproval(kept, a.State)
	})
}

// MarkExecutionUnknown is A12 with its audit row and event 27: the deadline plus grace passed with no answer.
func (s *Service) MarkExecutionUnknown(ctx context.Context, tenant port.TenantID, id port.ApprovalID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		unk, err := approval.Unknown(a, s.cfg.Approval, s.now())
		if err != nil {
			return nil
		}
		if err := tx.UpdateApproval(unk, port.ApprovalExecuting); err != nil {
			return err
		}
		call, ok, err := tx.LatestCall(a.Session, a.CallHash)
		if err != nil {
			return err
		}
		if !ok || call.Approval != id || call.State != port.CallStarted {
			return nil
		}
		return s.callUnknown(tx, call)
	})
}

func (s *Service) callUnknown(tx port.Tx, call port.Call) error {
	unk := call
	unk.State, unk.FinishedAt = port.CallOutcomeUnknown, s.now()
	if err := tx.UpdateCall(unk, port.CallStarted); err != nil {
		return err
	}
	return s.record(tx, call.Tenant, "worker", string(call.ID), evCallUnknown, catalog.CallOutcomeUnknown{Call: string(call.ID), Approval: string(call.Approval), Function: call.Function, StartedAt: call.StartedAt})
}

// SweepCall is C12 for a call that was never held: a row left in started beyond its deadline plus grace becomes outcome_unknown.
func (s *Service) SweepCall(ctx context.Context, tenant port.TenantID, id port.CallID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		call, err := tx.GetCall(id)
		if err != nil {
			return err
		}
		if call.State != port.CallStarted || call.Approval != "" || s.now().Before(call.Deadline.Add(s.cfg.Approval.ExecuteGrace)) {
			return nil
		}
		return s.callUnknown(tx, call)
	})
}

// ExpireGrant deletes a row whose expiry passed and publishes event 12.
func (s *Service) ExpireGrant(ctx context.Context, tenant port.TenantID, id port.GrantID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		g, err := tx.GetGrant(id)
		if port.IsCode(err, port.CodeNotFound) {
			return nil
		}
		if err != nil || g.ExpiresAt.IsZero() || s.now().Before(g.ExpiresAt) {
			return err
		}
		if err := tx.DeleteGrant(id); err != nil {
			return err
		}
		return s.record(tx, tenant, "worker", string(id), evGrantExpired, catalog.GrantRef{Grant: string(id)})
	})
}

// ExpirePauses ends every pause of the tenant whose expiry passed (§8.3).
func (s *Service) ExpirePauses(ctx context.Context, tenant port.TenantID) error {
	return s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		all, err := tx.ListPauses()
		if err != nil {
			return err
		}
		for _, pz := range all {
			if pz.Paused && !pz.ExpiresAt.IsZero() && !s.now().Before(pz.ExpiresAt) {
				if err := tx.PutPause(approval.Clear(pz)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// RelistDue re-lists every enabled connector of the tenant whose upstream ttl, kept within pinning.relistMin and relistMax, has lapsed.
func (s *Service) RelistDue(ctx context.Context, tenant port.TenantID) ([]port.ConnectorID, error) {
	var due []port.ConnectorID
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		conns, err := tx.ListConnectors(port.ConnectorFilter{})
		for _, c := range conns {
			if c.Enabled && c.RemovedAt.IsZero() && s.relistDue(c) {
				due = append(due, c.ID)
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, id := range due {
		if _, err := s.Relist(ctx, tenant, id); err != nil {
			return due, err
		}
	}
	return due, nil
}

// RefreshDueCredentials refreshes the credentials of the tenant whose refresh_after passed.
func (s *Service) RefreshDueCredentials(ctx context.Context, tenant port.TenantID) error {
	var due []port.CredentialID
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		creds, err := tx.ListCredentials(port.CredentialFilter{})
		for _, c := range creds {
			if c.Status == port.CredentialActive && !c.RefreshAfter.IsZero() && !s.now().Before(c.RefreshAfter) {
				due = append(due, c.ID)
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range due {
		if err := s.RefreshCredential(ctx, tenant, id); err != nil {
			return err
		}
	}
	return nil
}

type heldRun struct {
	A      port.Approval
	Conn   port.Connector
	Bind   port.Binding
	M      port.Manifest
	Args   []byte
	Sub    port.Subject
	Groups map[port.PersonID][]port.GroupID
}

func (s *Service) loadHeld(ctx context.Context, tenant port.TenantID, id port.ApprovalID) (heldRun, bool, error) {
	var h heldRun
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		var err error
		if h.A, err = tx.GetApproval(id); err != nil || h.A.State != port.ApprovalApproved {
			return err
		}
		if h.Conn, err = tx.GetConnector(h.A.Connector); err != nil {
			return err
		}
		if h.Bind, err = tx.GetBinding(h.A.Connector, h.A.Agent); err != nil && !port.IsCode(err, port.CodeNotFound) {
			return err
		}
		lv, err := tx.GetVersion(h.Conn.Listing)
		h.M = lv.Manifest
		return err
	})
	if err != nil || h.A.State != port.ApprovalApproved {
		return h, false, err
	}
	h.Sub = port.Subject{Tenant: tenant, Workspace: h.A.Workspace, Agent: h.A.Agent, AgentOwner: h.Bind.AgentOwner, OnBehalfOf: h.A.OnBehalfOf, Session: h.A.Session, Caller: port.CallerModel}
	if h.A.Arguments != nil {
		if h.Args, err = s.d.Secrets.Open(ctx, tenant, *h.A.Arguments); err != nil {
			return h, false, err
		}
	}
	h.Groups, err = s.groups(ctx, tenant, evaluate.People(h.Sub)...)
	return h, true, err
}

// ExecuteApproved is A8 and A9 for one approved record: re-list, re-check and guards, then one forward; a failed re-list leaves it approved
// for the next sweep until it is stale (A9), a failed guard cancels it.
func (s *Service) ExecuteApproved(ctx context.Context, tenant port.TenantID, id port.ApprovalID) error {
	h, ok, err := s.loadHeld(ctx, tenant, id)
	if err != nil || !ok {
		return err
	}
	if _, err := s.Relist(ctx, tenant, h.A.Connector); err != nil && s.now().Sub(h.A.DecidedAt) <= s.cfg.Approval.ExecuteWithin {
		return err
	}
	ext, err := s.d.Authorizer.Evaluate(ctx, port.Query{Tenant: tenant, Workspace: h.A.Workspace, Agent: h.A.Agent, AgentOwner: h.Sub.AgentOwner, OnBehalfOf: h.A.OnBehalfOf,
		Caller: port.CallerModel, Session: h.A.Session, Connector: h.A.Connector, Function: h.A.Function, Class: h.A.Class, Arguments: nonEmpty(h.Args)})
	if err != nil {
		return err
	}
	var plan *forwardPlan
	var started port.Approval
	err = s.d.Store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		cur, err := tx.GetApproval(id)
		if err != nil || cur.State != port.ApprovalApproved {
			return err
		}
		refused, cred, reserved, err := s.recheck(tx, h, cur, ext)
		if err != nil {
			return err
		}
		timeout := cmpOr(h.Conn.CallTimeout, s.cfg.CallTimeout)
		next, err := approval.Start(cur, refused, timeout, s.cfg.Approval, s.now())
		if err != nil {
			return err
		}
		if next.State == port.ApprovalCancelled {
			if err := tx.UpdateApproval(next, port.ApprovalApproved); err != nil {
				return err
			}
			return s.emit(tx, tenant, string(id), evApprovalCancelled, catalog.ApprovalCancelled{Approval: string(id), Reason: next.Reason})
		}
		if err := tx.ReserveGrants(reserved); err != nil {
			return err
		}
		fn, err := tx.GetFunction(cur.Connector, cur.Function)
		if err != nil {
			return err
		}
		call, err := s.startCall(tx, h.Conn, fn, h.Sub, cur.CallHash, cur.ID, cred)
		if err != nil {
			return err
		}
		plan, started = &forwardPlan{Call: call, Credential: cred, Conn: h.Conn}, next
		return tx.UpdateApproval(next, port.ApprovalApproved)
	})
	if err != nil || plan == nil {
		return err
	}
	inv := s.invoke(ctx, invocation{Conn: h.Conn, Manifest: h.M, Function: h.A.Function, Credential: plan.Credential, Args: json.RawMessage(nonEmpty(h.Args)), Fields: h.A.Fields, Deadline: plan.Call.Deadline})
	_, err = s.settle(ctx, plan.Call, inv, &started)
	return err
}

func nonEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func (s *Service) recheck(tx port.Tx, h heldRun, cur port.Approval, ext port.AccessDecision) (port.Code, *port.Credential, []port.GrantID, error) {
	f, err := s.facts(tx, h.Sub, cur.Connector, h.Groups)
	if err != nil {
		return "", nil, nil, err
	}
	d := evaluate.Evaluate(f, evaluate.Request{Subject: h.Sub, Function: cur.Function, Now: s.now(), Arguments: nonEmpty(h.Args), Call: true})
	switch fn := f.Functions[cur.Function]; {
	case d.Mode == port.ModeDeny:
		return d.Reason, nil, nil, nil
	case ext.Mode == port.ModeDeny:
		return ext.Reason, nil, nil, nil
	case fn.DefinitionHash != cur.DefinitionHash:
		return port.CodeDefinitionChanged, nil, nil, nil
	}
	res, err := s.resolveCredential(tx, h.Conn, h.Bind, h.Sub.OnBehalfOf, cur.Function, h.M)
	switch {
	case err != nil:
		return "", nil, nil, err
	case res.Code != "":
		return port.CodeCredentialRequired, nil, nil, nil
	}
	for _, id := range d.Matched {
		g, err := tx.GetGrant(id)
		if err != nil {
			return "", nil, nil, err
		}
		if g.MaxUses > 0 && g.Uses >= g.MaxUses {
			return port.CodeGrantExhausted, nil, nil, nil
		}
	}
	return "", res.Credential, d.Matched, nil
}
