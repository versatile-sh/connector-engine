package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/core/projection"
	"github.com/agents-control/connector-engine/port"
)

func evaluatePeople(s port.Subject) []port.PersonID { return evaluate.People(s) }

func catalogDenied(sub port.Subject, connector port.ConnectorID, function string, reason port.Code) catalog.CallDenied {
	return catalog.CallDenied{Agent: string(sub.Agent), Connector: string(connector), Function: function, Reason: string(reason)}
}

type admitInput struct {
	Sub    port.Subject
	T      target
	Args   json.RawMessage
	Fields []string
	Dec    port.AccessDecision
	Sealed *port.Sealed
	Groups map[port.PersonID][]port.GroupID
}

type forwardPlan struct {
	Call       port.Call
	Credential *port.Credential
	Conn       port.Connector
}

type admission struct {
	Out     Outcome
	Deliver *port.Approval
	Forward *forwardPlan
}

func (s *Service) pending(a port.Approval) Outcome {
	return Outcome{Status: OutcomePending, Approval: &a, PollAfter: s.cfg.PollAfter}
}

func (s *Service) admit(tx port.Tx, in admitInput) (admission, error) {
	sub := in.Sub
	f, err := s.facts(tx, sub, in.T.Conn.ID, in.Groups)
	if err != nil {
		return admission{}, err
	}
	d := evaluate.Evaluate(f, evaluate.Request{Subject: sub, Function: in.T.Fn.Name, Now: s.now(), Arguments: in.Args, Call: true})
	if d.Mode == port.ModeDeny {
		return admission{Out: unknown(d.Reason)}, nil
	}
	conn, fn := *f.Connector, f.Functions[in.T.Fn.Name]
	mode := d.Mode.Stricter(in.Dec.Mode)
	proto := port.Approval{Tenant: sub.Tenant, Agent: sub.Agent, Connector: conn.ID, Function: fn.Name, DefinitionHash: fn.DefinitionHash}
	hash, err := pinning.CallHash(proto, in.Args)
	if err != nil {
		return admission{}, err
	}
	records, err := tx.ListApprovals(port.ApprovalFilter{Session: sub.Session, CallHash: hash})
	if err != nil {
		return admission{}, err
	}
	latest, ok, err := tx.LatestCall(sub.Session, hash)
	if err != nil {
		return admission{}, err
	}
	var lp *port.Call
	if ok {
		lp = &latest
	}
	v := approval.Lookup(records, lp)
	switch v.Rule {
	case approval.Attach:
		return admission{Out: s.pending(v.Record)}, nil
	case approval.Answer:
		return admission{Deliver: &v.Record}, nil
	case approval.HoldAfterUnknown:
		if mode == port.ModeAllow && slices.Contains(d.Notes, port.NoteSessionGrant) {
			mode = port.ModeAskOnce
		}
	}
	if mode == port.ModeAllow {
		return s.admitForward(tx, in, d, conn, fn, hash)
	}
	return s.admitHold(tx, in, conn, fn, hash, mode)
}

func (s *Service) admitForward(tx port.Tx, in admitInput, d evaluate.Decision, conn port.Connector, fn port.Function, hash port.Hash) (admission, error) {
	sub := in.Sub
	bind, err := tx.GetBinding(conn.ID, sub.Agent)
	if err != nil {
		return admission{}, err
	}
	res, err := s.resolveCredential(tx, conn, bind, sub.OnBehalfOf, fn.Name, in.T.Manifest)
	if err != nil {
		return admission{}, err
	}
	if res.Code != "" {
		return admission{Out: Outcome{Status: OutcomeError, Code: res.Code, Detail: "no usable credential for this call", Scopes: res.Scopes, ConnectAddress: res.Connect}}, nil
	}
	if err := tx.ReserveGrants(d.Matched); err != nil {
		return admission{}, err
	}
	call, err := s.startCall(tx, conn, fn, sub, hash, "", res.Credential)
	return admission{Forward: &forwardPlan{Call: call, Credential: res.Credential, Conn: conn}}, err
}

func (s *Service) startCall(tx port.Tx, conn port.Connector, fn port.Function, sub port.Subject, hash port.Hash, approvalID port.ApprovalID, cred *port.Credential) (port.Call, error) {
	timeout := conn.CallTimeout
	if timeout == 0 {
		timeout = s.cfg.CallTimeout
	}
	call := port.Call{ID: port.CallID(s.d.NewID()), Tenant: conn.Tenant, Workspace: conn.Workspace, Agent: sub.Agent, Session: sub.Session, OnBehalfOf: sub.OnBehalfOf, Connector: conn.ID,
		Function: fn.Name, Class: fn.Class, CallHash: hash, Approval: approvalID, State: port.CallStarted, StartedAt: s.now(), Deadline: s.now().Add(timeout)}
	if cred != nil {
		call.Credential = cred.ID
	}
	o, err := s.servedOverlay(tx, conn, fn)
	if err != nil {
		return call, err
	}
	if o != nil {
		call.OverlayHash = o.OverlayHash
	}
	if err := tx.CreateCall(call); err != nil {
		return call, err
	}
	return call, s.record(tx, conn.Tenant, string(sub.Agent), string(call.ID), evCallStarted, catalog.CallStarted{Call: string(call.ID), Agent: string(sub.Agent), Session: string(sub.Session),
		Connector: string(conn.ID), Function: fn.Name, Class: string(fn.Class), Approval: string(approvalID), Credential: string(call.Credential), OverlayHash: string(call.OverlayHash)})
}

func (s *Service) admitHold(tx port.Tx, in admitInput, conn port.Connector, fn port.Function, hash port.Hash, mode port.Mode) (admission, error) {
	sub := in.Sub
	if in.Sealed == nil {
		return admission{}, port.Errorf(port.CodeConflict, "the decision changed while the call was being checked")
	}
	bind, err := tx.GetBinding(conn.ID, sub.Agent)
	if err != nil {
		return admission{}, err
	}
	a := port.Approval{ID: port.ApprovalID(s.d.NewID()), Tenant: sub.Tenant, Workspace: conn.Workspace, Agent: sub.Agent, Session: sub.Session, OnBehalfOf: sub.OnBehalfOf,
		Connector: conn.ID, Function: fn.Name, DefinitionHash: fn.DefinitionHash, Class: fn.Class, Arguments: in.Sealed, CallHash: hash, Fields: in.Fields, Mode: mode,
		Approvers: approval.Approvers(bind, sub.OnBehalfOf)}
	if timeout := cmpOr(bind.ApprovalTimeout, conn.ApprovalTimeout); timeout > 0 {
		a.ExpiresAt = s.now().Add(timeout)
	}
	open, err := tx.ListApprovals(port.ApprovalFilter{States: []port.ApprovalState{port.ApprovalPending}, Session: sub.Session, Agent: sub.Agent, Connector: conn.ID})
	if err != nil {
		return admission{}, err
	}
	a, err = approval.Open(a, len(open), s.cfg.Approval, s.now())
	if port.IsCode(err, port.CodeNoApprover) || port.IsCode(err, port.CodeTooManyPending) {
		code := port.CodeOf(err)
		if err := s.record(tx, sub.Tenant, string(sub.Agent), fn.Name, evCallDenied, catalogDenied(sub, conn.ID, fn.Name, code)); err != nil {
			return admission{}, err
		}
		return admission{Out: toolError(code, "%v", err.(*port.Error).Detail)}, nil
	}
	if err != nil {
		return admission{}, err
	}
	if err := tx.CreateApproval(a); err != nil {
		return admission{}, err
	}
	err = s.record(tx, sub.Tenant, string(sub.Agent), string(a.ID), evApprovalRequested, catalog.ApprovalRequested{Approval: string(a.ID), Approvers: a.Approvers, Connector: string(conn.ID),
		Function: fn.Name, Class: string(fn.Class), Agent: string(sub.Agent), Session: string(sub.Session), ExpiresAt: a.ExpiresAt, CallHash: string(hash)})
	return admission{Out: s.pending(a)}, err
}

func cmpOr(a, b time.Duration) time.Duration {
	if a != 0 {
		return a
	}
	return b
}

type invocation struct {
	Conn       port.Connector
	Manifest   port.Manifest
	Function   string
	Credential *port.Credential
	Args       json.RawMessage
	Fields     []string
	Deadline   time.Time
}

type invoked struct {
	Result   port.Result
	Err      error
	Unknown  bool
	Findings map[string]int
	Bytes    int
}

func textBlock(raw json.RawMessage) json.RawMessage {
	b, _ := json.Marshal([]map[string]string{{"type": "text", "text": string(raw)}})
	return b
}

func (s *Service) invoke(ctx context.Context, in invocation) invoked {
	var cred port.CredentialID
	var owner port.OwnerKind
	if in.Credential != nil {
		cred, owner = in.Credential.ID, in.Credential.OwnerKind
	}
	h, err := s.handle(ctx, in.Conn, in.Manifest, cred, owner)
	if err != nil {
		return invoked{Err: err}
	}
	defer s.release(ctx, h)
	cctx, cancel := context.WithDeadline(ctx, in.Deadline)
	defer cancel()
	res, err := s.d.Runtime.Call(cctx, h.h, in.Function, in.Args, in.Deadline)
	if port.IsCode(err, port.CodeUnavailable) {
		s.drop(h)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), port.IsCode(err, port.CodeOutcomeUnknown):
		return invoked{Unknown: true}
	case err != nil:
		return invoked{Err: err}
	}
	if len(in.Fields) > 0 && len(res.Structured) > 0 && !res.IsError {
		pr, err := projection.Project(res.Structured, in.Fields)
		if err != nil {
			return invoked{Err: err}
		}
		res.Structured, res.Content = pr, textBlock(pr)
	}
	out := invoked{Result: res}
	if s.d.Filter != nil {
		filtered, counts, err := s.d.Filter.Filter(ctx, res)
		if err != nil {
			return invoked{Err: port.Errorf(port.CodeUnavailable, "the result filter failed: %v", err)}
		}
		out.Result, out.Findings = filtered, counts
	}
	out.Bytes = len(out.Result.Content) + len(out.Result.Structured)
	return out
}

func (s *Service) forwardDirect(ctx context.Context, in admitInput, plan forwardPlan) (Outcome, error) {
	inv := s.invoke(ctx, invocation{Conn: plan.Conn, Manifest: in.T.Manifest, Function: in.T.Fn.Name, Credential: plan.Credential, Args: in.Args, Fields: in.Fields, Deadline: plan.Call.Deadline})
	return s.settle(context.WithoutCancel(ctx), plan.Call, inv, nil)
}

func (s *Service) settle(ctx context.Context, call port.Call, inv invoked, a *port.Approval) (Outcome, error) {
	if inv.Unknown {
		return toolError(CodeOutcomeUnknown, "the call may have run; it was not retried"), nil
	}
	var sealed *port.Sealed
	if a != nil {
		payload := []byte(nil)
		if inv.Err != nil {
			payload = []byte(inv.Err.Error())
		} else {
			payload, _ = json.Marshal(inv.Result)
		}
		sl, err := s.d.Secrets.Seal(ctx, call.Tenant, payload)
		if err != nil {
			return Outcome{}, err
		}
		sealed = &sl
	}
	now := s.now()
	late := false
	err := s.write(ctx, port.Scope{Tenant: call.Tenant}, "Settle", func(tx port.Tx) error {
		fin := call
		fin.FinishedAt, fin.ResultBytes, fin.IsError, fin.Findings = now, inv.Bytes, inv.Result.IsError || inv.Err != nil, inv.Findings
		fin.State = port.CallFinished
		if inv.Err != nil {
			fin.State, fin.Reason = port.CallFailed, inv.Err.Error()
		}
		if err := tx.UpdateCall(fin, port.CallStarted); err != nil {
			if !port.IsCode(err, port.CodeConflict) {
				return err
			}
			late = true
			cur, gerr := tx.GetCall(call.ID)
			if gerr != nil {
				return gerr
			}
			cur.LateAnswer = true
			return tx.UpdateCall(cur, cur.State)
		}
		var err error
		if inv.Err != nil {
			err = s.record(tx, call.Tenant, "system", string(call.ID), evCallFailed, catalog.CallFailed{Call: string(call.ID), Reason: fin.Reason})
		} else {
			err = s.record(tx, call.Tenant, "system", string(call.ID), evCallFinished, catalog.CallFinished{Call: string(call.ID), Agent: string(call.Agent), OnBehalfOf: string(call.OnBehalfOf),
				Connector: string(call.Connector), Function: call.Function, DurationMS: now.Sub(call.StartedAt).Milliseconds(), ResultBytes: int64(inv.Bytes), IsError: fin.IsError})
		}
		if err != nil || a == nil {
			return err
		}
		cur, err := tx.GetApproval(a.ID)
		if err != nil {
			return err
		}
		done, err := approval.Finish(cur, sealed, inv.Err != nil, s.cfg.Approval, now)
		if err != nil {
			return err
		}
		return tx.UpdateApproval(done, port.ApprovalExecuting)
	})
	switch {
	case err != nil:
		return Outcome{}, err
	case late:
		return toolError(CodeOutcomeUnknown, "the call may have run; it was not retried"), nil
	case inv.Err != nil:
		return toolError(CodeCallFailed, "%v", inv.Err), nil
	}
	res := inv.Result
	return Outcome{Status: OutcomeResult, Result: &res, Findings: inv.Findings}, nil
}

func (s *Service) answer(ctx context.Context, a port.Approval) (Outcome, error) {
	open := func() ([]byte, error) { return s.d.Secrets.Open(ctx, a.Tenant, *a.ResultRef) }
	switch a.State {
	case port.ApprovalPending, port.ApprovalApproved, port.ApprovalExecuting:
		return s.pending(a), nil
	case port.ApprovalExecuted, port.ApprovalFailed:
		if a.ResultRef == nil {
			return Outcome{Status: OutcomeError, Code: CodeResultNotKept, Detail: "the result is no longer kept", Approval: &a}, nil
		}
		plain, err := open()
		if err != nil {
			return Outcome{}, err
		}
		if a.State == port.ApprovalFailed {
			return Outcome{Status: OutcomeError, Code: CodeCallFailed, Detail: string(plain), Approval: &a}, nil
		}
		var res port.Result
		if err := json.Unmarshal(plain, &res); err != nil {
			return Outcome{}, err
		}
		return Outcome{Status: OutcomeResult, Result: &res, Approval: &a}, nil
	case port.ApprovalOutcomeUnknown:
		return Outcome{Status: OutcomeError, Code: CodeOutcomeUnknown, Detail: "the call may have run; it was not retried", Approval: &a}, nil
	case port.ApprovalDenied:
		return Outcome{Status: OutcomeError, Code: CodeApprovalDenied, Detail: a.DecisionNote, Approval: &a}, nil
	case port.ApprovalExpired:
		return Outcome{Status: OutcomeError, Code: port.CodeApprovalExpired, Detail: "the approval expired", Approval: &a}, nil
	}
	return Outcome{Status: OutcomeError, Code: port.Code(a.Reason), Detail: "the approval was cancelled", Approval: &a}, nil
}

func (s *Service) ownApproval(ctx context.Context, p port.Principal, id port.ApprovalID) (port.Approval, error) {
	sub, err := s.agentSubject(ctx, p)
	if err != nil {
		return port.Approval{}, err
	}
	var a port.Approval
	err = s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) (err error) {
		a, err = tx.GetApproval(id)
		return err
	})
	if err == nil && (a.Agent != sub.Agent || a.Session != sub.Session) {
		err = port.Errorf(port.CodeNotFound, "approval %s not found", id)
	}
	return a, err
}

// ApprovalStatus is approval_status: the answer of one record of the calling agent and session; it does not mark the record delivered.
func (s *Service) ApprovalStatus(ctx context.Context, p port.Principal, id port.ApprovalID) (Outcome, error) {
	a, err := s.ownApproval(ctx, p, id)
	if err != nil {
		return Outcome{}, err
	}
	return s.answer(ctx, a)
}

// MarkDelivered is A14: the surface calls it once the terminal answer of a record of the calling agent and session reached the agent.
func (s *Service) MarkDelivered(ctx context.Context, p port.Principal, id port.ApprovalID) error {
	a, err := s.ownApproval(ctx, p, id)
	if err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: a.Tenant}, "Deliver", func(tx port.Tx) error {
		cur, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		delivered, err := approval.Deliver(cur, s.now())
		if err != nil {
			return nil
		}
		return tx.UpdateApproval(delivered, cur.State)
	})
}

// ApprovalCancelByAgent is approval_cancel: A7 for a pending record of the calling agent and session.
func (s *Service) ApprovalCancelByAgent(ctx context.Context, p port.Principal, id port.ApprovalID) (Outcome, error) {
	a, err := s.ownApproval(ctx, p, id)
	if err != nil {
		return Outcome{}, err
	}
	err = s.write(ctx, port.Scope{Tenant: a.Tenant}, "ApprovalCancelByAgent", func(tx port.Tx) error {
		cur, err := tx.GetApproval(id)
		if err != nil {
			return err
		}
		a, err = s.cancelOne(tx, cur, reasonCancelledAgent)
		return err
	})
	if err != nil && !port.IsCode(err, port.CodeInvalidTransition) {
		return Outcome{}, err
	}
	return s.answer(ctx, a)
}
