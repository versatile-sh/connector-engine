package catalog_test

import (
	"testing"

	"github.com/agents-control/connector-engine/core/catalog"
	"github.com/agents-control/connector-engine/port"
)

func TestCeilingMode(t *testing.T) {
	want := map[port.Ceiling][3]port.Mode{
		port.CeilingRead:  {port.ModeAllow, port.ModeDeny, port.ModeDeny},
		port.CeilingWrite: {port.ModeAllow, port.ModeAskOnce, port.ModeAsk},
		port.CeilingFull:  {port.ModeAllow, port.ModeAllow, port.ModeAllow},
	}
	for c, modes := range want {
		for i, class := range []port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive} {
			if got := catalog.CeilingMode(c, class); got != modes[i] {
				t.Errorf("§5.5 V7: ceiling %s, class %s: %s, want %s", c, class, got, modes[i])
			}
		}
		if catalog.CeilingMode(c, port.ClassNone) != modes[2] {
			t.Errorf("§5.2: an unclassified function follows the destructive column under %s", c)
		}
	}
}

func TestRulesAndPolicies(t *testing.T) {
	if catalog.ValidatePolicy(port.SubjectTenant) == nil || catalog.ValidatePolicy(port.SubjectWorkspace) != nil {
		t.Fatal("§5.5: a policy names a workspace, never the tenant")
	}
	r := port.CatalogRule{Tenant: "t", SubjectKind: port.SubjectGroup, SubjectID: "g", TargetKind: port.TargetListing, Target: "/a", Ceiling: port.CeilingRead, Audience: port.AudienceNamed}
	if catalog.ValidateRule(r) == nil {
		t.Fatal("§5.5 V8: a named audience is on tenant and workspace rules only")
	}
	r.SubjectKind = port.SubjectWorkspace
	if err := catalog.ValidateRule(r); err != nil {
		t.Fatal(err)
	}
	rules := catalog.StarterRules("t9", []port.Listing{
		{Ref: port.ListingRef{Name: "b"}, Starter: true}, {Ref: port.ListingRef{Name: "a"}, Starter: true},
		{Ref: port.ListingRef{Name: "c"}}, {Ref: port.ListingRef{Owner: "t1", Name: "p"}, Starter: true},
	})
	if len(rules) != 2 || rules[0].Target != "/a" || rules[1].Ceiling != port.CeilingRead || rules[1].CreatedBy != port.StarterCreator {
		t.Fatalf("§5.5 V2: starter rules %+v", rules)
	}
}
