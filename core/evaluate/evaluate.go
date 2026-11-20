// Package evaluate runs the sixteen steps for one function: Evaluate for a call, Search for a list.
package evaluate

import (
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/core/catalog"
	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

// Facts is what the evaluation reads for one agent on one connector; DestructiveMax is ask_once unless it says ask.
type Facts struct {
	Connector      *port.Connector
	Functions      map[string]port.Function
	Bound          bool
	SessionEnded   bool
	PersonRemoved  bool
	Catalog        catalog.View
	Paused         bool
	Grants         []port.Grant
	ForceAsk       bool
	LegC           map[string]bool
	DestructiveMax port.Mode
	External       func(port.Query) port.Mode
}

// Request is one question: the claims of the token, the function and, on tools/call only, the arguments.
type Request struct {
	Subject   port.Subject
	Function  string
	Now       time.Time
	Arguments []byte
	Call      bool
}

// Decision is the evaluation's answer, with the mode each of the tenant, workspace, agent and user layers gave.
type Decision struct {
	port.AccessDecision
	Layers  [4]string `json:"layers"`
	Exposed bool      `json:"exposed,omitempty"`
}

func deny(step int, reason port.Code) Decision {
	return Decision{AccessDecision: port.AccessDecision{Mode: port.ModeDeny, Reason: reason, Step: step}}
}

// People are the persons a request considers at step 2: the owner of the agent and the person acted for.
func People(s port.Subject) []port.PersonID {
	var out []port.PersonID
	for _, p := range []port.PersonID{s.AgentOwner, s.OnBehalfOf} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Evaluate runs steps 1 to 15 for one function; the external point is asked again with the arguments only when r.Call is set.
func Evaluate(f Facts, r Request) Decision {
	c := f.Connector
	switch {
	case c == nil || !c.RemovedAt.IsZero():
		return deny(1, port.ReasonNoConnector)
	case !c.Enabled:
		return deny(1, port.ReasonConnectorDisabled)
	case !f.Bound:
		return deny(1, port.ReasonNoBinding)
	case f.SessionEnded:
		return deny(1, port.ReasonSessionEnded)
	case f.PersonRemoved:
		return deny(1, port.ReasonPersonRemoved)
	}
	ref := c.Listing.Listing
	switch f.Catalog.Status(ref, c.Listing.Version) {
	case port.ListingQuarantined:
		return deny(2, port.ReasonListingQuarantined)
	case port.ListingDeleted:
		return deny(2, port.ReasonNotInCatalog)
	}
	cat := f.Catalog.Resolve(c.Workspace, People(r.Subject), ref)
	if !cat.Visible {
		return deny(2, port.ReasonNotInCatalog)
	}
	fn, ok := f.Functions[r.Function]
	switch {
	case !ok:
		return deny(3, port.ReasonUnknownFunction)
	case fn.State != port.FunctionActive:
		return deny(3, map[port.FunctionState]port.Code{port.FunctionQuarantined: port.ReasonQuarantined, port.FunctionRemoved: port.ReasonRemoved, port.FunctionDisabled: port.ReasonDisabled}[fn.State])
	}
	if caller := callerOf(r.Subject); !visibleTo(fn, caller) {
		if caller == port.CallerModel {
			return deny(4, port.ReasonAppOnly)
		}
		return deny(4, port.ReasonModelOnly)
	}
	if f.Paused {
		return deny(5, port.ReasonPaused)
	}
	d := Decision{AccessDecision: port.AccessDecision{Mode: port.ModeAllow, Step: 10}}
	layers := []struct {
		step    int
		kind    port.ScopeKind
		id      string
		missing port.Code
		narrows bool
	}{
		{6, port.ScopeTenant, string(c.Tenant), "", true},
		{7, port.ScopeWorkspace, string(c.Workspace), port.ReasonNoWorkspaceGrant, false},
		{8, port.ScopeAgent, string(r.Subject.Agent), port.ReasonNoAgentGrant, false},
		{9, port.ScopeUser, string(r.Subject.OnBehalfOf), "", true},
	}
	for i, l := range layers {
		a := grant.Match(rowsOf(f.Grants, l.kind, l.id, c), fn, r.Now)
		if l.narrows && (!a.Found || l.id == "") {
			d.Layers[i] = "skip"
			continue
		}
		if !a.Found {
			d.Layers[i] = "none"
			return Decision{AccessDecision: port.AccessDecision{Mode: port.ModeDeny, Reason: l.missing, Step: l.step}, Layers: d.Layers}
		}
		d.Layers[i] = string(a.Mode)
		d.Matched = append(d.Matched, a.Row.ID)
		if a.Mode == port.ModeDeny {
			reason := a.Reason
			if reason == "" {
				reason = port.ReasonDeniedByRow
			}
			return Decision{AccessDecision: port.AccessDecision{Mode: port.ModeDeny, Reason: reason, Step: l.step}, Layers: d.Layers}
		}
		d.Mode = d.Mode.Stricter(a.Mode)
	}
	if m := catalog.CeilingMode(cat.Ceiling, fn.Class); m.Rank() < d.Mode.Rank() {
		d.Mode, d.Step = m, 11
		if m == port.ModeDeny {
			d.Reason = port.ReasonOverCatalogCeiling
			return d
		}
		d.Notes = append(d.Notes, port.NoteCappedByCatalogCeiling)
	}
	if fn.Class.Effective() == port.ClassDestructive && d.Mode == port.ModeAllow {
		d.Mode, d.Step = port.ModeAskOnce, 12
		if f.DestructiveMax == port.ModeAsk {
			d.Mode = port.ModeAsk
		}
		d.Notes = append(d.Notes, port.NoteCappedByDestructiveMax)
	}
	d.Exposed = f.LegC[fn.Name]
	if f.ForceAsk && d.Exposed && d.Mode.Rank() > port.ModeAsk.Rank() {
		d.Mode, d.Step = port.ModeAsk, 13
		d.Notes = append(d.Notes, port.NoteForcedAsk)
	}
	if d.Mode == port.ModeAskOnce && SessionGranted(f.Grants, r.Subject, c.ID, fn, r.Now) {
		d.Mode, d.Step = port.ModeAllow, 14
		d.Notes = append(d.Notes, port.NoteSessionGrant)
	}
	if f.External != nil {
		q := port.Query{
			Tenant: c.Tenant, Workspace: c.Workspace, Agent: r.Subject.Agent, AgentOwner: r.Subject.AgentOwner, OnBehalfOf: r.Subject.OnBehalfOf,
			Caller: callerOf(r.Subject), Session: r.Subject.Session, Connector: c.ID, Function: fn.Name, Class: fn.Class,
		}
		queries := []port.Query{q}
		if r.Call {
			q.Arguments = r.Arguments
			if q.Arguments == nil {
				q.Arguments = []byte{}
			}
			queries = append(queries, q)
		}
		for _, q := range queries {
			if m := f.External(q); m.Rank() < d.Mode.Rank() {
				d.Mode, d.Step = m, 15
				if m == port.ModeDeny {
					d.Reason = port.ReasonExternalDeny
					return d
				}
				d.Notes = append(d.Notes, port.NoteExternalNarrowed)
			}
		}
	}
	return d
}

// Search evaluates every function of the connector without arguments, in name order: what tools/list lists.
func Search(f Facts, s port.Subject, now time.Time) []port.FunctionDecision {
	names := make([]string, 0, len(f.Functions))
	for n := range f.Functions {
		names = append(names, n)
	}
	slices.Sort(names)
	out := make([]port.FunctionDecision, 0, len(names))
	for _, n := range names {
		out = append(out, port.FunctionDecision{Function: n, Decision: Evaluate(f, Request{Subject: s, Function: n, Now: now}).AccessDecision})
	}
	return out
}

// SessionGranted reports whether an unexpired session row of this agent covers fn:F at the current hash.
func SessionGranted(rows []port.Grant, s port.Subject, connector port.ConnectorID, fn port.Function, now time.Time) bool {
	for _, g := range rows {
		if g.ScopeKind == port.ScopeSession && g.ScopeID == string(s.Session) && g.Agent == s.Agent && g.Connector == connector &&
			g.Selector == port.SelectorOf(fn.Name) && g.DefinitionHash == fn.DefinitionHash && grant.Live(g, now) {
			return true
		}
	}
	return false
}

func rowsOf(rows []port.Grant, kind port.ScopeKind, id string, c *port.Connector) []port.Grant {
	var out []port.Grant
	for _, g := range rows {
		if g.ScopeKind != kind || g.ScopeID != id || g.Tenant != c.Tenant {
			continue
		}
		if kind == port.ScopeTenant && g.Listing != nil && *g.Listing == c.Listing.Listing || kind != port.ScopeTenant && g.Connector == c.ID {
			out = append(out, g)
		}
	}
	return out
}

func callerOf(s port.Subject) port.Caller {
	if s.Caller == "" {
		return port.CallerModel
	}
	return s.Caller
}

func visibleTo(f port.Function, c port.Caller) bool {
	want := map[port.Caller]string{port.CallerModel: port.VisibleToModel, port.CallerView: port.VisibleToApp}[c]
	return len(f.Visibility) == 0 || slices.Contains(f.Visibility, want)
}
