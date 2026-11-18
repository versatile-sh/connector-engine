package e2e

import (
	"slices"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
)

func (k *kit) visible(t *testing.T) []string {
	t.Helper()
	var list items[port.ListingVersion]
	k.admin(t).call(t, &list, "listListings", map[string]any{"workspace": workspace})
	var out []string
	for _, lv := range list.Items {
		out = append(out, lv.Ref.Listing.String())
	}
	slices.Sort(out)
	return out
}

func tenancyHidden(t *testing.T, w *world) {
	hidden, open := w.newKit(t, "thidden"), w.newKit(t, "tiso1x")
	for _, k := range []*kit{hidden, open} {
		k.catalogRule(t, "/acme.dev/issues", port.CeilingFull)
	}
	if got := hidden.visible(t); !slices.Contains(got, "/acme.dev/issues") {
		t.Fatalf("before the offer is withheld: %v", got)
	}
	w.operator().call(t, nil, "putOffer", map[string]any{"target_kind": port.TargetListing, "target": "/acme.dev/issues", "withheld": true}, "thidden")

	if got := hidden.visible(t); slices.Contains(got, "/acme.dev/issues") {
		t.Fatalf("a withheld listing is still visible: %v", got)
	}
	if got := open.visible(t); !slices.Contains(got, "/acme.dev/issues") {
		t.Fatalf("withholding for one tenant hid it from another: %v", got)
	}
	hidden.admin(t).refusedWith(t, "404 not_found", "installConnector", map[string]any{"workspace": workspace, "alias": "s", "listing": map[string]any{"name": "acme.dev/issues"}})
	var offered items[port.Listing]
	hidden.admin(t).call(t, &offered, "listOfferedListings", nil)
	for _, l := range offered.Items {
		if l.Ref.Name == "acme.dev/issues" {
			t.Fatalf("a withheld listing is in the offered view: %+v", l)
		}
	}
	w.operator().call(t, nil, "putOffer", map[string]any{"target_kind": port.TargetListing, "target": "/acme.dev/issues", "withheld": false}, "thidden")
	if got := hidden.visible(t); !slices.Contains(got, "/acme.dev/issues") {
		t.Fatalf("the listing does not come back with the offer: %v", got)
	}
}

func tenancyIsolated(t *testing.T, w *world) {
	one, two := w.newKit(t, "tiso1"), w.newKit(t, "tiso2")
	for _, k := range []*kit{one, two} {
		k.issuesConnector(t, port.CeilingFull)
		k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
		k.bind(t, "x")
	}
	one.grant(t, port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	two.grant(t, port.ScopeAgent, "x", "fn:create_issue", port.ModeAllow)

	if one.conn == two.conn {
		t.Fatal("two tenants share a connector id")
	}
	if got := one.agent(t, "x", "s1").names(t); !slices.Equal(got, []string{"s__search"}) {
		t.Fatalf("tenant one lists %v", got)
	}
	if got := two.agent(t, "x", "s1").names(t); !slices.Equal(got, []string{"s__create_issue"}) {
		t.Fatalf("tenant two lists %v", got)
	}
	one.admin(t).refusedWith(t, "404 not_found", "getConnector", nil, string(two.conn))
	var views items[map[string]any]
	one.admin(t).call(t, &views, "listConnectors", map[string]any{"workspace": workspace})
	if len(views.Items) != 1 {
		t.Fatalf("tenant one sees %d connectors", len(views.Items))
	}
	one.admin(t).refusedWith(t, "404 not_found", "createGrant", port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: two.conn, Selector: "*", Mode: port.ModeAllow, ExpiresAt: time.Now().Add(time.Hour)})
	one.admin(t).refusedWith(t, "404 not_found", "listRoles", nil, "--tenant", "tiso2")
	var rows items[port.Grant]
	one.admin(t).call(t, &rows, "listGrants", map[string]any{"connector": string(two.conn)})
	if len(rows.Items) != 0 {
		t.Fatalf("tenant one reads %d grant rows of tenant two", len(rows.Items))
	}
}

func tenancyPrivate(t *testing.T, w *world) {
	owner, other := w.newKit(t, "tpriv"), w.newKit(t, "tiso3")
	manifest := remoteListing("acme.dev/private", "1.0.0", w.mcp.url())
	var created port.ListingVersion
	owner.admin(t).call(t, &created, "createListing", map[string]any{"manifest": manifest})
	if created.Ref.Listing.Owner != "tpriv" || created.Manifest.Source == nil || created.Manifest.Source.Registry != port.PrivateSource {
		t.Fatalf("a private listing is owned by the tenant: %+v", created.Ref)
	}
	if got := owner.visible(t); !slices.Contains(got, "tpriv/acme.dev/private") {
		t.Fatalf("the owner does not see its own listing: %v", got)
	}
	if got := other.visible(t); slices.Contains(got, "tpriv/acme.dev/private") {
		t.Fatalf("another tenant sees a private listing: %v", got)
	}
	other.catalogRule(t, "tpriv/acme.dev/private", port.CeilingFull)
	if got := other.visible(t); slices.Contains(got, "tpriv/acme.dev/private") {
		t.Fatalf("a rule of another tenant exposes a private listing: %v", got)
	}
	other.admin(t).refusedWith(t, "404 not_found", "installConnector", map[string]any{"workspace": workspace, "alias": "p", "listing": map[string]any{"owner": "tpriv", "name": "acme.dev/private"}})
	owner.admin(t).call(t, nil, "installConnector", map[string]any{"workspace": workspace, "alias": "p", "listing": map[string]any{"owner": "tpriv", "name": "acme.dev/private"}})
}
