package grant_test

import (
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

func named(g port.Grant, agent port.AgentID) port.Grant {
	g.Agent = agent
	return g
}

func TestAUserRowCannotNameAnAgent(t *testing.T) {
	tenantRow := port.Grant{Tenant: "t", ScopeKind: port.ScopeTenant, ScopeID: "t", Listing: &port.ListingRef{Name: "svc"}, Selector: "*", Mode: port.ModeDeny}
	for _, tc := range []struct {
		name string
		g    port.Grant
		ok   bool
	}{
		{"a user row naming an agent", named(row(port.ScopeUser, "*", port.ModeDeny), "a"), false},
		{"a workspace row naming an agent", named(row(port.ScopeWorkspace, "*", port.ModeDeny), "a"), false},
		{"a tenant row naming an agent", named(tenantRow, "a"), false},
		{"an agent row naming another agent", named(row(port.ScopeAgent, "*", port.ModeDeny), "a"), false},
		{"an agent row naming its own agent", named(row(port.ScopeAgent, "*", port.ModeDeny), "x"), true},
		{"an agent row naming no agent", row(port.ScopeAgent, "*", port.ModeDeny), true},
		{"a user row naming no agent", row(port.ScopeUser, "*", port.ModeDeny), true},
	} {
		err := grant.Validate(tc.g, grant.Policy{})
		if (err == nil) != tc.ok || (err != nil && !port.IsCode(err, port.CodeRowRefused)) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestNormalizeKeepsTheAgentOfASessionRowOnly(t *testing.T) {
	session := named(row(port.ScopeSession, "fn:f", port.ModeAllow), "a")
	if got := grant.Normalize(session).Agent; got != "a" {
		t.Errorf("session row: agent %q", got)
	}
	for _, scope := range []port.ScopeKind{port.ScopeTenant, port.ScopeWorkspace, port.ScopeAgent, port.ScopeUser} {
		if got := grant.Normalize(named(row(scope, "*", port.ModeDeny), "a")).Agent; got != "" {
			t.Errorf("%s row: agent %q", scope, got)
		}
	}
}

func TestCanonicalKeepsTheRowThatDecidesEveryInstant(t *testing.T) {
	forever := named(row(port.ScopeUser, "*", port.ModeDeny), "a")
	forever.ID = "g2"
	brief := row(port.ScopeUser, "*", port.ModeAllow)
	brief.ID, brief.ExpiresAt = "g1", now.Add(time.Hour)
	other := row(port.ScopeUser, "class:read", port.ModeAllow)
	other.ID = "g3"
	got := grant.Canonical([]port.Grant{brief, forever, other}, now)
	if len(got) != 2 || got[0].ID != "g2" || got[0].Agent != "" || got[0].Mode != port.ModeDeny || got[1].ID != "g3" {
		t.Fatalf("got %+v", got)
	}
}

func TestCanonicalMergesRowsNoneOfWhichDecidesIntoADeny(t *testing.T) {
	soon := named(row(port.ScopeUser, "*", port.ModeDeny), "a")
	soon.ID, soon.ExpiresAt, soon.Origin = "g1", now.Add(time.Hour), port.OriginLevel
	later := row(port.ScopeUser, "*", port.ModeAllow)
	later.ID, later.ExpiresAt = "g2", now.Add(2*time.Hour)
	got := grant.Canonical([]port.Grant{soon, later}, now)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	g := got[0]
	if g.ID != "g1" || g.Agent != "" || g.Mode != port.ModeDeny || !g.ExpiresAt.IsZero() || g.MaxUses != 0 || g.Origin != port.OriginSettings {
		t.Errorf("got %+v", g)
	}
}

func TestCanonicalNeverKeepsACountedRowOfACollision(t *testing.T) {
	a := named(row(port.ScopeUser, "*", port.ModeAllow), "a")
	a.ID, a.MaxUses = "g1", 2
	b := a
	b.ID, b.Agent = "g2", "b"
	got := grant.Canonical([]port.Grant{a, b}, now)
	if len(got) != 1 || got[0].Mode != port.ModeDeny {
		t.Errorf("got %+v", got)
	}
}

func TestCanonicalKeepsOneOfTwoAskRowsAtDifferentHashes(t *testing.T) {
	a := named(row(port.ScopeAgent, "fn:f", port.ModeAsk), "x")
	a.ID = "g1"
	b := row(port.ScopeAgent, "fn:f", port.ModeAsk)
	b.ID, b.DefinitionHash = "g2", "h0"
	got := grant.Canonical([]port.Grant{b, a}, now)
	if len(got) != 1 || got[0].ID != "g1" || got[0].Mode != port.ModeAsk {
		t.Errorf("got %+v", got)
	}
}

func TestCanonicalLeavesRowsDeadAtTheRewriteOutOfAMerge(t *testing.T) {
	dead := named(row(port.ScopeAgent, "fn:f", port.ModeDeny), "x")
	dead.ID, dead.ExpiresAt = "g1", now.Add(-24*time.Hour)
	used := named(row(port.ScopeAgent, "fn:f", port.ModeAllow), "x2")
	used.ID, used.MaxUses, used.Uses = "g2", 1, 1
	allow := row(port.ScopeAgent, "fn:f", port.ModeAllow)
	allow.ID = "g3"
	got := grant.Canonical([]port.Grant{dead, used, allow}, now)
	if len(got) != 1 || got[0].ID != "g3" || got[0].Mode != port.ModeAllow || got[0].Reason != "" {
		t.Errorf("got %+v", got)
	}
}

func TestTheDenyOfAConflictBelongsToTheSystem(t *testing.T) {
	a := named(row(port.ScopeUser, "*", port.ModeAllow), "a")
	a.ID, a.MaxUses, a.CreatedBy, a.Reason = "g1", 2, "alice", "trusted"
	b := a
	b.ID, b.Agent = "g2", "b"
	got := grant.Canonical([]port.Grant{a, b}, now)
	if len(got) != 1 || got[0].CreatedBy != "system" || got[0].Reason != grant.ConflictReason || !got[0].CreatedAt.Equal(now) {
		t.Errorf("got %+v", got)
	}
}
