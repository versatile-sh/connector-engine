package service

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/core/projection"
	"github.com/agents-control/connector-engine/internal/tokenscan"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service/internal/schemacheck"
)

// ServedFunction is one entry of tools/list: the stored definition under its exposed name, with an accepted overlay's text.
type ServedFunction struct {
	Name        string
	Connector   port.ConnectorID
	Function    string
	Definition  json.RawMessage
	Mode        port.Mode
	OverlayHash port.Hash
}

// FunctionList is what ListFunctions answers: tools, skills, connector items, and whether approval_status and approval_cancel are listed too.
type FunctionList struct {
	Functions     []ServedFunction
	Skills        []ServedSkill
	Items         []ServedItem
	ApprovalTools bool
	TTL           time.Duration
	Exposure      port.Exposure
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
	CodeOutcomeUnknown   port.Code = port.CodeOutcomeUnknown
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

// CallRequest is one tools/call, a skill load when Skill is set and Name is the skill's alias, or a prompt get or resource read when Item names its kind and Name its agent-facing name or URI, with the projection paths.
type CallRequest struct {
	Name      string
	Arguments json.RawMessage
	Fields    []string
	Skill     bool
	Item      string
}

func subjectOf(c port.Claims) port.Subject {
	return port.Subject{Tenant: c.Tenant, Workspace: c.Workspace, Agent: c.Agent, AgentOwner: c.AgentOwner, OnBehalfOf: c.OnBehalfOf, Session: c.Session, Caller: c.Caller}
}

func (s *Service) agentSubject(ctx context.Context, p port.Principal) (port.Subject, error) {
	if (p.Kind != port.PrincipalAgent && p.Kind != port.PrincipalView) || p.Claims.Agent == "" {
		return port.Subject{}, port.Errorf(port.CodePermissionDenied, "the agent endpoint takes an agent principal")
	}
	return subjectOf(p.Claims), s.TenantActive(ctx, p.Claims.Tenant)
}

type target struct {
	Conn     port.Connector
	Fn       port.Function
	Bind     port.Binding
	Manifest port.Manifest
	Args     json.RawMessage
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

func (s *Service) resolve(ctx context.Context, sub port.Subject, req CallRequest) (target, bool, error) {
	var t target
	found := false
	name := req.Name
	err := s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) error {
		conns, err := s.boundConnectors(tx, sub)
		if err != nil {
			return err
		}
		if req.Skill || req.Item != "" {
			if req.Skill {
				t.Conn, t.Fn, found, err = s.resolveSkill(tx, conns, name)
			} else {
				t, found, err = s.resolveItem(tx, conns, req.Item, name)
			}
			if err != nil || !found {
				return err
			}
			if t.Bind, err = tx.GetBinding(t.Conn.ID, sub.Agent); err != nil {
				return err
			}
			lv, err := tx.GetVersion(t.Conn.Listing)
			t.Manifest = lv.Manifest
			return err
		}
		aliases := map[port.ConnectorID]string{}
		var entries []names.Entry
		for _, c := range conns {
			if isSkill, err := s.isSkill(tx, c); err != nil {
				return err
			} else if isSkill {
				continue
			}
			aliases[c.ID] = c.Alias
			fns, err := tx.ListFunctions(c.ID)
			if err != nil {
				return err
			}
			for _, f := range fns {
				if port.ItemKind(f.Name) != "" {
					continue
				}
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
	sub, err := s.agentSubject(ctx, p)
	if err != nil {
		return FunctionList{}, err
	}
	return s.functionsFor(ctx, sub)
}

// ListAgentFunctions is what an agent would be listed, for an agent configurer to read.
func (s *Service) ListAgentFunctions(ctx context.Context, p port.Principal, sub port.Subject) (FunctionList, error) {
	if err := s.authorize(ctx, p, authz.BindingWrite, s.tenantResource(p)); err != nil {
		return FunctionList{}, err
	}
	sub.Tenant = p.Claims.Tenant
	out, err := s.functionsFor(ctx, sub)
	if err != nil {
		return out, err
	}
	out.Exposure, err = s.RefreshExposure(ctx, sub.Tenant, sub.Agent)
	return out, err
}

type boundFunction struct {
	f port.Function
	o *port.Overlay
}

type boundConnector struct {
	c     port.Connector
	skill bool
	fns   map[string]boundFunction
}

func (s *Service) loadBound(tx port.Tx, sub port.Subject) ([]boundConnector, error) {
	conns, err := s.boundConnectors(tx, sub)
	if err != nil {
		return nil, err
	}
	out := make([]boundConnector, 0, len(conns))
	for _, c := range conns {
		b := boundConnector{c: c, fns: map[string]boundFunction{}}
		if b.skill, err = s.isSkill(tx, c); err != nil {
			return nil, err
		}
		fns, err := tx.ListFunctions(c.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range fns {
			bf := boundFunction{f: f}
			if port.ItemKind(f.Name) == "" && !b.skill {
				if bf.o, err = s.servedOverlay(tx, c, f); err != nil {
					return nil, err
				}
			}
			b.fns[f.Name] = bf
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Service) functionsFor(ctx context.Context, sub port.Subject) (FunctionList, error) {
	out := FunctionList{TTL: s.cfg.ListTTL}
	var all []boundConnector
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: sub.Tenant}, func(tx port.Tx) (err error) {
		all, err = s.loadBound(tx, sub)
		return err
	}); err != nil {
		return out, err
	}
	bound := map[string]bool{}
	for _, b := range all {
		decisions, err := s.d.Authorizer.Search(ctx, sub, b.c.ID)
		if err != nil {
			return out, err
		}
		if !b.skill {
			bound[b.c.Listing.Listing.Name] = true
		}
		for _, d := range decisions {
			if bf, ok := b.fns[d.Function]; ok && d.Decision.Mode != port.ModeDeny {
				s.serve(&out, b, bf, d.Decision.Mode)
			}
		}
	}
	markUnbound(out.Skills, bound)
	slices.SortFunc(out.Functions, func(a, b ServedFunction) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(out.Items, func(a, b ServedItem) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

func (s *Service) serve(out *FunctionList, b boundConnector, bf boundFunction, mode port.Mode) {
	c, f := b.c, bf.f
	if port.ItemKind(f.Name) != "" {
		s.serveItem(out, c, f, mode)
		return
	}
	out.ApprovalTools = out.ApprovalTools || mode == port.ModeAsk || mode == port.ModeAskOnce
	if b.skill {
		out.Skills = append(out.Skills, servedSkill(c, f, mode))
		return
	}
	name, err := names.Exposed(c.Alias, f.Name, s.cfg.Separator, s.cfg.NameMax)
	if err != nil {
		return
	}
	sf := ServedFunction{Name: name, Connector: c.ID, Function: f.Name, Definition: served(name, f, bf.o), Mode: mode}
	if bf.o != nil {
		sf.OverlayHash = bf.o.OverlayHash
	}
	out.Functions = append(out.Functions, sf)
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
	if s.relistDue(t.Conn) {
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
	t, found, err := s.resolve(ctx, sub, req)
	if err != nil || !found {
		return unknown(port.ReasonUnknownFunction), err
	}
	if t.Args != nil {
		args = t.Args
	}
	query := queryOf(sub, t, args)
	dec, err := s.d.Authorizer.Evaluate(ctx, query)
	if err != nil {
		return Outcome{}, err
	}
	if s.needRelist(t, dec.Mode) {
		if _, err := s.Relist(ctx, sub.Tenant, t.Conn.ID); err != nil {
			return Outcome{}, err
		}
		if t, _, err = s.resolve(ctx, sub, req); err != nil {
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
		if bad.Code == port.CodeTokenInArguments {
			s.denied(ctx, sub, t.Conn.ID, t.Fn.Name, bad.Code)
		}
		return *bad, nil
	}
	in := admitInput{Sub: sub, Exp: p.Claims.ExpiresAt, T: t, Args: args, Fields: req.Fields, Dec: dec}
	if dec.Mode != port.ModeAllow || slices.Contains(dec.Notes, port.NoteSessionGrant) {
		sealed, err := s.d.Secrets.Seal(ctx, sub.Tenant, args)
		if err != nil {
			return Outcome{}, err
		}
		in.Sealed = &sealed
	}
	if in.Groups, err = s.groups(ctx, sub.Tenant, evaluate.People(sub)...); err != nil {
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
		return s.answer(ctx, *adm.Deliver)
	case adm.Forward != nil:
		return s.forwardDirect(ctx, in, *adm.Forward)
	}
	return adm.Out, nil
}

func queryOf(sub port.Subject, t target, args json.RawMessage) port.Query {
	return port.Query{Tenant: sub.Tenant, Workspace: t.Conn.Workspace, Agent: sub.Agent, AgentOwner: sub.AgentOwner, OnBehalfOf: sub.OnBehalfOf, Caller: sub.Caller,
		Session: sub.Session, Connector: t.Conn.ID, Function: t.Fn.Name, Class: t.Fn.Class, Arguments: args}
}

func (s *Service) checkArguments(p port.Principal, t target, req CallRequest, args json.RawMessage) *Outcome {
	fail := func(o Outcome) *Outcome { return &o }
	if s.cfg.ScanForToken && tokenscan.Contains(args, p.Token) {
		return fail(toolError(port.CodeTokenInArguments, "the arguments carry the bearer token of this request"))
	}
	if err := schemacheck.Validate(schemaOf(t.Fn, "inputSchema"), args); err != nil {
		return fail(toolError(port.CodeInvalidArgument, "%v", err))
	}
	if len(req.Fields) == 0 {
		return nil
	}
	if !t.Fn.Projection && !s.cfg.ProjectionDefault {
		return fail(toolError(port.CodeInvalidPath, "projection is off for %s", t.Fn.Name))
	}
	if err := projection.Validate(schemaOf(t.Fn, "outputSchema"), req.Fields, s.cfg.ProjectionMaxPaths); err != nil {
		return fail(toolError(port.CodeInvalidPath, "%v", err))
	}
	return nil
}
