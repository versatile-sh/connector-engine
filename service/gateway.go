package service

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/core/projection"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service/internal/schemacheck"
)

// ServedFunction is one entry of tools/list: the stored definition under its exposed name, with an accepted overlay's text (§9, §10.2).
type ServedFunction struct {
	Name        string
	Connector   port.ConnectorID
	Function    string
	Definition  json.RawMessage
	Mode        port.Mode
	OverlayHash port.Hash
}

// FunctionList is what ListFunctions answers; ApprovalTools says approval_status and approval_cancel are listed too.
type FunctionList struct {
	Functions     []ServedFunction
	ApprovalTools bool
	TTL           time.Duration
}

// OutcomeStatus is what a call came to.
type OutcomeStatus string

// Statuses of an Outcome.
const (
	OutcomeResult      OutcomeStatus = "result"
	OutcomePending     OutcomeStatus = "pending_approval"
	OutcomeError       OutcomeStatus = "error"
	OutcomeUnknownTool OutcomeStatus = "unknown_tool"
)

// Codes of tool errors that are not in package port.
const (
	CodeApprovalDenied   port.Code = "approval_denied"
	CodeOutcomeUnknown   port.Code = "outcome_unknown"
	CodeResultNotKept    port.Code = "result_not_kept"
	CodeCallFailed       port.Code = "call_failed"
	reasonCancelledAgent port.Code = "cancelled_by_agent"
)

// Outcome is the neutral answer of a call: a result, a pending approval, a tool error or an unknown tool; the gateway maps it to MCP.
type Outcome struct {
	Status         OutcomeStatus
	Result         *port.Result
	Code           port.Code
	Detail         string
	Reason         port.Code
	Approval       *port.Approval
	PollAfter      time.Duration
	Scopes         []string
	ConnectAddress bool
	Findings       map[string]int
}

// CallRequest is one tools/call: the exposed name, the arguments and the projection paths of §10.5.
type CallRequest struct {
	Name      string
	Arguments json.RawMessage
	Fields    []string
}

func subjectOf(c port.Claims) port.Subject {
	return port.Subject{Tenant: c.Tenant, Workspace: c.Workspace, Agent: c.Agent, AgentOwner: c.AgentOwner, OnBehalfOf: c.OnBehalfOf, Session: c.Session, Caller: c.Caller}
}

func (s *Service) agentSubject(ctx context.Context, p port.Principal) (port.Subject, error) {
	if (p.Kind != port.PrincipalAgent && p.Kind != port.PrincipalView) || p.Claims.Agent == "" {
		return port.Subject{}, port.Errorf(port.CodePermissionDenied, "the agent endpoint takes an agent principal")
	}
	return subjectOf(p.Claims), s.activeTenant(ctx, p.Claims.Tenant)
}

type target struct {
	Conn     port.Connector
	Fn       port.Function
	Bind     port.Binding
	Manifest port.Manifest
}

func (s *Service) boundConnectors(tx port.Tx, sub port.Subject) ([]port.Connector, error) {
	binds, err := tx.ListBindings(port.BindingFilter{Agent: sub.Agent})
	if err != nil {
		return nil, err
	}
	var out []port.Connector
	for _, b := range binds {
		c, err := tx.GetConnector(b.Connector)
		if err != nil {
			return nil, err
		}
		if c.RemovedAt.IsZero() && (sub.Workspace == "" || c.Workspace == sub.Workspace) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Service) resolve(ctx context.Context, sub port.Subject, name string) (target, bool, error) {
	var t target
	found := false
	err := s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) error {
		conns, err := s.boundConnectors(tx, sub)
		if err != nil {
			return err
		}
		aliases := map[port.ConnectorID]string{}
		var entries []names.Entry
		for _, c := range conns {
			aliases[c.ID] = c.Alias
			fns, err := tx.ListFunctions(c.ID)
			if err != nil {
				return err
			}
			for _, f := range fns {
				if _, err := names.Exposed(c.Alias, f.Name, s.cfg.Separator, s.cfg.NameMax); err == nil {
					entries = append(entries, names.Entry{Connector: c.ID, Function: f.Name})
				}
			}
		}
		table, err := names.Build(s.cfg.Separator, s.cfg.NameMax, aliases, entries)
		if err != nil {
			return err
		}
		e, ok := table.Resolve(name)
		if !ok {
			return nil
		}
		found = true
		if t.Conn, err = tx.GetConnector(e.Connector); err != nil {
			return err
		}
		if t.Fn, err = tx.GetFunction(e.Connector, e.Function); err != nil {
			return err
		}
		if t.Bind, err = tx.GetBinding(e.Connector, sub.Agent); err != nil {
			return err
		}
		lv, err := tx.GetVersion(t.Conn.Listing)
		t.Manifest = lv.Manifest
		return err
	})
	return t, found, err
}

func served(name string, f port.Function, o *port.Overlay) json.RawMessage {
	var def map[string]json.RawMessage
	if json.Unmarshal(f.Definition, &def) != nil {
		def = map[string]json.RawMessage{}
	}
	def["name"], _ = json.Marshal(name)
	if o != nil {
		if o.Title != "" {
			def["title"], _ = json.Marshal(o.Title)
		}
		def["description"], _ = json.Marshal(o.Description)
	}
	out, _ := json.Marshal(def)
	return out
}

// ListFunctions is tools/list L3 and L4: Authorizer.Search for each bound connector, then the stored definitions of every function that is not denied.
func (s *Service) ListFunctions(ctx context.Context, p port.Principal) (FunctionList, error) {
	out := FunctionList{TTL: s.cfg.ListTTL}
	sub, err := s.agentSubject(ctx, p)
	if err != nil {
		return out, err
	}
	var conns []port.Connector
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) (err error) {
		conns, err = s.boundConnectors(tx, sub)
		return err
	}); err != nil {
		return out, err
	}
	modes := map[port.ConnectorID]map[string]port.Mode{}
	for _, c := range conns {
		decisions, err := s.d.Authorizer.Search(ctx, sub, c.ID)
		if err != nil {
			return out, err
		}
		modes[c.ID] = map[string]port.Mode{}
		for _, d := range decisions {
			if d.Decision.Mode != port.ModeDeny {
				modes[c.ID][d.Function] = d.Decision.Mode
			}
		}
	}
	err = s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) error {
		for _, c := range conns {
			for fn, mode := range modes[c.ID] {
				f, err := tx.GetFunction(c.ID, fn)
				if err != nil {
					return err
				}
				name, err := names.Exposed(c.Alias, fn, s.cfg.Separator, s.cfg.NameMax)
				if err != nil {
					continue
				}
				o, err := s.servedOverlay(tx, c, f)
				if err != nil {
					return err
				}
				sf := ServedFunction{Name: name, Connector: c.ID, Function: fn, Definition: served(name, f, o), Mode: mode}
				if o != nil {
					sf.OverlayHash = o.OverlayHash
				}
				out.Functions = append(out.Functions, sf)
				out.ApprovalTools = out.ApprovalTools || mode == port.ModeAsk || mode == port.ModeAskOnce
			}
		}
		return nil
	})
	slices.SortFunc(out.Functions, func(a, b ServedFunction) int { return compare(a.Name, b.Name) })
	return out, err
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func schemaOf(f port.Function, key string) json.RawMessage {
	var d map[string]json.RawMessage
	_ = json.Unmarshal(f.Definition, &d)
	return d[key]
}

func (s *Service) needRelist(t target, mode port.Mode) bool {
	if mode != port.ModeAllow {
		return false
	}
	if s.now().Sub(t.Conn.LastRelistAt) >= s.cfg.RelistMax {
		return true
	}
	return s.cfg.RelistBeforeCall == "all" || (s.cfg.RelistBeforeCall == "write" && t.Fn.Class.Effective() != port.ClassRead)
}

func (s *Service) denied(ctx context.Context, sub port.Subject, connector port.ConnectorID, function string, reason port.Code) {
	_ = s.write(ctx, port.Scope{Tenant: sub.Tenant}, "CallDenied", func(tx port.Tx) error {
		return s.record(tx, sub.Tenant, string(sub.Agent), function, evCallDenied, catalogDenied(sub, connector, function, reason))
	})
}

func unknown(reason port.Code) Outcome { return Outcome{Status: OutcomeUnknownTool, Reason: reason} }

func toolError(code port.Code, format string, args ...any) Outcome {
	return Outcome{Status: OutcomeError, Code: code, Detail: port.Errorf(code, format, args...).(*port.Error).Detail}
}

// Call is tools/call C2 to C12 without transport: resolve, evaluate, check, look up a repeat, hold or forward, and return a neutral outcome.
func (s *Service) Call(ctx context.Context, p port.Principal, req CallRequest) (Outcome, error) {
	sub, err := s.agentSubject(ctx, p)
	if err != nil {
		return Outcome{}, err
	}
	args := req.Arguments
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	for attempt := 0; ; attempt++ {
		out, err := s.callOnce(ctx, p, sub, req, args)
		if (port.IsCode(err, port.CodeGrantExhausted) || port.IsCode(err, port.CodeConflict)) && attempt == 0 {
			continue
		}
		if port.IsCode(err, port.CodeGrantExhausted) {
			return toolError(port.CodeGrantExhausted, "a use-limited grant was exhausted"), nil
		}
		return out, err
	}
}

func (s *Service) callOnce(ctx context.Context, p port.Principal, sub port.Subject, req CallRequest, args json.RawMessage) (Outcome, error) {
	t, found, err := s.resolve(ctx, sub, req.Name)
	if err != nil || !found {
		return unknown(port.ReasonUnknownFunction), err
	}
	query := port.Query{Tenant: sub.Tenant, Workspace: t.Conn.Workspace, Agent: sub.Agent, AgentOwner: sub.AgentOwner, OnBehalfOf: sub.OnBehalfOf, Caller: sub.Caller,
		Session: sub.Session, Connector: t.Conn.ID, Function: t.Fn.Name, Class: t.Fn.Class, Arguments: args}
	dec, err := s.d.Authorizer.Evaluate(ctx, query)
	if err != nil {
		return Outcome{}, err
	}
	if s.needRelist(t, dec.Mode) {
		if _, err := s.Relist(ctx, sub.Tenant, t.Conn.ID); err != nil {
			return Outcome{}, err
		}
		if t, _, err = s.resolve(ctx, sub, req.Name); err != nil {
			return Outcome{}, err
		}
		query.Class = t.Fn.Class
		if dec, err = s.d.Authorizer.Evaluate(ctx, query); err != nil {
			return Outcome{}, err
		}
	}
	if dec.Mode == port.ModeDeny {
		s.denied(ctx, sub, t.Conn.ID, t.Fn.Name, dec.Reason)
		return unknown(dec.Reason), nil
	}
	if bad := s.checkArguments(p, t, req, args); bad != nil {
		s.denied(ctx, sub, t.Conn.ID, t.Fn.Name, bad.Code)
		return *bad, nil
	}
	in := admitInput{Sub: sub, T: t, Args: args, Fields: req.Fields, Dec: dec}
	if dec.Mode != port.ModeAllow || slices.Contains(dec.Notes, port.NoteSessionGrant) {
		sealed, err := s.d.Secrets.Seal(ctx, sub.Tenant, args)
		if err != nil {
			return Outcome{}, err
		}
		in.Sealed = &sealed
	}
	if in.Groups, err = s.groups(ctx, sub.Tenant, evaluatePeople(sub)...); err != nil {
		return Outcome{}, err
	}
	var adm admission
	if err := s.write(ctx, port.Scope{Tenant: sub.Tenant}, "Call", func(tx port.Tx) (err error) {
		adm, err = s.admit(tx, in)
		return err
	}); err != nil {
		return Outcome{}, err
	}
	switch {
	case adm.Deliver != nil:
		return s.answerAndDeliver(ctx, *adm.Deliver)
	case adm.Forward != nil:
		return s.forwardDirect(ctx, in, *adm.Forward)
	}
	return adm.Out, nil
}

func (s *Service) checkArguments(p port.Principal, t target, req CallRequest, args json.RawMessage) *Outcome {
	var out Outcome
	switch {
	case s.cfg.ScanForToken && p.Token != "" && bytes.Contains(args, []byte(p.Token)):
		out = toolError(port.CodeTokenInArguments, "the arguments carry the bearer token of this request")
	case schemacheck.Validate(schemaOf(t.Fn, "inputSchema"), args) != nil:
		out = toolError(port.CodeInvalidArgument, "%v", schemacheck.Validate(schemaOf(t.Fn, "inputSchema"), args))
	case len(req.Fields) > 0 && !(t.Fn.Projection || s.cfg.ProjectionDefault):
		out = toolError(port.CodeInvalidPath, "projection is off for %s", t.Fn.Name)
	case len(req.Fields) > 0 && projection.Validate(schemaOf(t.Fn, "outputSchema"), req.Fields, s.cfg.ProjectionMaxPaths) != nil:
		out = toolError(port.CodeInvalidPath, "%v", projection.Validate(schemaOf(t.Fn, "outputSchema"), req.Fields, s.cfg.ProjectionMaxPaths))
	default:
		return nil
	}
	return &out
}
