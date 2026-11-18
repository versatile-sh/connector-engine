package evaluate_test

import (
	"errors"
	"fmt"
	"testing"
)

func fixture() *Installation {
	i := NewInstallation()
	fns := []Function{
		{Name: "search", Hash: "a1", Class: Read},
		{Name: "create_issue", Hash: "b1", Class: Write},
		{Name: "delete_repo", Hash: "c1", Class: Destructive},
	}
	i.AddListing(&Listing{Name: "github", Source: "official", Publisher: "io.github", Functions: fns})
	i.AddListing(&Listing{Name: "slack", Source: "docker", Publisher: "com.slack", Functions: fns})
	i.AddListing(&Listing{Name: "notes", Source: "own", Publisher: "operator", Starter: true, Functions: fns})
	i.AddListing(&Listing{Owner: "t1", Name: "crm", Source: "private", Publisher: "t1", Functions: fns})
	return i
}

func show(tenant string, kind SubjectKind, subject string, target TargetKind, name string, ceiling Ceiling) *CatalogRule {
	return &CatalogRule{Tenant: tenant, SubjectKind: kind, SubjectID: subject, TargetKind: target, Target: name, Show: true, Install: true, Ceiling: ceiling}
}

func hide(tenant string, kind SubjectKind, subject string, target TargetKind, name string) *CatalogRule {
	return &CatalogRule{Tenant: tenant, SubjectKind: kind, SubjectID: subject, TargetKind: target, Target: name}
}

func open(t *testing.T, i *Installation, tenant, workspace, listing string) *World {
	t.Helper()
	if err := i.Install(tenant, workspace, "admin", listing, conn); err != nil {
		t.Fatalf("install %s in %s: %v", listing, tenant, err)
	}
	w := i.Tenant(tenant)
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: workspace, Connector: conn, Selector: "*", Mode: Allow})
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "*", Mode: Allow})
	if err := w.Bind(conn, "x"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	return w
}

func same(t *testing.T, where string, got []string, want ...string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s: got %v, want %v", where, got, want)
	}
}

func TestListingHiddenForTheTenant(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	same(t, "03-engine §5.5 V2: the store of a tenant shows only what its admin enabled", i.Store("t1", "w", "admin", ""), "/github", "t1/crm")
	same(t, "03-engine §5.5: search", i.Store("t1", "w", "admin", "slack"))
	if _, err := i.GetListing("t1", "w", "admin", "/slack"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("03-engine §5.5: getListing of a hidden listing: %v, want not_found", err)
	}
	if err := i.Install("t1", "w", "admin", "/slack", "sl"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("03-engine §5.5: install of a hidden listing: %v, want not_found", err)
	}
	w := open(t, i, "t1", "w", "/github")
	wantList(t, "before the rule is withdrawn", w, "x", "s1", "s__create_issue", "s__delete_repo", "s__search")
	i.Rules = i.Rules[:len(i.Rules)-1]
	wantList(t, "03-engine §7.2 row 48: tools/list after the tenant withdrew the listing", w, "x", "s1")
	same(t, "03-engine §7.2 row 48: find", w.Find("x", "", "s1", Model, 10, "search"))
	if d := wantCall(t, "03-engine §7.2 row 48: tools/call", w, "x", "s1", "search", UnknownTool); d.Reason != "not_in_catalog" || d.Step != 2 {
		t.Fatalf("03-engine §7.1 step 2: reason %s at step %d, want not_in_catalog at step 2", d.Reason, d.Step)
	}
	if err := i.Install("t1", "w", "admin", "/github", "again"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("03-engine §5.5: install after the withdrawal: %v", err)
	}
}

func TestWorkspaceCannotWidenATenantCeiling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ceiling Ceiling
		want    map[string]Outcome
		modes   map[string]Mode
	}{
		{"03-engine §7.2 rows 49 and 50: tenant ceiling read", CeilRead,
			map[string]Outcome{"search": Forwarded, "create_issue": UnknownTool, "delete_repo": UnknownTool},
			map[string]Mode{"search": Allow, "create_issue": Deny, "delete_repo": Deny}},
		{"03-engine §7.2 row 51: tenant ceiling write", CeilWrite,
			map[string]Outcome{"search": Forwarded, "create_issue": Held, "delete_repo": Held},
			map[string]Mode{"search": Allow, "create_issue": AskOnce, "delete_repo": Ask}},
		{"03-engine §7.2 row 52: tenant ceiling full", CeilFull,
			map[string]Outcome{"search": Forwarded, "create_issue": Forwarded, "delete_repo": Held},
			map[string]Mode{"search": Allow, "create_issue": Allow, "delete_repo": AskOnce}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := fixture()
			i.Rules = append(i.Rules,
				show("t1", TenantSubject, "t1", ListingTarget, "/github", tc.ceiling),
				show("t1", WorkspaceSubject, "w", ListingTarget, "/github", CeilFull))
			w := open(t, i, "t1", "w", "/github")
			for f, want := range tc.want {
				d := wantCall(t, "§7.1 step 11: workspace * allow and agent * allow under the ceiling", w, "x", "s1", f, want)
				if d.Mode != tc.modes[f] {
					t.Fatalf("%s: %v, want %v", f, d.Mode, tc.modes[f])
				}
			}
		})
	}
}

func TestTenantGrantLayer(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	w := open(t, i, "t1", "w", "/github")
	wantCall(t, "03-engine §7.1 step 6: no tenant row, the layer is skipped", w, "x", "s1", "create_issue", Forwarded)
	w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: "fn:delete_repo", Mode: Deny})
	if d := wantCall(t, "03-engine §7.2 row 53: tenant fn:F deny", w, "x", "s1", "delete_repo", UnknownTool); d.Step != 6 {
		t.Fatalf("step %d, want 6", d.Step)
	}
	if d := wantCall(t, "03-engine §7.2 row 57, 02-options §16.6: a tenant fn: deny row leaves the other functions of the listing alone", w, "x", "s1", "create_issue", Forwarded); d.Layers[0] != "skip" {
		t.Fatalf("tenant layer %s, want skip (03-engine §7.1 step 6: no tenant row matches F)", d.Layers[0])
	}
	wantCall(t, "03-engine §7.2 row 57: the same for a read function", w, "x", "s1", "search", Forwarded)
	if err := w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: "*", Mode: Ask}); err != nil {
		t.Fatal(err)
	}
	wantCall(t, "03-engine §7.1 step 10: tenant * ask under workspace and agent allow", w, "x", "s1", "create_issue", Held)
	wantCall(t, "03-engine §7.1 M2: the tenant fn: row is more specific than its * row", w, "x", "s1", "delete_repo", UnknownTool)
}

func TestTenantRowAcrossWorkspacesAndVersions(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	w := open(t, i, "t1", "w", "/github")
	if err := i.Install("t1", "lab", "admin", "/github", "lab-gh"); err != nil {
		t.Fatal(err)
	}
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: "lab", Connector: "lab-gh", Selector: "*", Mode: Allow})
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: "lab-gh", Selector: "*", Mode: Allow})
	if err := w.Bind("lab-gh", "x"); err != nil {
		t.Fatal(err)
	}
	if err := w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: "fn:create_issue", Mode: Ask}); err != nil {
		t.Fatal(err)
	}
	w.Update("lab-gh", "2.0.0", []Function{{Name: "create_issue", Hash: "b2", Class: Write}, {Name: "search", Hash: "a1", Class: Read}})
	w.Accept("lab-gh", "create_issue")
	at := func(c string) Decision {
		_, d := w.Call(Request{Agent: "x", Session: "s1", Connector: c, Function: "create_issue", Now: 10})
		return d
	}
	if w.Connectors[conn].Functions["create_issue"].Hash == w.Connectors["lab-gh"].Functions["create_issue"].Hash {
		t.Fatal("the two workspaces must pin different hashes for this case")
	}
	for _, c := range []string{conn, "lab-gh"} {
		if d := at(c); d.Mode != Ask || d.Layers[0] != "ask" {
			t.Fatalf("03-engine §6.4 G8, §7.2 row 58: a tenant fn: ask row answers in %s whatever hash that workspace pins: %v %s", c, d.Mode, d.Reason)
		}
	}
	w.ConfirmGrant(Tenant, "t1", "lab-gh", "create_issue")
	for _, g := range w.Grants {
		if g.Scope == Tenant && g.Hash != "" {
			t.Fatal("03-engine §6.4 G8: a tenant row has no hash and confirmGrant has nothing to confirm on it")
		}
	}
	i.Rules = append(i.Rules, hide("t1", WorkspaceSubject, "lab", ListingTarget, "/github"))
	w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: "fn:delete_repo", Mode: Deny})
	w.Update(conn, "2.0.0", []Function{{Name: "delete_repo", Hash: "c2", Class: Destructive}})
	w.Accept(conn, "delete_repo")
	if d := wantCall(t, "03-engine §5.1, §7.2 row 59: a tenant deny row names (owner, name) and survives the update of a connector", w, "x", "s1", "delete_repo", UnknownTool); d.Step != 6 {
		t.Fatalf("step %d, want 6", d.Step)
	}
	if out, d := w.Call(Request{Agent: "x", Session: "s1", Connector: "lab-gh", Function: "search", Now: 10}); out != UnknownTool || d.Reason != "not_in_catalog" {
		t.Fatalf("03-engine §5.1: a hide rule names (owner, name) and survives the update to version %s: %v %s", w.Connectors["lab-gh"].Version, out, d.Reason)
	}
}

func TestCatalogPolicyOfATenantIsRefused(t *testing.T) {
	i := fixture()
	if err := i.PutCatalogPolicy("t1", TenantSubject, "t1", false); !errors.Is(err, ErrPolicyRefused) {
		t.Fatalf("03-engine §5.5: a catalog policy with subject tenant and default visible: %v, want refused", err)
	}
	same(t, "03-engine §5.5 V2: the tenant still sees only what a rule shows", i.Store("t1", "w", "admin", ""), "t1/crm")
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "official", CeilWrite))
	same(t, "03-engine §5.5: a whole source is opened by a rule, which states its ceiling", i.Store("t1", "w", "admin", ""), "/github", "t1/crm")
	if _, c, _ := i.Visible("t1", "w", nil, "/github"); c != CeilWrite {
		t.Fatalf("ceiling %v, want write", c)
	}
	if err := i.PutCatalogPolicy("t1", WorkspaceSubject, "lab", true); err != nil {
		t.Fatal(err)
	}
	same(t, "03-engine §5.5 V3: a workspace policy of hidden", i.Store("t1", "lab", "admin", ""))
}

func TestListingStatus(t *testing.T) {
	setup := func() (*Installation, *World, *World) {
		i := fixture()
		for _, tenant := range []string{"t1", "t2"} {
			i.Rules = append(i.Rules, show(tenant, TenantSubject, tenant, ListingTarget, "/github", CeilFull), show(tenant, TenantSubject, tenant, ListingTarget, "/slack", CeilFull))
			open(t, i, tenant, "w", "/github")
		}
		return i, i.Tenant("t1"), i.Tenant("t2")
	}
	t.Run("03-engine §5.5 V1, §7.2 row 60: the operator quarantines one listing", func(t *testing.T) {
		i, w1, w2 := setup()
		if out, _ := call(w1, "x", "s1", "create_issue"); out != Forwarded {
			t.Fatal("before the quarantine")
		}
		i.Listings["/github"].Status = ListingQuarantined
		for _, w := range []*World{w1, w2} {
			if d := wantCall(t, "an installed connector of a quarantined listing", w, "x", "s1", "search", UnknownTool); d.Reason != "listing_quarantined" || d.Step != 2 {
				t.Fatalf("reason %s at step %d, want listing_quarantined at step 2", d.Reason, d.Step)
			}
			wantList(t, "tools/list", w, "x", "s1")
		}
		same(t, "the store", i.Store("t1", "w", "admin", ""), "/slack", "t1/crm")
		if err := i.Install("t1", "w", "admin", "/github", "again"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("install of a quarantined listing: %v", err)
		}
		if _, ok := w1.Connectors[conn]; !ok || !w1.Bindings[[2]string{conn, "x"}] {
			t.Fatal("the connector and its binding stay")
		}
		i.Listings["/github"].Status = ListingActive
		wantCall(t, "restoreListing: the connector works again", w1, "x", "s1", "search", Forwarded)
	})
	t.Run("03-engine §5.5 V1: a deprecated listing keeps working and takes no new install", func(t *testing.T) {
		i, w1, _ := setup()
		i.Listings["/github"].Status = ListingDeprecated
		wantCall(t, "an installed connector", w1, "x", "s1", "search", Forwarded)
		same(t, "the store still shows it", i.Store("t1", "w", "admin", "github"), "/github")
		if err := i.Install("t1", "w", "admin", "/github", "again"); !errors.Is(err, ErrDeprecated) {
			t.Fatalf("install of a deprecated listing: %v, want listing_deprecated", err)
		}
	})
	t.Run("03-engine §5.5 V1, §7.2 row 61: the operator withdraws one listing from one tenant", func(t *testing.T) {
		i, w1, w2 := setup()
		i.WithheldListing[[2]string{"t1", "/github"}] = true
		if d := wantCall(t, "the tenant it was withdrawn from", w1, "x", "s1", "search", UnknownTool); d.Reason != "not_in_catalog" {
			t.Fatalf("reason %s", d.Reason)
		}
		same(t, "its store: the rest of the source stays", i.Store("t1", "w", "admin", ""), "/slack", "t1/crm")
		wantCall(t, "another tenant", w2, "x", "s1", "search", Forwarded)
		i.Rules = append(i.Rules, show("t1", WorkspaceSubject, "w", ListingTarget, "/github", CeilFull))
		wantCall(t, "no rule of the tenant brings it back", w1, "x", "s1", "search", UnknownTool)
	})
	t.Run("03-engine §5.5 V1: the operator withdraws one listing from everyone", func(t *testing.T) {
		i, w1, w2 := setup()
		i.Listings["/github"].Status = ListingWithdrawn
		for _, w := range []*World{w1, w2} {
			if d := wantCall(t, "withdrawn", w, "x", "s1", "search", UnknownTool); d.Reason != "not_in_catalog" {
				t.Fatalf("reason %s", d.Reason)
			}
		}
	})
}

func TestPrivateListingWithTheNameOfAnOperatorListing(t *testing.T) {
	i := fixture()
	i.AddListing(&Listing{Owner: "t1", Name: "github", Source: "official", Publisher: "io.github", Functions: []Function{{Name: "search", Hash: "p1", Class: Read}}})
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	same(t, "03-engine §5.1: two listings of one name are two entries, told apart by the owner", i.Store("t1", "w", "admin", "github"), "/github", "t1/github")
	same(t, "the other tenant sees neither", i.Store("t2", "w", "admin", "github"))
	w := open(t, i, "t1", "w", "/github")
	if err := i.Install("t1", "w", "admin", "t1/github", "mine"); err != nil {
		t.Fatal(err)
	}
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: "w", Connector: "mine", Selector: "*", Mode: Allow})
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: "mine", Selector: "*", Mode: Allow})
	if err := w.Bind("mine", "x"); err != nil {
		t.Fatal(err)
	}
	private := func() Outcome {
		out, _ := w.Call(Request{Agent: "x", Session: "s1", Connector: "mine", Function: "search", Now: 10})
		return out
	}
	w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: "*", Mode: Deny})
	wantCall(t, "03-engine §7.2 row 62: a tenant row on the operator listing", w, "x", "s1", "search", UnknownTool)
	if private() != Forwarded {
		t.Fatal("03-engine §5.1, §7.2 row 62: a tenant row on (installation, github) reached the private listing (t1, github)")
	}
	i.Rules = append(i.Rules, hide("t1", TenantSubject, "t1", PublisherTarget, "io.github"), hide("t1", TenantSubject, "t1", SourceTarget, "official"))
	if private() != Forwarded {
		t.Fatal("03-engine §5.5 V5: a rule on a publisher or a source covered a private listing, which declares both itself")
	}
	i.Rules = append(i.Rules, hide("t1", TenantSubject, "t1", ListingTarget, "t1/github"))
	if private() != UnknownTool {
		t.Fatal("a rule that names the private listing by (owner, name) hides it")
	}
}

func TestPersonRemovedWhileRequesterOfAPendingApproval(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	w := open(t, i, "t1", "w", "/github")
	w.AgentOwner["a1"] = "u1"
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "a1", Connector: conn, Selector: "*", Mode: Allow})
	if err := w.Bind(conn, "a1"); err != nil {
		t.Fatal(err)
	}
	w.PutGrant(&Grant{Scope: User, ScopeID: "u1", Connector: conn, Selector: "*", Mode: Ask})
	r := Request{Agent: "x", User: "u1", Session: "s1", Connector: conn, Function: "create_issue", Now: 10}
	if out, _ := w.Call(r); out != Held {
		t.Fatalf("the call for u1 is held: %v", out)
	}
	w.RemovePerson("u1")
	if out, d := w.ExecuteApproved(r); out != UnknownTool || d.Reason != "person_removed" || d.Step != 1 {
		t.Fatalf("03-engine §8.2 A9, §7.2 row 63: the re-check of an approval whose requester was removed: %v %s at step %d", out, d.Reason, d.Step)
	}
	if out, d := w.Call(r); out != UnknownTool || d.Reason != "person_removed" {
		t.Fatalf("03-engine §7.1 step 1: a run that still acts for the removed person: %v %s", out, d.Reason)
	}
	if d := wantCall(t, "03-engine §7.1 step 1: an agent the removed person owns", w, "a1", "s2", "search", UnknownTool); d.Reason != "person_removed" {
		t.Fatalf("reason %s", d.Reason)
	}
	wantCall(t, "the workspace agent, for nobody", w, "x", "s1", "create_issue", Forwarded)
}

func TestListingHiddenWithBindingsAndPendingApprovals(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilWrite))
	w := open(t, i, "t1", "w", "/github")
	r := Request{Agent: "x", Session: "s1", Connector: conn, Function: "create_issue", Now: 10, TokenExp: 5000}
	if out, d := w.Call(r); out != Held || d.Mode != AskOnce {
		t.Fatalf("held under ceiling write: %v %v", out, d.Mode)
	}
	if !w.ApproveForSession(r) {
		t.Fatal("approve for the session")
	}
	i.Rules = append(i.Rules, hide("t1", WorkspaceSubject, "w", ListingTarget, "/github"))
	if out, d := w.ExecuteApproved(r); out != UnknownTool || d.Reason != "not_in_catalog" {
		t.Fatalf("03-engine §8.2 A9: the re-check of an approved record after the listing was hidden: %v %s", out, d.Reason)
	}
	wantList(t, "03-engine §5.5: the list", w, "x", "s1")
	if !w.Bindings[[2]string{conn, "x"}] || !w.sessionRow(r, w.Connectors[conn].Functions["create_issue"]) {
		t.Fatal("03-engine §5.5: the binding and the session row stay while the listing is hidden")
	}
	i.Rules = i.Rules[:len(i.Rules)-1]
	wantCall(t, "03-engine §5.5: the rule returns and the session row answers again", w, "x", "s1", "create_issue", Forwarded)
}

func TestCatalogRuleRankAndTenantFilter(t *testing.T) {
	t.Run("03-engine §5.5 V5: a listing rule on a group answers before a publisher rule on the person", func(t *testing.T) {
		i := fixture()
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		i.Tenant("t1").Groups["u1"] = []string{"g1"}
		i.Rules = append(i.Rules, show("t1", UserSubject, "u1", PublisherTarget, "io.github", CeilFull), hide("t1", GroupSubject, "g1", ListingTarget, "/github"))
		same(t, "the more specific target hides, although it is a group's", i.Store("t1", "w", "u1", "github"))
		i.Rules = i.Rules[:len(i.Rules)-2]
		i.Rules = append(i.Rules, hide("t1", UserSubject, "u1", SourceTarget, "official"), show("t1", GroupSubject, "g1", ListingTarget, "/github", CeilRead))
		same(t, "the more specific target shows, although it is a group's (stated consequence of V5)", i.Store("t1", "w", "u1", "github"), "/github")
		if _, c, _ := i.Visible("t1", "w", []string{"u1"}, "/github"); c != CeilRead {
			t.Fatalf("ceiling %v, want the group rule's read", c)
		}
	})
	t.Run("03-engine §4.1 H3: two tenants hold a workspace, a group and a person of the same name", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			rule *CatalogRule
		}{
			{"workspace w", hide("t1", WorkspaceSubject, "w", ListingTarget, "/github")},
			{"group g1", hide("t1", GroupSubject, "g1", ListingTarget, "/github")},
			{"person u1", hide("t1", UserSubject, "u1", ListingTarget, "/github")},
		} {
			i := fixture()
			for _, tenant := range []string{"t1", "t2"} {
				i.Rules = append(i.Rules, show(tenant, TenantSubject, tenant, ListingTarget, "/github", CeilFull))
				w := open(t, i, tenant, "w", "/github")
				w.Groups["u1"] = []string{"g1"}
			}
			i.Rules = append(i.Rules, tc.rule)
			r := Request{Agent: "x", User: "u1", Session: "s1", Connector: conn, Function: "search", Now: 10}
			if out, _ := i.Tenant("t1").Call(r); out != UnknownTool {
				t.Fatalf("the rule of t1 on %s did not hide the listing in t1", tc.name)
			}
			if out, _ := i.Tenant("t2").Call(r); out != Forwarded {
				t.Fatalf("a rule of tenant t1 on %s hid the listing from %s of tenant t2", tc.name, tc.name)
			}
			same(t, "the store of t2", i.Store("t2", "w", "u1", "github"), "/github")
		}
	})
}

func TestListingShownOnlyToNamedSubjects(t *testing.T) {
	named := func(r *CatalogRule) *CatalogRule { r.Named = true; return r }
	i := fixture()
	i.Rules = append(i.Rules, named(show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilWrite)))
	w := open2(t, i)
	w.Groups["u1"], w.Groups["u2"] = []string{"g1"}, []string{"g2"}
	same(t, "03-engine §5.5 V8: nobody is named yet", i.Store("t1", "w", "u1", "github"))
	i.Rules = append(i.Rules, show("t1", GroupSubject, "g1", ListingTarget, "/github", CeilFull))
	same(t, "03-engine §5.5 V8: a member of the named group", i.Store("t1", "w", "u1", "github"), "/github")
	same(t, "03-engine §5.5 V8: a person nobody named does not inherit", i.Store("t1", "w", "u2", "github"))
	if _, c, _ := i.Visible("t1", "w", []string{"u1"}, "/github"); c != CeilWrite {
		t.Fatalf("03-engine §5.5 V6: naming a group does not raise the tenant ceiling: %v", c)
	}
	for u, want := range map[string]Outcome{"u1": Forwarded, "u2": UnknownTool, "": UnknownTool} {
		if out, _ := w.Call(Request{Agent: "x", User: u, Session: "s1", Connector: conn, Function: "search", Now: 10}); out != want {
			t.Fatalf("03-engine §7.2 row 64: a workspace agent acting for %q: %v, want %v", u, out, want)
		}
	}
	i.Rules = append(i.Rules, show("t1", WorkspaceSubject, "w", ListingTarget, "/github", CeilFull))
	same(t, "03-engine §5.5 V8: naming the workspace shows it to its members", i.Store("t1", "w", "u2", "github"), "/github")
	i.Rules[len(i.Rules)-1].Named = true
	same(t, "03-engine §5.5 V8: a workspace rule for named people only", i.Store("t1", "w", "u2", "github"))
	same(t, "the named group still sees it", i.Store("t1", "w", "u1", "github"), "/github")
}

func open2(t *testing.T, i *Installation) *World {
	t.Helper()
	w := i.Tenant("t1")
	fns := map[string]*Function{}
	for _, f := range i.Listings["/github"].Functions {
		f := f
		fns[f.Name] = &f
	}
	w.Connectors[conn] = &Connector{ID: conn, Listing: "/github", Workspace: "w", Enabled: true, Functions: fns}
	w.Bindings[[2]string{conn, "x"}] = true
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: "w", Connector: conn, Selector: "*", Mode: Allow})
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "*", Mode: Allow})
	return w
}

func TestAgentOfAUserWhoMayNotSeeTheListing(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	w := open(t, i, "t1", "w", "/github")
	w.AgentOwner["a1"], w.AgentOwner["a2"] = "u1", "u2"
	w.Groups["u3"] = []string{"interns"}
	for _, a := range []string{"a1", "a2"} {
		w.PutGrant(&Grant{Scope: Agent, ScopeID: a, Connector: conn, Selector: "*", Mode: Allow})
		if err := w.Bind(conn, a); err != nil {
			t.Fatal(err)
		}
	}
	i.Rules = append(i.Rules,
		hide("t1", UserSubject, "u1", ListingTarget, "/github"),
		hide("t1", GroupSubject, "interns", SourceTarget, "official"))
	same(t, "03-engine §5.5 V4: the store for u1", i.Store("t1", "w", "u1", ""), "t1/crm")
	same(t, "03-engine §5.5 V4: the store for u2", i.Store("t1", "w", "u2", ""), "/github", "t1/crm")
	same(t, "03-engine §5.5 V4: the store for u3, hidden through a group", i.Store("t1", "w", "u3", ""), "t1/crm")
	wantList(t, "03-engine §7.2 row 54: tools/list of the agent u1 owns", w, "a1", "s1")
	if d := wantCall(t, "03-engine §7.2 row 54: tools/call by the agent u1 owns", w, "a1", "s1", "search", UnknownTool); d.Reason != "not_in_catalog" {
		t.Fatalf("reason %s", d.Reason)
	}
	wantCall(t, "the agent u2 owns", w, "a2", "s1", "search", Forwarded)
	wantCall(t, "the workspace agent, for nobody", w, "x", "s1", "search", Forwarded)
	for _, u := range []string{"u1", "u3"} {
		r := Request{Agent: "x", User: u, Session: "s1", Connector: conn, Function: "search", Now: 10}
		if out, d := w.Call(r); out != UnknownTool || d.Reason != "not_in_catalog" {
			t.Fatalf("03-engine §7.2 row 55: the workspace agent acting for %s: %v %s", u, out, d.Reason)
		}
		if listed(w, r) {
			t.Fatalf("03-engine §7.2 row 55: listed for %s", u)
		}
	}
	delete(w.Bindings, [2]string{conn, "a1"})
	if err := w.Bind(conn, "a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("03-engine §5.5: binding the agent of u1 to a connector u1 may not see: %v, want not_found", err)
	}
	if err := i.Install("t1", "w", "u1", "/github", "mine"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("03-engine §5.5: install by u1: %v, want not_found", err)
	}
}

func TestTwoTenantsWithTheSameNames(t *testing.T) {
	i := fixture()
	for _, tenant := range []string{"t1", "t2"} {
		i.Rules = append(i.Rules, show(tenant, TenantSubject, tenant, ListingTarget, "/github", CeilFull))
		if err := i.Install(tenant, "w", "admin", "/github", conn); err != nil {
			t.Fatal(err)
		}
		if err := i.Tenant(tenant).Bind(conn, "x"); err != nil {
			t.Fatal(err)
		}
	}
	w1, w2 := i.Tenant("t1"), i.Tenant("t2")
	w1.PutGrant(&Grant{Scope: Workspace, ScopeID: "w", Connector: conn, Selector: "*", Mode: Allow})
	w1.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "*", Mode: Allow})
	wantList(t, "03-engine §15.3 tenancy: tenant t1", w1, "x", "s1", "s__create_issue", "s__delete_repo", "s__search")
	wantList(t, "03-engine §15.3 tenancy: tenant t2 has the same connector, agent and session names and no rows", w2, "x", "s1")
	wantCall(t, "tenant t2", w2, "x", "s1", "search", UnknownTool)
	w1.UpstreamChanged(conn, "search", "a2")
	if w2.Connectors[conn].Functions["search"].State != Active {
		t.Fatal("a quarantine in tenant t1 reached the connector of tenant t2")
	}
	i.Rules = append(i.Rules, hide("t1", TenantSubject, "t1", ListingTarget, "/github"))
	if show, _, _ := i.Visible("t2", "w", nil, "/github"); !show {
		t.Fatal("03-engine §5.5: a rule of tenant t1 changed what tenant t2 sees")
	}
	w2.PutGrant(&Grant{Scope: Workspace, ScopeID: "w", Connector: conn, Selector: "*", Mode: Allow})
	w2.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "*", Mode: Allow})
	wantCall(t, "tenant t1 after its own rule hid the listing", w1, "x", "s1", "create_issue", UnknownTool)
	wantCall(t, "tenant t2, untouched by the rule of t1", w2, "x", "s1", "create_issue", Forwarded)
}

func TestPrivateListingOfATenant(t *testing.T) {
	i := fixture()
	same(t, "03-engine §5.5 V1: the owner tenant sees its private listing", i.Store("t1", "w", "admin", "crm"), "t1/crm")
	same(t, "03-engine §5.5 V1: another tenant does not", i.Store("t2", "w", "admin", ""))
	i.Rules = append(i.Rules, show("t2", TenantSubject, "t2", ListingTarget, "t1/crm", CeilFull), show("t2", TenantSubject, "t2", SourceTarget, "private", CeilFull))
	same(t, "03-engine §5.5 V1: a rule of tenant t2 cannot reach the private listing of t1", i.Store("t2", "w", "admin", ""))
	if _, err := i.GetListing("t2", "w", "admin", "t1/crm"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("getListing from another tenant: %v, want not_found", err)
	}
	if err := i.Install("t2", "w", "admin", "t1/crm", "crm"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("install from another tenant: %v, want not_found", err)
	}
	if err := i.Install("t1", "w", "admin", "t1/crm", "crm"); err != nil {
		t.Fatalf("install by the owner tenant: %v", err)
	}
}

func TestCatalogResolution(t *testing.T) {
	t.Run("03-engine §5.5 V2: a new tenant sees nothing with catalog.tenantDefault = none", func(t *testing.T) {
		same(t, "store", fixture().Store("t9", "w", "admin", ""))
	})
	t.Run("03-engine §5.5 V2: catalog.tenantDefault = starter writes the starter set as rules of a tenant when it is created", func(t *testing.T) {
		i := fixture()
		i.TenantDefault = "starter"
		i.CreateTenant("t9")
		same(t, "store", i.Store("t9", "w", "admin", ""), "/notes")
		if _, c, _ := i.Visible("t9", "w", nil, "/notes"); c != CeilRead {
			t.Fatalf("ceiling %v, want read", c)
		}
		i.Listings["/github"].Starter = true
		same(t, "03-engine §5.5 V2: a listing marked starter afterwards is not enabled for a tenant that exists", i.Store("t9", "w", "admin", ""), "/notes")
		same(t, "a tenant that reached the engine without createTenant has no starter rule", i.Store("t8", "w", "admin", ""))
		i.CreateTenant("t10")
		same(t, "a tenant created after the mark gets it", i.Store("t10", "w", "admin", ""), "/github", "/notes")
		i.CreateTenant("t9")
		same(t, "createTenant on a tenant that exists writes nothing", i.Store("t9", "w", "admin", ""), "/notes")
		kept := i.Rules[:0]
		for _, r := range i.Rules {
			if r.Tenant != "t9" {
				kept = append(kept, r)
			}
		}
		i.Rules = kept
		same(t, "the starter set is ordinary rules: the tenant admin deletes one and it is gone", i.Store("t9", "w", "admin", ""))
		i.Listings["/notes"].Starter = false
		same(t, "removing the mark takes nothing from a tenant that has the rule", i.Store("t10", "w", "admin", ""), "/github", "/notes")
	})
	t.Run("03-engine §5.5 V1: the operator withholds a source and a tenant rule cannot bring it back", func(t *testing.T) {
		i := fixture()
		i.Withheld[[2]string{"t1", "official"}] = true
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "official", CeilFull), show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		same(t, "store", i.Store("t1", "w", "admin", "github"))
	})
	t.Run("03-engine §5.5 V5: a listing rule answers before a publisher rule and a source rule", func(t *testing.T) {
		i := fixture()
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "official", CeilFull), hide("t1", TenantSubject, "t1", ListingTarget, "/github"))
		same(t, "store", i.Store("t1", "w", "admin", "github"))
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "docker", CeilFull), hide("t1", TenantSubject, "t1", PublisherTarget, "com.slack"), show("t1", TenantSubject, "t1", ListingTarget, "/slack", CeilRead))
		same(t, "store", i.Store("t1", "w", "admin", "slack"), "/slack")
	})
	t.Run("03-engine §5.5 V5: equal rank, hide wins and the lower ceiling wins", func(t *testing.T) {
		i := fixture()
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		i.Tenant("t1").Groups["u1"] = []string{"g1", "g2"}
		i.Rules = append(i.Rules, show("t1", GroupSubject, "g1", ListingTarget, "/github", CeilRead), show("t1", GroupSubject, "g2", ListingTarget, "/github", CeilWrite))
		if _, c, _ := i.Visible("t1", "w", []string{"u1"}, "/github"); c != CeilRead {
			t.Fatalf("ceiling %v, want read", c)
		}
		i.Rules = append(i.Rules, hide("t1", GroupSubject, "g2", ListingTarget, "/github"))
		same(t, "store", i.Store("t1", "w", "u1", "github"))
		i.Rules = append(i.Rules, show("t1", UserSubject, "u1", ListingTarget, "/github", CeilFull))
		same(t, "03-engine §5.5 V5: a rule on the user answers before a rule on a group", i.Store("t1", "w", "u1", "github"), "/github")
	})
	t.Run("03-engine §5.5 V3: a workspace whose default is hidden sees only what a rule shows it", func(t *testing.T) {
		i := fixture()
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "official", CeilFull), show("t1", TenantSubject, "t1", SourceTarget, "docker", CeilFull))
		i.WorkspaceHidden[[2]string{"t1", "lab"}] = true
		i.Rules = append(i.Rules, show("t1", WorkspaceSubject, "lab", ListingTarget, "/slack", CeilFull))
		same(t, "workspace lab", i.Store("t1", "lab", "admin", ""), "/slack")
		same(t, "workspace w inherits the tenant", i.Store("t1", "w", "admin", ""), "/github", "/slack", "t1/crm")
		if err := i.Install("t1", "w", "admin", "/github", conn); err != nil {
			t.Fatal(err)
		}
		lab := i.Tenant("t1")
		lab.Connectors["moved"] = &Connector{ID: "moved", Listing: "/github", Workspace: "lab", Enabled: true, Functions: lab.Connectors[conn].Functions}
		lab.Bindings[[2]string{"moved", "x"}] = true
		lab.PutGrant(&Grant{Scope: Workspace, ScopeID: "lab", Connector: "moved", Selector: "*", Mode: Allow})
		lab.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: "moved", Selector: "*", Mode: Allow})
		if out, d := lab.Call(Request{Agent: "x", Session: "s1", Connector: "moved", Function: "search", Now: 10}); out != UnknownTool || d.Reason != "not_in_catalog" {
			t.Fatalf("03-engine §7.2 row 56: a connector in a workspace that may not see its listing: %v %s", out, d.Reason)
		}
	})
	t.Run("03-engine §5.5 V6: visible and not installable", func(t *testing.T) {
		i := fixture()
		r := show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull)
		r.Install = false
		i.Rules = append(i.Rules, r)
		same(t, "store", i.Store("t1", "w", "admin", "github"), "/github")
		if err := i.Install("t1", "w", "admin", "/github", conn); !errors.Is(err, ErrNotFound) {
			t.Fatalf("install: %v", err)
		}
	})
}

func TestCatalogSymmetrySweep(t *testing.T) {
	type rule struct {
		name string
		add  func(*Installation)
	}
	tenantRules := []rule{
		{"tenant: no rule", func(*Installation) {}},
		{"tenant: show full", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		}},
		{"tenant: show write", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilWrite))
		}},
		{"tenant: show read", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilRead))
		}},
		{"tenant: source shown, listing hidden", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", SourceTarget, "official", CeilFull), hide("t1", TenantSubject, "t1", ListingTarget, "/github"))
		}},
		{"operator withholds the source", func(i *Installation) {
			i.Withheld[[2]string{"t1", "official"}] = true
			i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		}},
	}
	lowerRules := []rule{
		{"no lower rule", func(*Installation) {}},
		{"workspace hides", func(i *Installation) {
			i.Rules = append(i.Rules, hide("t1", WorkspaceSubject, "w", ListingTarget, "/github"))
		}},
		{"workspace shows full", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", WorkspaceSubject, "w", ListingTarget, "/github", CeilFull))
		}},
		{"workspace ceiling read", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", WorkspaceSubject, "w", ListingTarget, "/github", CeilRead))
		}},
		{"user hides", func(i *Installation) {
			i.Rules = append(i.Rules, hide("t1", UserSubject, "u1", ListingTarget, "/github"))
		}},
		{"group hides", func(i *Installation) {
			i.Rules = append(i.Rules, hide("t1", GroupSubject, "g1", PublisherTarget, "io.github"))
		}},
		{"user shows full", func(i *Installation) {
			i.Rules = append(i.Rules, show("t1", UserSubject, "u1", ListingTarget, "/github", CeilFull))
		}},
	}
	rank := map[Ceiling]int{CeilRead: 0, CeilWrite: 1, CeilFull: 2}
	worlds := 0
	for _, tr := range tenantRules {
		for _, lr := range lowerRules {
			for _, owner := range []string{"", "u1"} {
				for _, behalf := range []string{"", "u1", "u2"} {
					i := fixture()
					i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
					w := open(t, i, "t1", "w", "/github")
					i.Rules = i.Rules[:len(i.Rules)-1]
					w.Groups["u1"] = []string{"g1"}
					w.AgentOwner["x"] = owner
					tr.add(i)
					lr.add(i)
					worlds++
					where := fmt.Sprintf("%s, %s, owner %q, on behalf of %q", tr.name, lr.name, owner, behalf)
					var people []string
					for _, p := range []string{owner, behalf} {
						if p != "" {
							people = append(people, p)
						}
					}
					visible, ceiling, _ := i.Visible("t1", "w", people, "/github")
					tenantVisible, tenantCeiling, _ := i.Visible("t1", "w", nil, "/github")
					if visible && !tenantVisible {
						t.Fatalf("a lower level made visible what the tenant level hides: %s", where)
					}
					if visible && rank[ceiling] > rank[tenantCeiling] {
						t.Fatalf("a lower level raised the tenant ceiling: %s", where)
					}
					for _, p := range people {
						inStore := len(i.Store("t1", "w", p, "github")) == 1
						_, getErr := i.GetListing("t1", "w", p, "/github")
						installErr := i.Install("t1", "w", p, "/github", "probe")
						if !inStore && (getErr == nil || installErr == nil) {
							t.Fatalf("absent from the store and reachable by API: %s", where)
						}
						delete(w.Connectors, "probe")
					}
					for _, f := range []string{"search", "create_issue", "delete_repo"} {
						r := Request{Agent: "x", User: behalf, Session: "s1", Connector: conn, Function: f, Now: 10}
						out, d := w.Call(r)
						isListed := listed(w, r)
						if !visible && (isListed || out != UnknownTool) {
							t.Fatalf("listing not visible and function %s listed %v, call %v: %s", f, isListed, out, where)
						}
						if isListed != (out != UnknownTool) {
							t.Fatalf("list and call disagree on %s: %s", f, where)
						}
						if out != UnknownTool && d.Mode.Rank() > mode(ceiling, w.Connectors[conn].Functions[f].Class).Rank() {
							t.Fatalf("%s got %v above the ceiling: %s", f, d.Mode, where)
						}
					}
				}
			}
		}
	}
	t.Logf("catalog worlds %d", worlds)
}

func TestTenantAllowListOfFunctions(t *testing.T) {
	i := fixture()
	i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
	w := open(t, i, "t1", "w", "/github")
	tenant := func(sel string, m Mode) {
		t.Helper()
		if err := w.PutGrant(&Grant{Scope: Tenant, ScopeID: "t1", Listing: "/github", Selector: sel, Mode: m}); err != nil {
			t.Fatalf("tenant row %s %v: %v", sel, m, err)
		}
	}
	tenant("fn:search", Inherit)
	wantCall(t, "03-engine §6.4 G10: an inherit row with no broader tenant row changes nothing", w, "x", "s1", "create_issue", Forwarded)
	tenant("*", Deny)
	if d := wantCall(t, "03-engine §7.2 row 65: * deny with fn:search inherit, the listed function", w, "x", "s1", "search", Forwarded); d.Layers[0] != "skip" {
		t.Fatalf("tenant layer %s, want skip: the function is left to the workspace and the agent, with no prompt added", d.Layers[0])
	}
	if d := wantCall(t, "03-engine §7.2 row 66: a function the list does not name", w, "x", "s1", "create_issue", UnknownTool); d.Step != 6 || d.Reason != "denied_by_row" {
		t.Fatalf("step %d reason %s, want 6 denied_by_row", d.Step, d.Reason)
	}
	wantList(t, "tools/list is the allow list", w, "x", "s1", conn+"__search")
	w.Update(conn, "2.0.0", []Function{{Name: "search", Hash: "a2", Class: Read}, {Name: "search_all", Hash: "d1", Class: Read}, {Name: "issue_create", Hash: "b1", Class: Write}})
	for _, f := range []string{"search", "search_all", "issue_create"} {
		w.Accept(conn, f)
	}
	wantCall(t, "03-engine §7.2 row 67: the listed function at a new, accepted hash (an inherit row has no hash)", w, "x", "s1", "search", Forwarded)
	for _, f := range []string{"search_all", "issue_create"} {
		if d := wantCall(t, "03-engine §7.2 row 67: a new or renamed function a workspace admin accepted is outside the company's list: "+f, w, "x", "s1", f, UnknownTool); d.Step != 6 {
			t.Fatalf("%s refused at step %d, want 6", f, d.Step)
		}
	}
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: conn, Selector: "fn:search", Mode: Ask, Hash: "a2"})
	wantCall(t, "03-engine §6.4 G10: inherit grants nothing, the agent row still answers", w, "x", "s1", "search", Held)
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: "w", Connector: conn, Selector: "*", Mode: Deny})
	if d := wantCall(t, "03-engine §6.4 G10: inherit never widens the workspace", w, "x", "s1", "search", UnknownTool); d.Step != 7 {
		t.Fatalf("step %d, want 7", d.Step)
	}
	if err := i.Install("t1", "lab", "admin", "/github", "lab-gh"); err != nil {
		t.Fatal(err)
	}
	w.PutGrant(&Grant{Scope: Workspace, ScopeID: "lab", Connector: "lab-gh", Selector: "*", Mode: Allow})
	w.PutGrant(&Grant{Scope: Agent, ScopeID: "x", Connector: "lab-gh", Selector: "*", Mode: Allow})
	if err := w.Bind("lab-gh", "x"); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]Outcome{"search": Forwarded, "create_issue": UnknownTool, "delete_repo": UnknownTool} {
		if out, _ := w.Call(Request{Agent: "x", Session: "s1", Connector: "lab-gh", Function: f, Now: 10}); out != want {
			t.Fatalf("03-engine §6.4 G10: the list holds in every workspace, whatever hash it pins: %s in lab: %v, want %v", f, out, want)
		}
	}
	tenant("fn:search", Ask)
	if out, _ := w.Call(Request{Agent: "x", Session: "s1", Connector: "lab-gh", Function: "search", Now: 10}); out != Held {
		t.Fatalf("03-engine §6.4 G1: writing ask over the inherit row replaces it: %v", out)
	}
}

func TestNamedAudienceWithTwoPeopleConsidered(t *testing.T) {
	i := fixture()
	rule := show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull)
	rule.Named = true
	i.Rules = append(i.Rules, rule, show("t1", UserSubject, "u1", ListingTarget, "/github", CeilFull))
	w := open2(t, i)
	w.AgentOwner["x"] = "u1"
	at := func(user string) Outcome {
		out, _ := w.Call(Request{Agent: "x", User: user, Session: "s1", Connector: conn, Function: "search", Now: 10})
		return out
	}
	if out := at(""); out != Forwarded {
		t.Fatalf("03-engine §5.5 V8: the agent of a named owner, acting for nobody: %v", out)
	}
	if out := at("u2"); out != UnknownTool {
		t.Fatalf("03-engine §5.5 V8: the owner of the agent is named and the person acted for is not; each must be named: %v", out)
	}
	if show, _, _ := i.Visible("t1", "w", []string{"u1", "u2"}, "/github"); show {
		t.Fatal("03-engine §5.5 V8: visible although one of the two people considered is not named")
	}
	w.AgentOwner["x"] = "u2"
	if out := at("u1"); out != UnknownTool {
		t.Fatalf("03-engine §5.5 V8: the person acted for is named and the owner of the agent is not: %v", out)
	}
	i.Rules = append(i.Rules, show("t1", UserSubject, "u2", ListingTarget, "/github", CeilFull))
	if out := at("u1"); out != Forwarded {
		t.Fatalf("03-engine §5.5 V8: both people are named: %v", out)
	}
}

func TestListingStatusAcrossVersions(t *testing.T) {
	setup := func() (*Installation, *World) {
		i := fixture()
		i.Rules = append(i.Rules, show("t1", TenantSubject, "t1", ListingTarget, "/github", CeilFull))
		i.Import("/github", "1.0.0")
		w := open(t, i, "t1", "w", "/github")
		if v := w.Connectors[conn].Version; v != "1.0.0" {
			t.Fatalf("installed version %q", v)
		}
		return i, w
	}
	t.Run("03-engine §5.5 V1: a version imported after the listing was quarantined or withdrawn does not bring it back", func(t *testing.T) {
		for status, reason := range map[ListingStatus]string{ListingQuarantined: "listing_quarantined", ListingWithdrawn: "not_in_catalog"} {
			i, w := setup()
			i.Listings["/github"].Status = status
			i.Import("/github", "2.0.0")
			same(t, "the store after the import", i.Store("t1", "w", "admin", "github"))
			if err := i.Install("t1", "w", "admin", "/github", "again"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("install of the new version: %v, want not found", err)
			}
			if d := wantCall(t, "the installed connector", w, "x", "s1", "search", UnknownTool); d.Reason != reason || d.Step != 2 {
				t.Fatalf("reason %s at step %d, want %s at step 2", d.Reason, d.Step, reason)
			}
			w.Update(conn, "2.0.0", nil)
			if d := wantCall(t, "a connector moved to the new version", w, "x", "s1", "search", UnknownTool); d.Reason != reason {
				t.Fatalf("reason %s, want %s", d.Reason, reason)
			}
			i.Listings["/github"].Status = ListingActive
			wantCall(t, "restoreListing on the listing restores every version that has no status of its own", w, "x", "s1", "search", Forwarded)
		}
	})
	t.Run("03-engine §5.5 V1: one version quarantined, the listing shows the newest version that exists", func(t *testing.T) {
		i, w := setup()
		i.Import("/github", "2.0.0")
		i.Listings["/github"].Versions["2.0.0"] = ListingQuarantined
		same(t, "the store still shows the listing", i.Store("t1", "w", "admin", "github"), "/github")
		wantCall(t, "a connector pinned to the older version works", w, "x", "s1", "search", Forwarded)
		if err := i.Install("t1", "lab", "admin", "/github", "lab-gh"); err != nil {
			t.Fatal(err)
		}
		if v := w.Connectors["lab-gh"].Version; v != "1.0.0" {
			t.Fatalf("a new install takes the newest version that exists for the tenant: %q, want 1.0.0", v)
		}
		w.Update(conn, "2.0.0", nil)
		if d := wantCall(t, "a connector pinned to the quarantined version", w, "x", "s1", "search", UnknownTool); d.Reason != "listing_quarantined" {
			t.Fatalf("reason %s", d.Reason)
		}
		i.Listings["/github"].Versions["1.0.0"] = ListingWithdrawn
		same(t, "no version left: the listing is absent", i.Store("t1", "w", "admin", "github"))
	})
	t.Run("03-engine §5.5 V1: the newest version deprecated", func(t *testing.T) {
		i, w := setup()
		i.Listings["/github"].Versions["1.0.0"] = ListingDeprecated
		wantCall(t, "an installed connector keeps working", w, "x", "s1", "search", Forwarded)
		if err := i.Install("t1", "lab", "admin", "/github", "lab-gh"); !errors.Is(err, ErrDeprecated) {
			t.Fatalf("install: %v, want listing_deprecated", err)
		}
	})
}
