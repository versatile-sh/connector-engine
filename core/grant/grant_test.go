package grant_test

import (
	"testing"
	"time"

	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/port"
)

var now = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func row(scope port.ScopeKind, sel string, mode port.Mode) port.Grant {
	g := port.Grant{Tenant: "t", ScopeKind: scope, ScopeID: "x", Connector: "c", Selector: sel, Mode: mode, Origin: port.OriginSettings}
	if grant.Specificity(sel) == 2 {
		g.DefinitionHash = "h1"
	}
	return g
}

func TestValidate(t *testing.T) {
	session := row(port.ScopeSession, "fn:f", port.ModeAllow)
	session.Agent, session.ExpiresAt, session.Origin = "a", now.Add(time.Hour), port.OriginApproval
	noAgent := session
	noAgent.Agent = ""
	noHash := row(port.ScopeAgent, "fn:f", port.ModeAllow)
	noHash.DefinitionHash = ""
	forever := row(port.ScopeAgent, "*", port.ModeAllow)
	bounded := forever
	bounded.ExpiresAt = now.Add(24 * time.Hour)
	for _, tc := range []struct {
		name string
		g    port.Grant
		p    grant.Policy
		ok   bool
	}{
		{"G5: a session row written by an approval", session, grant.Policy{}, true},
		{"G5: a session row carries its agent", noAgent, grant.Policy{}, false},
		{"an fn: row below the tenant carries the hash the grantor saw", noHash, grant.Policy{}, false},
		{"unknown selector", row(port.ScopeAgent, "class:other", port.ModeAllow), grant.Policy{}, false},
		{"an allow row of an agent written in settings is bounded by permission.alwaysMaxDuration", forever, grant.Policy{Now: now, AlwaysMax: 90 * 24 * time.Hour}, false},
		{"a bounded allow row", bounded, grant.Policy{Now: now, AlwaysMax: 90 * 24 * time.Hour}, true},
		{"a workspace allow row needs no expiry", row(port.ScopeWorkspace, "*", port.ModeAllow), grant.Policy{Now: now, AlwaysMax: time.Hour}, true},
	} {
		if err := grant.Validate(tc.g, tc.p); (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestSetLevel(t *testing.T) {
	layer := []port.Grant{
		row(port.ScopeAgent, "class:write", port.ModeAllow),
		row(port.ScopeAgent, "fn:f", port.ModeAsk),
		row(port.ScopeAgent, "*", port.ModeDeny),
		{Tenant: "t", ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "c", Selector: "class:read", Mode: port.ModeDeny, Origin: port.OriginLevel},
	}
	base := port.Grant{Tenant: "t", ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "c"}
	removed, added := grant.SetLevel(layer, base, port.LevelRead, grant.DefaultLevelModes)
	if len(removed) != 2 || removed[0].Selector != "class:write" || removed[1].Selector != "class:read" {
		t.Fatalf("G3: removed %+v, want the hand-written class:write (same key) and the level row", removed)
	}
	want := map[string]port.Mode{"class:read": port.ModeAllow, "class:write": port.ModeDeny, "class:destructive": port.ModeDeny}
	for _, g := range added {
		if want[g.Selector] != g.Mode || g.Origin != port.OriginLevel {
			t.Fatalf("level read: %+v", g)
		}
	}
	if _, added := grant.SetLevel(layer, base, port.LevelCustom, grant.DefaultLevelModes); len(added) != 0 {
		t.Fatal("G3: level custom writes no row")
	}
}

func TestMatchTenantRowIsNeverStale(t *testing.T) {
	ref := port.ListingRef{Name: "svc"}
	tenantRow := port.Grant{Tenant: "t", ScopeKind: port.ScopeTenant, ScopeID: "t", Listing: &ref, Selector: "fn:f", Mode: port.ModeAskOnce}
	if err := grant.Validate(tenantRow, grant.Policy{}); err != nil {
		t.Fatal(err)
	}
	a := grant.Match([]port.Grant{tenantRow}, port.Function{Name: "f", DefinitionHash: "h2"}, now)
	if a.Mode != port.ModeAskOnce || a.Reason != "" {
		t.Fatalf("G8: M4 never applies to a tenant row: %+v", a)
	}
	if _, err := grant.Confirm(tenantRow, port.Function{Name: "f", DefinitionHash: "h2"}); err == nil {
		t.Fatal("G8: confirmGrant has nothing to confirm on a tenant row")
	}
}

func TestCapped(t *testing.T) {
	fns := []port.Function{{Name: "r", Class: port.ClassRead}, {Name: "d", Class: port.ClassDestructive}, {Name: "u"}}
	if got := grant.Capped(row(port.ScopeAgent, "*", port.ModeAllow), fns); len(got) != 2 || got[0] != "d" || got[1] != "u" {
		t.Fatalf("capped_by_destructive_max lists the destructive and unclassified functions: %v", got)
	}
}

func TestAnItemIsCoveredOnlyByItsNameOrItsKindWildcard(t *testing.T) {
	item := port.Function{Name: port.InstructionsItem, Class: port.ClassRead}
	tool := port.Function{Name: "f", Class: port.ClassRead}
	for _, tc := range []struct {
		sel        string
		item, tool bool
		spec       int
	}{
		{"*", false, true, 0},
		{"class:read", false, true, 1},
		{"fn:" + port.InstructionsItem, false, false, 2},
		{port.InstructionsItem, true, false, 2},
		{port.ItemInstructions + "*", true, false, 1},
	} {
		if grant.Covers(tc.sel, item) != tc.item || grant.Covers(tc.sel, tool) != tc.tool || grant.Specificity(tc.sel) != tc.spec {
			t.Errorf("%s: covers item %v, tool %v, specificity %d", tc.sel, grant.Covers(tc.sel, item), grant.Covers(tc.sel, tool), grant.Specificity(tc.sel))
		}
	}
	for sel, ok := range map[string]bool{port.InstructionsItem: true, port.ItemInstructions + "*": true, port.ItemInstructions: false} {
		if err := grant.Validate(row(port.ScopeWorkspace, sel, port.ModeAllow), grant.Policy{}); (err == nil) != ok {
			t.Errorf("validate %s: %v", sel, err)
		}
	}
}
