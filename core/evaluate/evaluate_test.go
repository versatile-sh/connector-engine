package evaluate_test

import (
	"fmt"
	"sync"
	"testing"
)

const (
	conn = "s"
	fn   = "f"
	h1   = "h1"
	h0   = "h0"
)

const svc = "/svc"

func shown() *World {
	i := NewInstallation()
	i.AddListing(&Listing{Name: "svc", Source: "own", Publisher: "operator"})
	i.Rules = append(i.Rules, &CatalogRule{Tenant: "t", SubjectKind: TenantSubject, SubjectID: "t", TargetKind: ListingTarget, Target: svc, Show: true, Install: true, Ceiling: CeilFull})
	return i.Tenant("t")
}

func world(class Class) *World {
	w := shown()
	w.Connectors[conn] = &Connector{ID: conn, Listing: svc, Workspace: "w", Enabled: true, Functions: map[string]*Function{
		fn: {Name: fn, Hash: h1, Class: class},
	}}
	w.Bindings[[2]string{conn, "x"}] = true
	w.Bindings[[2]string{conn, "x2"}] = true
	return w
}

func row(scope Scope, id, sel string, mode Mode) *Grant {
	g := &Grant{Scope: scope, ScopeID: id, Connector: conn, Selector: sel, Mode: mode}
	if len(sel) > 3 && sel[:3] == "fn:" {
		g.Hash = h1
	}
	return g
}

func ws(sel string, mode Mode) *Grant { return row(Workspace, "w", sel, mode) }
func ag(sel string, mode Mode) *Grant { return row(Agent, "x", sel, mode) }
func us(sel string, mode Mode) *Grant { return row(User, "u", sel, mode) }

func stale(g *Grant) *Grant {
	g.Hash = h0
	return g
}

func sess(session, agent string) *Grant {
	return &Grant{Scope: Session, ScopeID: session, Agent: agent, Connector: conn, Selector: "fn:" + fn, Mode: Allow, Hash: h1, ExpiresAt: 1000, Origin: FromApproval}
}

func req() Request {
	return Request{Agent: "x", Session: "s1", Connector: conn, Function: fn, Now: 10}
}

func listed(w *World, r Request) bool {
	for _, n := range w.Listed(r.Agent, r.User, r.Session, r.Caller, r.Now) {
		if n == r.Connector+"__"+r.Function {
			return true
		}
	}
	return false
}

type truthRow struct {
	name   string
	class  Class
	grants []*Grant
	setup  func(*World)
	req    func(*Request)
	mode   Mode
	reason string
	listed bool
	call   Outcome
}

func onBehalf(r *Request) { r.User = "u" }

func truthTable() []truthRow {
	return []truthRow{
		{name: "row 1: W allow, A allow -> allow, listed, forwarded", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)}, mode: Allow, listed: true, call: Forwarded},
		{name: "row 2: W allow, A none -> deny no_agent_grant, hidden, unknown tool", class: Write,
			grants: []*Grant{ws("*", Allow)}, mode: Deny, reason: "no_agent_grant", call: UnknownTool},
		{name: "row 3: W none, A allow -> deny no_workspace_grant", class: Write,
			grants: []*Grant{ag("*", Allow)}, mode: Deny, reason: "no_workspace_grant", call: UnknownTool},
		{name: "row 4: W allow, A ask -> ask, listed, held every time", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Ask)}, mode: Ask, listed: true, call: Held},
		{name: "row 5: W allow, A ask, session row -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Ask), sess("s1", "x")}, mode: Ask, listed: true, call: Held},
		{name: "row 6: W allow, A ask_once, no session row -> ask_once, held the first time", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", AskOnce)}, mode: AskOnce, listed: true, call: Held},
		{name: "row 7: W allow, A ask_once, session row -> allow, forwarded", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", AskOnce), sess("s1", "x")}, mode: Allow, listed: true, call: Forwarded},
		{name: "row 8: W ask, A allow -> ask", class: Write,
			grants: []*Grant{ws("*", Ask), ag("*", Allow)}, mode: Ask, listed: true, call: Held},
		{name: "row 9: W deny, A allow -> deny", class: Write,
			grants: []*Grant{ws("*", Deny), ag("*", Allow)}, mode: Deny, reason: "denied_by_row", call: UnknownTool},
		{name: "row 10: W allow, A allow, U deny -> deny", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), us("*", Deny)}, req: onBehalf, mode: Deny, reason: "denied_by_row", call: UnknownTool},
		{name: "row 11: W allow, A allow, U ask -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), us("*", Ask)}, req: onBehalf, mode: Ask, listed: true, call: Held},
		{name: "row 12: U has other rows for C, none matches F -> the user layer is skipped", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), us("fn:other", Deny)}, req: onBehalf, mode: Allow, listed: true, call: Forwarded},
		{name: "row 13: W ask_once, A allow, U ask_once, session row -> allow", class: Write,
			grants: []*Grant{ws("*", AskOnce), ag("*", Allow), us("*", AskOnce), sess("s1", "x")}, req: onBehalf, mode: Allow, listed: true, call: Forwarded},
		{name: "row 14: quarantined, W allow, A allow, session row -> deny quarantined", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), sess("s1", "x")},
			setup:  func(w *World) { w.Connectors[conn].Functions[fn].State = Quarantined },
			mode:   Deny, reason: "quarantined", call: UnknownTool},
		{name: "row 15: agent fn:F allow at an old hash and * ask -> deny grant_stale", class: Write,
			grants: []*Grant{ws("*", Allow), stale(ag("fn:"+fn, Allow)), ag("*", Ask)}, mode: Deny, reason: "grant_stale", call: UnknownTool},
		{name: "row 16: class destructive, W allow, agent level write -> ask", class: Destructive,
			grants: []*Grant{ws("*", Allow)},
			setup:  func(w *World) { w.SetLevel(Agent, "x", conn, WriteLevel) },
			mode:   Ask, listed: true, call: Held},
		{name: "row 17: class read, workspace level write, agent level read -> allow", class: Read,
			setup: func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, ReadLevel) },
			mode:  Allow, listed: true, call: Forwarded},
		{name: "row 18: class write, workspace level write, agent level read -> deny", class: Write,
			setup: func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, ReadLevel) },
			mode:  Deny, reason: "denied_by_row", call: UnknownTool},
		{name: "row 19: external point denies without arguments -> deny external_deny, hidden", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.External = func(Query) Mode { return Deny } },
			mode:   Deny, reason: "external_deny", call: UnknownTool},
		{name: "row 20: external point says allow, table says ask -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Ask)},
			setup:  func(w *World) { w.External = func(Query) Mode { return Allow } },
			mode:   Ask, listed: true, call: Held},
		{name: "row 21 (audit: unclassified under * allow): -> ask_once", class: Unclassified,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)}, mode: AskOnce, listed: true, call: Held},
		{name: "row 22: unclassified, * allow in both layers, session row -> allow", class: Unclassified,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), sess("s1", "x")}, mode: Allow, listed: true, call: Forwarded},
		{name: "row 23: class destructive, agent fn:F allow at the current hash -> ask_once", class: Destructive,
			grants: []*Grant{ws("*", Allow), ag("fn:"+fn, Allow)}, mode: AskOnce, listed: true, call: Held},
		{name: "row 24 (audit: class raised after a grant): fn:F allow written at class read, class then destructive -> ask_once", class: Read,
			grants: []*Grant{ws("*", Allow), ag("fn:"+fn, Allow), sess("s1", "x")},
			setup:  func(w *World) { w.Classify(conn, fn, Destructive) },
			mode:   AskOnce, listed: true, call: Held},
		{name: "row 25 (audit: fn deny at an old hash): agent fn:F deny at old hash and * allow -> deny", class: Write,
			grants: []*Grant{ws("*", Allow), stale(ag("fn:"+fn, Deny)), ag("*", Allow)}, mode: Deny, reason: "denied_by_row", call: UnknownTool},
		{name: "row 26: agent fn:F ask at old hash and * allow -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), stale(ag("fn:"+fn, Ask)), ag("*", Allow)}, mode: Ask, listed: true, call: Held},
		{name: "row 27: agent fn:F ask_once at old hash and * allow -> deny grant_stale", class: Write,
			grants: []*Grant{ws("*", Allow), stale(ag("fn:"+fn, AskOnce)), ag("*", Allow)}, mode: Deny, reason: "grant_stale", call: UnknownTool},
		{name: "row 28 (audit: two agents in one session): session row of agent x, request by agent x2 with ask_once -> ask_once", class: Write,
			grants: []*Grant{ws("*", Allow), row(Agent, "x2", "*", AskOnce), sess("s1", "x")},
			req:    func(r *Request) { r.Agent = "x2" },
			mode:   AskOnce, listed: true, call: Held},
		{name: "row 29 (audit: equal specificity): two agent rows class:write, allow and ask -> ask", class: Write,
			grants: []*Grant{ws("*", Allow)},
			setup: func(w *World) {
				w.Grants = append(w.Grants, ag("class:write", Allow), ag("class:write", Ask))
			},
			mode: Ask, listed: true, call: Held},
		{name: "row 30: a pause for (S, A, C) -> deny paused", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.Pauses[[3]string{"s1", "x", conn}] = true },
			mode:   Deny, reason: "paused", call: UnknownTool},
		{name: "row 31: force_ask, F in leg (c) of a true exposure check -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  forceAsk,
			mode:   Ask, listed: true, call: Held},
		{name: "row 32: force_ask, agent ask_once, session row -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", AskOnce), sess("s1", "x")},
			setup:  forceAsk,
			mode:   Ask, listed: true, call: Held},
		{name: "row 33: app-only function, caller model -> deny app_only", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.Connectors[conn].Functions[fn].Visibility = []Caller{View} },
			mode:   Deny, reason: "app_only", call: UnknownTool},
		{name: "row 34: app-only function, caller view -> allow, listed to the view", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.Connectors[conn].Functions[fn].Visibility = []Caller{View} },
			req:    func(r *Request) { r.Caller = View },
			mode:   Allow, listed: true, call: Forwarded},
		{name: "row 35: model-only function, caller view -> deny model_only", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.Connectors[conn].Functions[fn].Visibility = []Caller{Model} },
			req:    func(r *Request) { r.Caller = View },
			mode:   Deny, reason: "model_only", call: UnknownTool},
		{name: "row 36: session ended -> deny session_ended, even with a session row", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), sess("s1", "x")},
			setup:  func(w *World) { w.EndedSessions["s1"] = true },
			mode:   Deny, reason: "session_ended", call: UnknownTool},
		{name: "row 37: connector disabled -> deny connector_disabled", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.Connectors[conn].Enabled = false },
			mode:   Deny, reason: "connector_disabled", call: UnknownTool},
		{name: "row 38: agent fn:F allow with max_uses 1 and uses 1, and * ask -> ask from *", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Ask)},
			setup: func(w *World) {
				g := ag("fn:"+fn, Allow)
				g.MaxUses, g.Uses = 1, 1
				w.PutGrant(g)
			},
			mode: Ask, listed: true, call: Held},
		{name: "row 41: external point answers ask on a table allow -> ask", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup:  func(w *World) { w.External = func(Query) Mode { return Ask } },
			mode:   Ask, listed: true, call: Held},
		{name: "row 42 (owner case vii): a new function, not yet accepted -> deny quarantined", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)},
			setup: func(w *World) {
				delete(w.Connectors[conn].Functions, fn)
				w.UpstreamAdded(conn, &Function{Name: fn, Hash: h1, Class: Read})
			},
			mode: Deny, reason: "quarantined", call: UnknownTool},
		{name: "row 43: W allow, A allow, U ask_once, session row -> allow", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow), us("*", AskOnce), sess("s1", "x")}, req: onBehalf, mode: Allow, listed: true, call: Forwarded},
		{name: "row 44: class destructive, workspace level write, agent * allow -> ask", class: Destructive,
			grants: []*Grant{ag("*", Allow)},
			setup:  func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel) },
			mode:   Ask, listed: true, call: Held},
		{name: "row 45 (owner case viii): unclassified, workspace level write, agent level read -> deny", class: Unclassified,
			setup: func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, ReadLevel) },
			mode:  Deny, reason: "denied_by_row", call: UnknownTool},
		{name: "row 46 (owner case viii): unclassified, workspace level write, agent level write -> ask", class: Unclassified,
			setup: func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, WriteLevel) },
			mode:  Ask, listed: true, call: Held},
		{name: "row 47: class write, * allow in both layers -> allow, forwarded with no prompt", class: Write,
			grants: []*Grant{ws("*", Allow), ag("*", Allow)}, mode: Allow, listed: true, call: Forwarded},
	}
}

func forceAsk(w *World) {
	w.ForceAsk = true
	w.ExposedLegC["x"] = map[string]bool{conn + "/" + fn: true}
}

func TestTruthTable(t *testing.T) {
	rows := truthTable()
	if len(rows) != 45 {
		t.Fatalf("rows encoded here: %d, want 45 (rows 39 and 40 have their own tests)", len(rows))
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			w := world(tc.class)
			for _, g := range tc.grants {
				w.PutGrant(g)
			}
			if tc.setup != nil {
				tc.setup(w)
			}
			r := req()
			if tc.req != nil {
				tc.req(&r)
			}
			d := w.Evaluate(r, true)
			if d.Mode != tc.mode {
				t.Fatalf("effective %v (step %d, %s), want %v", d.Mode, d.Step, d.Reason, tc.mode)
			}
			if tc.reason != "" && d.Reason != tc.reason {
				t.Fatalf("reason %q, want %q", d.Reason, tc.reason)
			}
			if got := listed(w, r); got != tc.listed {
				t.Fatalf("listed %v, want %v", got, tc.listed)
			}
			if got, _ := w.Call(r); got != tc.call {
				t.Fatalf("call %v, want %v", got, tc.call)
			}
		})
	}
}

func TestRow39CountedGrantUnderTwoConcurrentCalls(t *testing.T) {
	for i := 0; i < 500; i++ {
		w := world(Write)
		w.PutGrant(ws("*", Allow))
		g := ag("fn:"+fn, Allow)
		g.MaxUses = 1
		w.PutGrant(g)
		var wg sync.WaitGroup
		out := make([]Outcome, 2)
		for k := range out {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out[k], _ = w.Call(req())
			}()
		}
		wg.Wait()
		forwarded := 0
		for _, o := range out {
			if o == Forwarded {
				forwarded++
			} else if o != UnknownTool {
				t.Fatalf("row 39: the losing call got %v, want unknown tool (no_agent_grant)", o)
			}
		}
		if forwarded != 1 || g.Uses != 1 {
			t.Fatalf("G6: forwarded %d, uses %d, want 1 and 1", forwarded, g.Uses)
		}
	}
}

func TestRow40ExternalPointWithArgumentConditions(t *testing.T) {
	w := world(Write)
	w.PutGrant(ws("*", Allow))
	w.PutGrant(ag("*", Allow))
	w.External = func(q Query) Mode {
		if q.HasArgs && q.Arguments == "bad" {
			return Deny
		}
		return Allow
	}
	r := req()
	if !listed(w, r) {
		t.Fatal("row 40: listed, because the query without arguments is not denied")
	}
	r.Arguments = "bad"
	if got, d := w.Call(r); got != UnknownTool || d.Reason != "external_deny" {
		t.Fatalf("row 40: call with the denied arguments got %v %s, want unknown tool external_deny", got, d.Reason)
	}
	r.Arguments = "good"
	if got, _ := w.Call(r); got != Forwarded {
		t.Fatalf("row 40: call with other arguments got %v, want forwarded", got)
	}
}

func catalog() *World {
	w := shown()
	w.Connectors[conn] = &Connector{ID: conn, Listing: svc, Workspace: "w", Enabled: true, Functions: map[string]*Function{
		"search":       {Name: "search", Hash: "a1", Class: Read},
		"create_issue": {Name: "create_issue", Hash: "b1", Class: Write},
		"delete_repo":  {Name: "delete_repo", Hash: "c1", Class: Destructive},
		"export":       {Name: "export", Hash: "d1", Class: Unclassified},
	}}
	for _, a := range []string{"x", "y", "z"} {
		w.Bindings[[2]string{conn, a}] = true
	}
	return w
}

func call(w *World, agent, session, function string) (Outcome, Decision) {
	return w.Call(Request{Agent: agent, Session: session, Connector: conn, Function: function, Now: 10, TokenExp: 5000})
}

func wantCall(t *testing.T, where string, w *World, agent, session, function string, want Outcome) Decision {
	t.Helper()
	got, d := call(w, agent, session, function)
	if got != want {
		t.Fatalf("%s: %s by %s in %s got %v (%v, step %d, %s), want %v", where, function, agent, session, got, d.Mode, d.Step, d.Reason, want)
	}
	return d
}

func wantList(t *testing.T, where string, w *World, agent, session string, want ...string) {
	t.Helper()
	got := w.Listed(agent, "", session, Model, 10)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s: list for %s is %v, want %v", where, agent, got, want)
	}
}

func fnRow(agent, function string, mode Mode, hash string) *Grant {
	return &Grant{Scope: Agent, ScopeID: agent, Connector: conn, Selector: "fn:" + function, Mode: mode, Hash: hash}
}

func TestOwnerCases(t *testing.T) {
	t.Run("(i) only function F of server S for agent X: rows 1 and 2", func(t *testing.T) {
		w := catalog()
		w.PutGrant(ws("*", Allow))
		w.PutGrant(fnRow("x", "create_issue", Allow, "b1"))
		wantList(t, "tools/list", w, "x", "s1", "s__create_issue")
		wantCall(t, "tools/call", w, "x", "s1", "create_issue", Forwarded)
		for _, f := range []string{"search", "delete_repo", "export"} {
			if d := wantCall(t, "tools/call", w, "x", "s1", f, UnknownTool); d.Reason != "no_agent_grant" {
				t.Fatalf("%s refused for %s, want no_agent_grant", f, d.Reason)
			}
		}
		wantList(t, "another agent with a binding and no rows", w, "y", "s1")
	})

	t.Run("(ii) all functions enabled, each use asks: rows 4 and 5", func(t *testing.T) {
		w := catalog()
		w.PutGrant(ws("*", Allow))
		w.PutGrant(row(Agent, "x", "*", Ask))
		wantList(t, "tools/list", w, "x", "s1", "s__create_issue", "s__delete_repo", "s__export", "s__search")
		for _, f := range []string{"search", "create_issue", "delete_repo", "export"} {
			wantCall(t, "first use", w, "x", "s1", f, Held)
			if w.ApproveForSession(Request{Agent: "x", Session: "s1", Connector: conn, Function: f, Now: 10}) {
				t.Fatalf("approve for this session offered for %s in mode ask", f)
			}
			wantCall(t, "second use", w, "x", "s1", f, Held)
		}
	})

	t.Run("(iii) read-only for agent Y: rows 17, 18 and 45", func(t *testing.T) {
		w := catalog()
		w.SetLevel(Workspace, "w", conn, WriteLevel)
		w.SetLevel(Agent, "y", conn, ReadLevel)
		wantList(t, "tools/list", w, "y", "s1", "s__search")
		wantCall(t, "tools/call", w, "y", "s1", "search", Forwarded)
		for _, f := range []string{"create_issue", "delete_repo", "export"} {
			wantCall(t, "tools/call", w, "y", "s1", f, UnknownTool)
		}
		for _, f := range w.Connectors[conn].Functions {
			f.Class = Unclassified
		}
		wantList(t, "unreviewed connector: read-only is empty", w, "y", "s1")
	})

	t.Run("(iv) ask once per session: rows 6, 7 and 28, G5", func(t *testing.T) {
		w := catalog()
		w.PutGrant(ws("*", Allow))
		w.PutGrant(fnRow("x", "create_issue", AskOnce, "b1"))
		w.PutGrant(fnRow("z", "create_issue", AskOnce, "b1"))
		wantCall(t, "first call in s1", w, "x", "s1", "create_issue", Held)
		r := Request{Agent: "x", Session: "s1", Connector: conn, Function: "create_issue", Now: 10, TokenExp: 5000}
		if !w.ApproveForSession(r) {
			t.Fatal("approve for this session refused in mode ask_once")
		}
		g := w.Grants[len(w.Grants)-1]
		if g.Scope != Session || g.Agent != "x" || g.ExpiresAt != 5000 {
			t.Fatalf("G5: session row %+v, want agent x and expiry at the token's exp 5000", g)
		}
		wantCall(t, "second call in s1", w, "x", "s1", "create_issue", Forwarded)
		wantCall(t, "first call in s2", w, "x", "s2", "create_issue", Held)
		wantCall(t, "G5 another agent in s1", w, "z", "s1", "create_issue", Held)
		if out, _ := w.Call(Request{Agent: "x", Session: "s1", Connector: conn, Function: "create_issue", Now: 5000}); out != Held {
			t.Fatalf("G5: after the session row expired the call got %v, want held", out)
		}
		w.PutGrant(ws("fn:create_issue", Ask))
		w.Grants[len(w.Grants)-1].Hash = "b1"
		wantCall(t, "the workspace later sets fn:create_issue to ask", w, "x", "s1", "create_issue", Held)
	})

	t.Run("(v) a workspace ceiling an agent row tries to widen: step 10, rows 3, 8 and 9", func(t *testing.T) {
		for _, tc := range []struct {
			ceiling *Grant
			want    Outcome
		}{{ws("*", Ask), Held}, {ws("*", Deny), UnknownTool}, {nil, UnknownTool}, {ws("*", AskOnce), Held}} {
			w := catalog()
			if tc.ceiling != nil {
				w.PutGrant(tc.ceiling)
			}
			w.PutGrant(row(Agent, "x", "*", Allow))
			w.PutGrant(fnRow("x", "search", Allow, "a1"))
			wantCall(t, "step 10", w, "x", "s1", "search", tc.want)
		}
		w := catalog()
		w.PutGrant(ws("*", Deny))
		w.PutGrant(ws("fn:search", Allow))
		w.Grants[len(w.Grants)-1].Hash = "a1"
		w.PutGrant(row(Agent, "x", "*", Allow))
		wantCall(t, "G4: inside one layer a more specific row may be looser", w, "x", "s1", "search", Forwarded)
		wantCall(t, "G4", w, "x", "s1", "create_issue", UnknownTool)
	})

	t.Run("(vi) a definition change after a grant: M4 and M5, rows 14, 25, 26, 27", func(t *testing.T) {
		w := catalog()
		w.PutGrant(ws("*", Allow))
		w.PutGrant(row(Agent, "x", "*", Allow))
		w.PutGrant(fnRow("x", "create_issue", AskOnce, "b1"))
		w.PutGrant(fnRow("x", "search", Deny, "a1"))
		r := Request{Agent: "x", Session: "s1", Connector: conn, Function: "create_issue", Now: 10}
		w.ApproveForSession(r)
		wantCall(t, "before the change", w, "x", "s1", "create_issue", Forwarded)
		w.UpstreamChanged(conn, "create_issue", "b2")
		w.UpstreamChanged(conn, "search", "a2")
		if d := wantCall(t, "hash changed", w, "x", "s1", "create_issue", UnknownTool); d.Reason != "quarantined" {
			t.Fatalf("reason %s, want quarantined", d.Reason)
		}
		w.Accept(conn, "create_issue")
		w.Accept(conn, "search")
		if d := wantCall(t, "M4 after acceptance, before confirmGrant", w, "x", "s1", "create_issue", UnknownTool); d.Reason != "grant_stale" {
			t.Fatalf("M4: reason %s, want grant_stale", d.Reason)
		}
		if d := wantCall(t, "M5 after acceptance the fn deny row still answers", w, "x", "s1", "search", UnknownTool); d.Reason != "denied_by_row" {
			t.Fatalf("M5: reason %s, want denied_by_row", d.Reason)
		}
		w.ConfirmGrant(Agent, "x", conn, "create_issue")
		wantCall(t, "after confirmGrant the session row is gone, so the call is held again", w, "x", "s1", "create_issue", Held)
	})

	t.Run("(vii) a new function in an update: row 42", func(t *testing.T) {
		w := catalog()
		w.PutGrant(ws("*", Allow))
		w.PutGrant(row(Agent, "x", "*", Allow))
		w.UpstreamAdded(conn, &Function{Name: "list_repos", Hash: "e1", Class: Read})
		wantList(t, "new function: not reachable through * rows", w, "x", "s1", "s__create_issue", "s__delete_repo", "s__export", "s__search")
		wantCall(t, "new function", w, "x", "s1", "list_repos", UnknownTool)
		w.Accept(conn, "list_repos")
		wantCall(t, "after acceptFunction", w, "x", "s1", "list_repos", Forwarded)
	})

	t.Run("(viii) an unclassified function: step 12, rows 21, 45, 46", func(t *testing.T) {
		for _, tc := range []struct {
			where string
			setup func(*World)
			want  Outcome
		}{
			{"level read", func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, ReadLevel) }, UnknownTool},
			{"level write", func(w *World) { w.SetLevel(Workspace, "w", conn, WriteLevel); w.SetLevel(Agent, "x", conn, WriteLevel) }, Held},
			{"* ask", func(w *World) { w.PutGrant(ws("*", Allow)); w.PutGrant(row(Agent, "x", "*", Ask)) }, Held},
			{"* allow", func(w *World) { w.PutGrant(ws("*", Allow)); w.PutGrant(row(Agent, "x", "*", Allow)) }, Held},
			{"fn allow", func(w *World) { w.PutGrant(ws("*", Allow)); w.PutGrant(fnRow("x", "export", Allow, "d1")) }, Held},
		} {
			w := catalog()
			tc.setup(w)
			wantCall(t, "unclassified under "+tc.where, w, "x", "s1", "export", tc.want)
		}
		w := catalog()
		w.DestructiveMax = Ask
		w.PutGrant(ws("*", Allow))
		w.PutGrant(row(Agent, "x", "*", Allow))
		if _, d := call(w, "x", "s1", "export"); d.Mode != Ask {
			t.Fatalf("step 12 with permission.destructiveMax = ask: %v, want ask", d.Mode)
		}
	})

	t.Run("enable all with destructive and unclassified functions present:", func(t *testing.T) {
		w := catalog()
		w.Connectors[conn].Functions["purge"] = &Function{Name: "purge", Hash: "p1", Class: Read, State: Quarantined}
		w.Connectors[conn].Functions["panel"] = &Function{Name: "panel", Hash: "v1", Class: Read, Visibility: []Caller{View}}
		w.PutGrant(ws("*", Allow))
		w.PutGrant(row(Agent, "x", "*", Allow))
		wantList(t, "tools/list", w, "x", "s1", "s__create_issue", "s__delete_repo", "s__export", "s__search")
		wantCall(t, "class read", w, "x", "s1", "search", Forwarded)
		wantCall(t, "class write", w, "x", "s1", "create_issue", Forwarded)
		wantCall(t, "quarantined", w, "x", "s1", "purge", UnknownTool)
		wantCall(t, "app-only, called by the model", w, "x", "s1", "panel", UnknownTool)
		for _, f := range []string{"delete_repo", "export"} {
			if d := wantCall(t, "destructive or unclassified, first use", w, "x", "s1", f, Held); d.Mode != AskOnce {
				t.Fatalf("%s is %v, want ask_once", f, d.Mode)
			}
			if !w.ApproveForSession(Request{Agent: "x", Session: "s1", Connector: conn, Function: f, Now: 10}) {
				t.Fatalf("approve for this session refused for %s", f)
			}
			wantCall(t, "after approve for this session", w, "x", "s1", f, Forwarded)
			wantCall(t, "in another session", w, "x", "s2", f, Held)
		}
		w.Classify(conn, "search", Destructive)
		wantCall(t, "G7 class raised after the grant", w, "x", "s1", "search", Held)
	})
}

func TestLevelOverHandWrittenRows(t *testing.T) {
	w := catalog()
	w.PutGrant(ws("*", Allow))
	w.PutGrant(row(Agent, "x", "class:write", Allow))
	w.PutGrant(fnRow("x", "delete_repo", Ask, "c1"))
	w.SetLevel(Agent, "x", conn, ReadLevel)
	wantCall(t, "G3: the hand-written class:write row has the key of the expanded one and is replaced", w, "x", "s1", "create_issue", UnknownTool)
	wantCall(t, "G3: the hand-written fn: row is kept", w, "x", "s1", "delete_repo", Held)
	w.SetLevel(Agent, "x", conn, Custom)
	wantList(t, "G3: level custom deletes the level rows and keeps the fn: row", w, "x", "s1", "s__delete_repo")
}

func TestUserLayerOnlyNarrows(t *testing.T) {
	w := world(Write)
	w.PutGrant(ws("*", Allow))
	w.PutGrant(ag("*", Allow))
	g := us("fn:"+fn, Allow)
	g.MaxUses, g.Uses = 1, 1
	w.PutGrant(g)
	r := req()
	r.User = "u"
	if d := w.Evaluate(r, true); d.Mode != Allow || d.Layers[3] != "skip" {
		t.Fatalf("step 9: an exhausted user row is no candidate and the layer has no opinion: got %v %s, layer %s", d.Mode, d.Reason, d.Layers[3])
	}
	w.PutGrant(us("*", Deny))
	if d := w.Evaluate(r, true); d.Mode != Deny || d.Step != 9 || d.Reason != "denied_by_row" {
		t.Fatalf("G4: only F is written as * deny with fn:F allow; the exhausted fn: row leaves the * deny row: got %v at step %d, %s", d.Mode, d.Step, d.Reason)
	}
	g.Uses = 0
	if d := w.Evaluate(r, true); d.Mode != Allow {
		t.Fatalf("M2: the fn: row answers before the * row: got %v %s", d.Mode, d.Reason)
	}
}

func TestRowsRefusedAtWriteTime(t *testing.T) {
	w := world(Write)
	for _, tc := range []struct {
		name string
		g    *Grant
		ok   bool
	}{
		{"G8: a tenant row never says allow", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "*", Mode: Allow}, false},
		{"G8: a tenant fn: row may say ask_once, as a class: or * row may", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: AskOnce}, true},
		{"G8: a tenant row carries no max_uses", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "*", Mode: AskOnce, MaxUses: 1}, false},
		{"G8: a tenant fn: ask_once row carries no max_uses", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: AskOnce, MaxUses: 2}, false},
		{"G8: a tenant row names a listing, never a connector", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Connector: conn, Selector: "*", Mode: Ask}, false},
		{"G10: a tenant fn: row may say inherit", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: Inherit}, true},
		{"G10: inherit is refused on a tenant * row", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "*", Mode: Inherit}, false},
		{"G10: inherit is refused on a tenant class: row", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "class:write", Mode: Inherit}, false},
		{"G10: an inherit row carries no hash", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: Inherit, Hash: h1}, false},
		{"G10: inherit is refused outside the tenant scope", &Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "fn:" + fn, Mode: Inherit, Hash: h1}, false},
		{"G10: inherit is refused on a user row", &Grant{Scope: User, ScopeID: "u", Connector: conn, Selector: "fn:" + fn, Mode: Inherit, Hash: h1}, false},
		{"G8: a tenant row carries no hash", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: Deny, Hash: h1}, false},
		{"G8: a tenant row carries no expiry", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "*", Mode: Ask, ExpiresAt: 50}, false},
		{"G8: tenant fn: deny", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "fn:" + fn, Mode: Deny}, true},
		{"G8: tenant class: ask_once", &Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: "class:write", Mode: AskOnce}, true},
		{"G9: a deny row is never counted", &Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "fn:" + fn, Mode: Deny, MaxUses: 3}, false},
		{"G9: an ask row is never counted", &Grant{Scope: User, ScopeID: "u", Connector: conn, Selector: "*", Mode: Ask, MaxUses: 1}, false},
		{"G9: a deny row may expire (a temporary block)", &Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "*", Mode: Deny, ExpiresAt: 50}, true},
	} {
		if err := w.PutGrant(tc.g); (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}

func TestHeldCallReservesAtExecution(t *testing.T) {
	w := world(Write)
	w.PutGrant(ws("*", Allow))
	g := ag("fn:"+fn, AskOnce)
	g.MaxUses = 1
	if err := w.PutGrant(g); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if out, _ := w.Call(req()); out != Held || g.Uses != 0 {
			t.Fatalf("G6: opening an approval reserves nothing: %v, uses %d", out, g.Uses)
		}
	}
	if out, _ := w.ExecuteApproved(req()); out != Forwarded || g.Uses != 1 {
		t.Fatalf("G6: A8 reserves: %v, uses %d", out, g.Uses)
	}
	if out, d := w.ExecuteApproved(req()); out != UnknownTool || d.Reason != "no_agent_grant" {
		t.Fatalf("G6: the second approved call finds the row exhausted: %v %s", out, d.Reason)
	}
}

type sweepCount struct {
	worlds, catalogWorlds, hidden, listed, forwarded, held, inherit int
	refusedAt                                                       [17]int
}

func TestListCallSymmetrySweep(t *testing.T) {
	modes := []Mode{Deny, Ask, AskOnce, Allow}
	type opt struct {
		name  string
		apply func(*World, *Request)
	}
	wsOpts := []opt{{"W none", func(*World, *Request) {}}}
	agOpts := []opt{{"A none", func(*World, *Request) {}}}
	usOpts := []opt{
		{"U nobody", func(*World, *Request) {}},
		{"U no rows", func(_ *World, r *Request) { r.User = "u" }},
		{"U other rows", func(w *World, r *Request) { r.User = "u"; w.PutGrant(us("fn:other", Allow)) }},
	}
	for _, m := range modes {
		wsOpts = append(wsOpts, opt{"W * " + string(m), func(w *World, _ *Request) { w.PutGrant(ws("*", m)) }})
		agOpts = append(agOpts,
			opt{"A * " + string(m), func(w *World, _ *Request) { w.PutGrant(ag("*", m)) }},
			opt{"A fn " + string(m), func(w *World, _ *Request) { w.PutGrant(ag("fn:"+fn, m)) }},
			opt{"A stale fn " + string(m) + " over * allow", func(w *World, _ *Request) {
				w.PutGrant(stale(ag("fn:"+fn, m)))
				w.PutGrant(ag("*", Allow))
			}})
		usOpts = append(usOpts, opt{"U * " + string(m), func(w *World, r *Request) { r.User = "u"; w.PutGrant(us("*", m)) }})
	}
	sessOpts := []opt{
		{"no session row", func(*World, *Request) {}},
		{"own session row", func(w *World, _ *Request) { w.PutGrant(sess("s1", "x")) }},
		{"another agent's session row", func(w *World, _ *Request) { w.PutGrant(sess("s1", "x2")) }},
	}
	gateOpts := []opt{
		{"open", func(*World, *Request) {}},
		{"paused", func(w *World, _ *Request) { w.Pauses[[3]string{"s1", "x", conn}] = true }},
		{"force_ask", func(w *World, _ *Request) { forceAsk(w) }},
		{"session ended", func(w *World, _ *Request) { w.EndedSessions["s1"] = true }},
		{"connector disabled", func(w *World, _ *Request) { w.Connectors[conn].Enabled = false }},
		{"no binding", func(w *World, _ *Request) { delete(w.Bindings, [2]string{conn, "x"}) }},
		{"destructiveMax ask", func(w *World, _ *Request) { w.DestructiveMax = Ask }},
	}
	visOpts := []opt{
		{"model, default visibility", func(*World, *Request) {}},
		{"model, app-only", func(w *World, _ *Request) { w.Connectors[conn].Functions[fn].Visibility = []Caller{View} }},
		{"view, app-only", func(w *World, r *Request) {
			w.Connectors[conn].Functions[fn].Visibility = []Caller{View}
			r.Caller = View
		}},
		{"view, model-only", func(w *World, r *Request) {
			w.Connectors[conn].Functions[fn].Visibility = []Caller{Model}
			r.Caller = View
		}},
	}
	extOpts := []opt{
		{"no external point", func(*World, *Request) {}},
		{"external deny", func(w *World, _ *Request) { w.External = func(Query) Mode { return Deny } }},
		{"external ask", func(w *World, _ *Request) { w.External = func(Query) Mode { return Ask } }},
		{"external allow", func(w *World, _ *Request) { w.External = func(Query) Mode { return Allow } }},
		{"external deny on arguments", func(w *World, _ *Request) {
			w.External = func(q Query) Mode {
				if q.HasArgs && q.Arguments == "bad" {
					return Deny
				}
				return Allow
			}
		}},
	}
	type catOpt struct {
		opt
		visible bool
		ceiling Ceiling
	}
	rule := func(w *World) *CatalogRule { return w.Inst.Rules[0] }
	catOpts := []catOpt{
		{opt{"catalog full", func(*World, *Request) {}}, true, CeilFull},
		{opt{"ceiling write", func(w *World, _ *Request) { rule(w).Ceiling = CeilWrite }}, true, CeilWrite},
		{opt{"ceiling read", func(w *World, _ *Request) { rule(w).Ceiling = CeilRead }}, true, CeilRead},
		{opt{"tenant rule withdrawn", func(w *World, _ *Request) { w.Inst.Rules = nil }}, false, CeilRead},
		{opt{"listing quarantined by the operator", func(w *World, _ *Request) { w.Inst.Listings[svc].Status = ListingQuarantined }}, false, CeilRead},
		{opt{"listing withheld from the tenant", func(w *World, _ *Request) { w.Inst.WithheldListing[[2]string{"t", svc}] = true }}, false, CeilRead},
		{opt{"hidden from person u", func(w *World, _ *Request) {
			w.Inst.Rules = append(w.Inst.Rules, &CatalogRule{Tenant: "t", SubjectKind: UserSubject, SubjectID: "u", TargetKind: ListingTarget, Target: svc})
		}}, true, CeilFull},
	}
	tn := func(sel string, m Mode) func(*World, *Request) {
		return func(w *World, _ *Request) {
			if err := w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t", Listing: svc, Selector: sel, Mode: m}); err != nil {
				t.Fatal(err)
			}
		}
	}
	tenOpts := []opt{
		{"T none", func(*World, *Request) {}},
		{"T * deny", tn("*", Deny)},
		{"T * ask", tn("*", Ask)},
		{"T * ask_once", tn("*", AskOnce)},
		{"T fn deny", tn("fn:"+fn, Deny)},
		{"T fn:other deny", tn("fn:other", Deny)},
		{"T * deny, fn inherit", func(w *World, r *Request) { tn("*", Deny)(w, r); tn("fn:"+fn, Inherit)(w, r) }},
		{"T * deny, fn:other inherit", func(w *World, r *Request) { tn("*", Deny)(w, r); tn("fn:other", Inherit)(w, r) }},
	}
	var n sweepCount
	stride, idx := sweepStride(t), 0
	for _, class := range []Class{Unclassified, Read, Write, Destructive} {
		for _, state := range []State{Active, Quarantined, Removed, Disabled} {
			for _, wo := range wsOpts {
				for _, ao := range agOpts {
					for _, uo := range usOpts {
						for _, so := range sessOpts {
							for _, gopt := range gateOpts {
								for vi, vo := range visOpts {
									for ei, eo := range extOpts {
										for ct := 0; ct < len(catOpts)*len(tenOpts); ct++ {
											if ct > 0 && (vi > 0 || ei > 0) {
												break
											}
											if idx++; idx%stride != 0 {
												continue
											}
											co, to := catOpts[ct/len(tenOpts)], tenOpts[ct%len(tenOpts)]
											w := world(class)
											w.Connectors[conn].Functions[fn].State = state
											r := req()
											for _, o := range []opt{wo, ao, uo, so, gopt, vo, eo, co.opt, to} {
												o.apply(w, &r)
											}
											where := fmt.Sprintf("class %q, state %v, %s, %s, %s, %s, %s, %s, %s, %s, %s", class, state, wo.name, ao.name, uo.name, so.name, gopt.name, vo.name, eo.name, co.name, to.name)
											n.worlds++
											if ct > 0 {
												n.catalogWorlds++
											}
											ownSession := so.name == "own session row"
											isListed := listed(w, r)
											d := w.Evaluate(r, false)
											for _, args := range []string{"good", "bad"} {
												r.Arguments = args
												out, dc := w.Call(r)
												if !isListed && out != UnknownTool {
													t.Fatalf("hidden from tools/list and not refused on tools/call: %s: %v", where, out)
												}
												if isListed && ei < 4 && out == UnknownTool {
													t.Fatalf("listed and refused without an argument condition: %s: %s", where, dc.Reason)
												}
												switch out {
												case Forwarded:
													n.forwarded++
												case Held:
													n.held++
												case UnknownTool:
													if args == "good" {
														n.refusedAt[dc.Step]++
													}
												}
												if out != UnknownTool && (!co.visible || (co.name == "hidden from person u" && r.User == "u")) {
													t.Fatalf("listed or called although the listing is not visible: %s", where)
												}
												if out != UnknownTool && (to.name == "T * deny" || to.name == "T fn deny" || to.name == "T * deny, fn:other inherit") {
													t.Fatalf("a tenant deny row did not refuse: %s", where)
												}
												if out != UnknownTool && mode(co.ceiling, class) == Deny {
													t.Fatalf("called above the catalog ceiling: %s", where)
												}
												if out == Forwarded && (mode(co.ceiling, class) == Ask || (mode(co.ceiling, class) == AskOnce && !ownSession)) {
													t.Fatalf("forwarded above the catalog ceiling: %s", where)
												}
												if out == Forwarded {
													if state != Active || gopt.name == "paused" || gopt.name == "session ended" || gopt.name == "connector disabled" || gopt.name == "no binding" {
														t.Fatalf("forwarded through a closed gate: %s", where)
													}
													if class.Effective() == Destructive && so.name != "own session row" {
														t.Fatalf("destructive or unclassified forwarded with no approval in the session: %s", where)
													}
													for _, l := range dc.Layers {
														if l == "ask" || l == "deny" || l == "none" || l == "" {
															t.Fatalf("forwarded although a layer answered %s: %s", l, where)
														}
														if l == "ask_once" && so.name != "own session row" {
															t.Fatalf("ask_once relaxed without this agent's session row: %s", where)
														}
													}
													if gopt.name == "force_ask" {
														t.Fatalf("forwarded under force_ask: %s", where)
													}
													if eo.name == "external deny" || eo.name == "external ask" || (eo.name == "external deny on arguments" && args == "bad") {
														t.Fatalf("forwarded against the external point: %s", where)
													}
												}
											}
											if to.name == "T * deny, fn inherit" {
												wb, rb := world(class), req()
												wb.Connectors[conn].Functions[fn].State = state
												for _, o := range []opt{wo, ao, uo, so, gopt, vo, eo, co.opt} {
													o.apply(wb, &rb)
												}
												if b := wb.Evaluate(rb, false); b.Mode != d.Mode || b.Reason != d.Reason {
													t.Fatalf("a tenant fn: inherit row under * deny changed the answer the other layers give: %s: %v %s, want %v %s", where, d.Mode, d.Reason, b.Mode, b.Reason)
												}
												n.inherit++
											}
											if isListed != (d.Mode != Deny) {
												t.Fatalf("list and evaluation disagree: %s", where)
											}
											if isListed {
												n.listed++
											} else {
												n.hidden++
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	t.Logf("worlds %d (of which %d vary the catalog or the tenant rows), hidden %d, listed %d, calls forwarded %d, calls held %d", n.worlds, n.catalogWorlds, n.hidden, n.listed, n.forwarded, n.held)
	t.Logf("hidden worlds by the step that refused: %v", n.refusedAt[1:])
	t.Logf("worlds with a tenant allow list (* deny, fn: inherit) that answered as the same world with no tenant row: %d", n.inherit)
	for _, step := range []int{2, 6, 11} {
		if n.refusedAt[step] == 0 {
			t.Fatalf("no world of the sweep is refused at step %d", step)
		}
	}
}
