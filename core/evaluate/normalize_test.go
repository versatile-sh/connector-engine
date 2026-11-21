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
	propTimes     = []int64{5, 6, 14, 15, 16, 40}
	rewriteAt     = at(5)
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
	g.ExpiresAt = at(pick(r, []int64{0, 0, 3, 5, 15}))
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

func twinOf(r *rand.Rand, base port.Grant, id string, agent port.AgentID) port.Grant {
	g := propRow(r, id)
	g.Tenant, g.ScopeKind, g.ScopeID, g.Connector, g.Listing, g.Selector, g.Agent = base.Tenant, base.ScopeKind, base.ScopeID, base.Connector, base.Listing, base.Selector, agent
	if base.ScopeKind == Tenant {
		g.DefinitionHash, g.ExpiresAt, g.MaxUses, g.Uses = "", at(0), 0, 0
		if g.Mode == Allow || (g.Mode == Inherit && grant.Specificity(g.Selector) != 2) {
			g.Mode = Deny
		}
		return g
	}
	if g.Mode == Inherit {
		g.Mode = Deny
	}
	if grant.Specificity(g.Selector) == 2 && g.DefinitionHash == "" {
		g.DefinitionHash = h1
	}
	return g
}

func conflicted(rows []port.Grant) bool {
	return slices.ContainsFunc(rows, func(g port.Grant) bool { return g.Reason == grant.ConflictReason })
}

func sameAnswer(a, b evaluate.Decision) bool {
	a.Matched, b.Matched = nil, nil
	return reflect.DeepEqual(a, b)
}

func compare(t *testing.T, w *World, rows []port.Grant) (exact bool) {
	t.Helper()
	after := grant.Canonical(rows, rewriteAt)
	exact = !conflicted(after)
	before, i := decisions(w, rows), 0
	eachTuple(w, after, func(req Request, d evaluate.Decision) {
		b := before[i]
		i++
		switch {
		case exact && !sameAnswer(b, d):
			t.Fatalf("%+v changed\nbefore %+v\nafter  %+v\nrows %+v\nrewritten %+v", req, b, d, rows, after)
		case d.Mode.Rank() > b.Mode.Rank():
			t.Fatalf("%+v widened from %s to %s\nrows %+v\nrewritten %+v", req, b.Mode, d.Mode, rows, after)
		}
	})
	return exact
}

func TestNormalizeKeepsEvaluation(t *testing.T) {
	w := propWorld()
	r := rand.New(rand.NewPCG(1, 2))
	exact := 0
	for range 1500 {
		if compare(t, w, propState(r)) {
			exact++
		}
	}
	if exact < 1000 {
		t.Fatalf("only %d states rewritten without a conflict", exact)
	}
}

func TestNormalizeCollisionNeverWidens(t *testing.T) {
	w := propWorld()
	r := rand.New(rand.NewPCG(3, 4))
	counts := map[bool]int{}
	for range 1500 {
		rows := propState(r)
		base := rows[r.IntN(len(rows))]
		if base.ScopeKind == Session {
			continue
		}
		agents := []port.AgentID{"", "x", "x2", "y", "z"}
		r.Shuffle(len(agents), func(i, j int) { agents[i], agents[j] = agents[j], agents[i] })
		for i := range 1 + r.IntN(3) {
			twin := twinOf(r, base, "twin"+string(rune('a'+i)), agents[i])
			if !slices.ContainsFunc(rows, func(g port.Grant) bool { return g.Key() == twin.Key() }) {
				rows = append(rows, twin)
			}
		}
		counts[compare(t, w, rows)]++
	}
	if counts[true] < 300 || counts[false] < 100 {
		t.Fatalf("%d exact merges, %d merged to deny", counts[true], counts[false])
	}
}
