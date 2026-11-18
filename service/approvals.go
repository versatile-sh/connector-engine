package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/port"
)

const reasonCancelledByApprover port.Code = "cancelled_by_approver"

// ApprovalDetail is a record with its arguments opened for the reader and the warnings of §8.5.
type ApprovalDetail struct {
	Approval       port.Approval   `json:"approval"`
	Arguments      []byte          `json:"arguments,omitempty"`
	ChangedRecent  bool            `json:"changed_recent"`
	MayHaveRun     bool            `json:"may_have_run"`
	TrustLevel     port.TrustLevel `json:"trust_level"`
	Publisher      string          `json:"publisher"`
	ReachesOutside bool            `json:"reaches_outside"`
}

func (s *Service) approverKeys(ctx context.Context, p port.Principal) ([]string, error) {
	keys := []string{string(p.Person)}
	g, err := s.d.Authorizer.Groups(ctx, p.Claims.Tenant, p.Person)
	for _, x := range g {
		keys = append(keys, string(x))
	}
	return keys, err
}

func (s *Service) mayRead(ctx context.Context, p port.Principal, a port.Approval) (bool, error) {
	keys, err := s.approverKeys(ctx, p)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(a.Approvers, func(x string) bool { return slices.Contains(keys, x) }) {
		return true, nil
	}
	return s.authorize(ctx, p, authz.ApprovalRead, authz.Workspace(string(a.Tenant), string(a.Workspace))) == nil, nil
}

// ListApprovals returns the approvals the caller may read: those that name them or their groups, or all for an admin or auditor.
func (s *Service) ListApprovals(ctx context.Context, p port.Principal, f port.ApprovalFilter) ([]port.Approval, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var all, out []port.Approval
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		all, err = tx.ListApprovals(f)
		return err
	}); err != nil {
		return nil, err
	}
	for _, a := range all {
		if ok, err := s.mayRead(ctx, p, a); err != nil {
			return nil, err
		} else if ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// GetApproval returns one record with its arguments opened for this reader (§13.1 getApproval).
func (s *Service) GetApproval(ctx context.Context, p port.Principal, id port.ApprovalID) (ApprovalDetail, error) {
	var d ApprovalDetail
	var siblings []port.Approval
	var fn port.Function
	var lv port.ListingVersion
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		d.Approval = a
		if siblings, err = tx.ListApprovals(port.ApprovalFilter{Session: a.Session, CallHash: a.CallHash}); err != nil {
			return err
		}
		if fn, err = tx.GetFunction(a.Connector, a.Function); err != nil {
			return err
		}
		c, err := tx.GetConnector(a.Connector)
		if err != nil {
			return err
		}
		lv, err = tx.GetVersion(c.Listing)
		return err
	})
	if err != nil {
		return d, err
	}
	if ok, err := s.mayRead(ctx, p, d.Approval); err != nil || !ok {
		if err == nil {
			err = port.Errorf(port.CodeNotFound, "approval %s not found", id)
		}
		return ApprovalDetail{}, err
	}
	if d.Approval.Arguments != nil {
		if d.Arguments, err = s.d.Secrets.Open(ctx, p.Claims.Tenant, *d.Approval.Arguments); err != nil {
			return d, err
		}
	}
	d.ChangedRecent = !fn.AcceptedAt.IsZero() && s.now().Sub(fn.AcceptedAt) < time.Duration(s.cfg.ChangedWarningDays)*24*time.Hour
	d.MayHaveRun = slices.ContainsFunc(siblings, func(o port.Approval) bool { return o.ID != id && o.State == port.ApprovalOutcomeUnknown })
	d.TrustLevel, d.Publisher, d.ReachesOutside = lv.Manifest.TrustLevel, lv.Manifest.Publisher.Name, fn.OpenWorld
	return d, nil
}

// DecideApproval is A3, A4 and A5; when another decision won the race the record as it now is comes back (§13.1).
func (s *Service) DecideApproval(ctx context.Context, p port.Principal, id port.ApprovalID, d port.Decision, note, channel string) (port.Approval, error) {
	var out port.Approval
	if err := s.authorize(ctx, p, authz.ApprovalDecide, s.tenantResource(p)); err != nil {
		return out, err
	}
	keys, err := s.approverKeys(ctx, p)
	if err != nil {
		return out, err
	}
	err = s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "DecideApproval", func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		by := string(p.Person)
		if i := slices.IndexFunc(a.Approvers, func(x string) bool { return slices.Contains(keys, x) }); i >= 0 {
			by = a.Approvers[i]
		}
		decided, session, err := approval.Decide(a, by, true, d, note, channel, s.cfg.Approval, s.now())
		if port.IsCode(err, port.CodeInvalidTransition) {
			out = a
			return nil
		}
		if err != nil {
			return err
		}
		decided.DecidedBy = string(p.Person)
		if err := tx.UpdateApproval(decided, port.ApprovalPending); err != nil {
			return err
		}
		out = decided
		if err := s.record(tx, a.Tenant, actor(p), string(id), evApprovalDecided, catalog.ApprovalDecided{Approval: string(id), Decision: string(d), By: actor(p), Channel: channel, SessionGrant: session}); err != nil {
			return err
		}
		if session {
			if err := s.writeSessionRow(tx, decided); err != nil {
				return err
			}
		}
		return s.countDecision(tx, decided, d)
	})
	return out, err
}

func (s *Service) writeSessionRow(tx port.Tx, a port.Approval) error {
	row := grant.SessionRow(a, s.now(), time.Time{}, s.cfg.SessionGrantMax)
	existing, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeSession, ScopeID: string(a.Session)})
	if err != nil {
		return err
	}
	for _, g := range existing {
		if g.Key() == row.Key() {
			if err := s.revokeGrant(tx, g, a.DecidedBy, "replaced"); err != nil {
				return err
			}
		}
	}
	return s.putGrant(tx, s.newGrant(row), a.DecidedBy)
}

func (s *Service) countDecision(tx port.Tx, a port.Approval, d port.Decision) error {
	pz, ok, err := tx.GetPause(a.Session, a.Agent, a.Connector)
	if err != nil {
		return err
	}
	if !ok {
		pz = port.Pause{ID: port.PauseID(s.d.NewID()), Tenant: a.Tenant, Session: a.Session, Agent: a.Agent, Connector: a.Connector}
	}
	pz, fired := approval.Brake(pz, d, s.cfg.Approval, s.now(), s.now().Add(s.cfg.SessionGrantMax))
	if err := tx.PutPause(pz); err != nil {
		return err
	}
	if !fired {
		return nil
	}
	if err := s.record(tx, a.Tenant, "system", string(pz.ID), evBrake, catalog.Brake{Agent: string(a.Agent), Session: string(a.Session), Connector: string(a.Connector), Consecutive: pz.Consecutive, Total: pz.Total}); err != nil {
		return err
	}
	_, err = s.cancelPending(tx, port.ApprovalFilter{Session: a.Session, Agent: a.Agent, Connector: a.Connector}, port.ReasonPaused, nil)
	return err
}

// CancelApproval is A7 by an approver or a workspace admin.
func (s *Service) CancelApproval(ctx context.Context, p port.Principal, id port.ApprovalID) (port.Approval, error) {
	var out port.Approval
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return out, err
	}
	keys, err := s.approverKeys(ctx, p)
	if err != nil {
		return out, err
	}
	err = s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "CancelApproval", func(tx port.Tx) error {
		a, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		named := slices.ContainsFunc(a.Approvers, func(x string) bool { return slices.Contains(keys, x) })
		if !named && s.authorize(ctx, p, authz.ApprovalCancel, authz.Workspace(string(a.Tenant), string(a.Workspace))) != nil {
			return port.Errorf(port.CodeNotAnApprover, "%s is not an approver of %s", actor(p), id)
		}
		out, err = s.cancelOne(tx, a, reasonCancelledByApprover)
		return err
	})
	return out, err
}

func (s *Service) cancelOne(tx port.Tx, a port.Approval, reason port.Code) (port.Approval, error) {
	c, err := approval.Cancel(a, reason)
	if err != nil {
		return a, err
	}
	if err := tx.UpdateApproval(c, port.ApprovalPending); err != nil {
		return a, err
	}
	return c, s.emit(tx, a.Tenant, string(a.ID), evApprovalCancelled, catalog.ApprovalCancelled{Approval: string(a.ID), Reason: string(reason)})
}

// WatchApprovals returns the approval events of the caller's tenant after a sequence number; a surface turns it into a stream.
func (s *Service) WatchApprovals(ctx context.Context, p port.Principal, after int64, limit int) ([]port.Event, error) {
	if err := s.authorize(ctx, p, authz.ApprovalRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Event
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		evs, err := tx.EventsAfter(port.SourceOf(p.Claims.Tenant), after, 0)
		for _, e := range evs {
			if strings.HasPrefix(e.Type, "connector.approval.") && (limit <= 0 || len(out) < limit) {
				out = append(out, e)
			}
		}
		return err
	})
	return out, err
}

// ExplainAccess returns the evaluation of one function for one subject: the mode, the step and reason, the matched rows and each layer (§7.1).
func (s *Service) ExplainAccess(ctx context.Context, p port.Principal, sub port.Subject, connector port.ConnectorID, function string) (evaluate.Decision, error) {
	var out evaluate.Decision
	c, err := s.connectorOf(ctx, p, connector, authz.ConnectorManage)
	if err != nil {
		return out, err
	}
	g, err := s.groups(ctx, c.Tenant, evaluate.People(sub)...)
	if err != nil {
		return out, err
	}
	sub.Tenant, sub.Workspace = c.Tenant, c.Workspace
	err = s.d.Store.Read(ctx, port.Scope{Tenant: c.Tenant}, func(tx port.Tx) error {
		f, err := s.facts(tx, sub, connector, g)
		if err != nil {
			return err
		}
		out = evaluate.Evaluate(f, evaluate.Request{Subject: sub, Function: function, Now: s.now()})
		return nil
	})
	return out, err
}

// GetCall returns the audit row of a forwarded call; arguments are references and hashes only (§13.1 getCall).
func (s *Service) GetCall(ctx context.Context, p port.Principal, id port.CallID) (port.Call, error) {
	var out port.Call
	if err := s.authorize(ctx, p, authz.AuditRead, s.tenantResource(p)); err != nil {
		return out, err
	}
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.GetCall(id)
		return err
	})
	return out, err
}
