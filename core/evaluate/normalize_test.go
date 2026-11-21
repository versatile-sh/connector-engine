package evaluate_test

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/versatile-sh/connector-engine/core/evaluate"
	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

type rowScope struct {
	kind Scope
	id   string
}

var (
	propScopes    = []rowScope{{Tenant, "t"}, {Workspace, "w"}, {Agent, "x"}, {Agent, "x2"}, {User, "u"}, {User, "v"}}
	propSelectors = []string{"*", "class:read", "class:write", "class:destructive", "fn:f", "fn:g", "fn:d"}
	propModes     = []Mode{Deny, Ask, AskOnce, Allow}
	propTimes     = []int64{1, 4, 5, 6, 14, 15, 16, 40}
)

func propWorld() *World {
	w := world(Write)
	w.Connectors[conn].Functions["g"] = &Function{Name: "g", Hash: h1, Class: Read}
	w.Connectors[conn].Functions["d"] = &Function{Name: "d", Hash: h1, Class: Destructive}
	return w
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

func propRow(r *rand.Rand, id string) port.Grant {
	s := pick(r, propScopes)
	g := port.Grant{ID: port.GrantID(id), Tenant: "t", ScopeKind: s.kind, ScopeID: s.id, Connector: conn, Selector: pick(r, propSelectors),
		Mode: pick(r, propModes), Agent: port.AgentID(pick(r, []string{"", s.id, "x", "x2"})), Origin: port.OriginSettings}
	if grant.Specificity(g.Selector) == 2 {
		g.DefinitionHash = port.Hash(pick(r, []string{h1, h0}))
	}
	if s.kind == Tenant {
		l := ref(svc)
		g.Connector, g.Listing, g.DefinitionHash = "", &l, ""
		g.Mode = pick(r, []Mode{Deny, Ask, AskOnce})
		if grant.Specificity(g.Selector) == 2 && r.IntN(4) == 0 {
			g.Mode = Inherit
		}
		return g
	}
	g.ExpiresAt = at(pick(r, []int64{0, 0, 5, 15}))
	if g.Mode == Allow || g.Mode == AskOnce {
		g.MaxUses = r.IntN(3)
		if g.MaxUses > 0 {
			g.Uses = r.IntN(g.MaxUses + 1)
		}
	}
	return g
}

func propState(r *rand.Rand) []port.Grant {
	seen := map[string]bool{}
	var rows []port.Grant
	for i := range 1 + r.IntN(8) {
		g := propRow(r, "g"+string(rune('a'+i)))
		if r.IntN(6) == 0 {
			g = port.Grant{ID: g.ID, Tenant: "t", ScopeKind: Session, ScopeID: "s1", Agent: port.AgentID(pick(r, []string{"x", "x2"})), Connector: conn,
				Selector: "fn:" + pick(r, []string{"f", "d"}), Mode: Allow, DefinitionHash: h1, ExpiresAt: at(15), Origin: port.OriginApproval}
		}
		if !seen[g.Key()] {
			seen[g.Key()] = true
			rows = append(rows, g)
		}
	}
	return rows
}

func eachTuple(w *World, rows []port.Grant, fn func(r Request, d evaluate.Decision)) {
	for _, agent := range []string{"x", "x2"} {
		for _, user := range []string{"", "u", "v"} {
			for _, function := range []string{"f", "g", "d"} {
				for _, now := range propTimes {
					r := Request{Agent: agent, User: user, Session: "s1", Connector: conn, Function: function, Now: now}
					f := w.facts(r)
					f.Grants = rows
					fn(r, evaluate.Evaluate(f, evaluate.Request{Subject: w.subject(r), Function: function, Now: at(now)}))
				}
			}
		}
	}
}

func decisions(w *World, rows []port.Grant) []evaluate.Decision {
	var out []evaluate.Decision
	eachTuple(w, rows, func(_ Request, d evaluate.Decision) { out = append(out, d) })
	return out
}

func TestNormalizeKeepsEvaluation(t *testing.T) {
	w := propWorld()
	r := rand.New(rand.NewPCG(1, 2))
	checked := 0
	for range 4000 {
		rows := propState(r)
		after := grant.Canonical(rows)
		if len(after) != len(rows) {
			continue
		}
		checked++
		before, got := decisions(w, rows), decisions(w, after)
		for i := range before {
			if !reflect.DeepEqual(before[i], got[i]) {
				t.Fatalf("rows %+v\nbefore %+v\nafter  %+v", rows, before[i], got[i])
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d states without a collision", checked)
	}
}

func TestNormalizeCollisionNeverWidens(t *testing.T) {
	w := propWorld()
	r := rand.New(rand.NewPCG(3, 4))
	merged, denied := 0, 0
	for range 4000 {
		rows := propState(r)
		base := rows[r.IntN(len(rows))]
		if base.ScopeKind == Session {
			continue
		}
		twin := propRow(r, "twin")
		twin.Tenant, twin.ScopeKind, twin.ScopeID, twin.Connector, twin.Listing, twin.Selector = base.Tenant, base.ScopeKind, base.ScopeID, base.Connector, base.Listing, base.Selector
		twin.Agent = port.AgentID(pick(r, []string{"", "x", "x2", "y"}))
		if base.ScopeKind == Tenant {
			twin.DefinitionHash, twin.ExpiresAt, twin.MaxUses, twin.Uses = "", at(0), 0, 0
			if twin.Mode == Allow || (twin.Mode == Inherit && grant.Specificity(twin.Selector) != 2) {
				twin.Mode = Deny
			}
		} else {
			if twin.Mode == Inherit {
				twin.Mode = Deny
			}
			if grant.Specificity(twin.Selector) == 2 && twin.DefinitionHash == "" {
				twin.DefinitionHash = h1
			}
		}
		if slices.ContainsFunc(rows, func(g port.Grant) bool { return g.Key() == twin.Key() }) {
			continue
		}
		rows = append(rows, twin)
		after := grant.Canonical(rows)
		if len(after) == len(rows) {
			t.Fatalf("no merge: %+v", rows)
		}
		merged++
		for _, g := range after {
			if g.ID == base.ID || g.ID == twin.ID {
				if g.Mode == Deny && base.Mode != Deny && twin.Mode != Deny {
					denied++
				}
			}
		}
		before := decisions(w, rows)
		i := 0
		eachTuple(w, after, func(req Request, d evaluate.Decision) {
			if d.Mode.Rank() > before[i].Mode.Rank() {
				t.Fatalf("%+v widened from %s to %s\nrows %+v\nafter %+v", req, before[i].Mode, d.Mode, rows, after)
			}
			i++
		})
	}
	if merged < 1000 || denied == 0 {
		t.Fatalf("merged %d states, %d merged to deny", merged, denied)
	}
}
