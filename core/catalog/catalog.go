// Package catalog is the visibility predicate: which listings a tenant, a workspace and a person see, and at which ceiling.
package catalog

import (
	"cmp"
	"slices"

	"github.com/agents-control/connector-engine/port"
)

// View is everything the predicate reads for one tenant.
type View struct {
	Tenant   port.TenantID
	Listings map[port.ListingRef]port.Listing
	Versions map[port.VersionRef]port.ListingStatus
	Offers   []port.Offer
	Rules    []port.CatalogRule
	Hidden   map[port.WorkspaceID]bool
	Groups   map[port.PersonID][]port.GroupID
}

// CeilingMode is the mode a ceiling gives a class, applied at step 11.
func CeilingMode(c port.Ceiling, class port.Class) port.Mode {
	switch {
	case c == port.CeilingFull:
		return port.ModeAllow
	case class.Effective() == port.ClassRead:
		return port.ModeAllow
	case c == port.CeilingRead:
		return port.ModeDeny
	case class.Effective() == port.ClassWrite:
		return port.ModeAskOnce
	}
	return port.ModeAsk
}

// Offered returns the listing when it exists for the tenant before statuses: its own, or an installation listing not withheld.
func (v View) Offered(ref port.ListingRef) (port.Listing, bool) {
	l, ok := v.Listings[ref]
	if !ok || (ref.Owner != port.Installation && ref.Owner != v.Tenant) {
		return port.Listing{}, false
	}
	if ref.Owner == port.Installation {
		for _, o := range v.Offers {
			if o.Tenant == v.Tenant && o.Withheld && ((o.TargetKind == port.TargetSource && o.Target == l.Source) || (o.TargetKind == port.TargetListing && o.Target == ref.String())) {
				return port.Listing{}, false
			}
		}
	}
	return l, true
}

// Status is the effective status of one version for the tenant; a listing not offered is deleted.
func (v View) Status(ref port.ListingRef, version string) port.ListingStatus {
	l, ok := v.Offered(ref)
	if !ok {
		return port.ListingDeleted
	}
	own := v.Versions[port.VersionRef{Listing: ref, Version: version}]
	if own == "" {
		own = port.ListingActive
	}
	return port.EffectiveStatus(l.Status, own)
}

// Newest is the newest version that exists for the tenant; a listing with no imported version stands for one implicit version "".
func (v View) Newest(ref port.ListingRef) (string, bool) {
	l, ok := v.Offered(ref)
	if !ok {
		return "", false
	}
	if len(l.Versions) == 0 {
		return "", v.Status(ref, "").Exists()
	}
	for _, ver := range slices.Backward(l.Versions) {
		if v.Status(ref, ver).Exists() {
			return ver, true
		}
	}
	return "", false
}

// Level is the answer of one level, with the rules that gave it.
type Level struct {
	Answered bool          `json:"answered"`
	Show     bool          `json:"show"`
	Install  bool          `json:"install"`
	Ceiling  port.Ceiling  `json:"ceiling"`
	Named    bool          `json:"named"`
	Rules    []port.RuleID `json:"rules,omitempty"`
}

// Shows reports whether the level answered and shows.
func (a Level) Shows() bool { return a.Answered && a.Show }

func covers(r port.CatalogRule, ref port.ListingRef, l port.Listing) bool {
	if ref.Owner != port.Installation {
		return r.TargetKind == port.TargetListing && r.Target == ref.String()
	}
	switch r.TargetKind {
	case port.TargetSource:
		return r.Target == l.Source
	case port.TargetPublisher:
		return r.Target == l.Publisher
	}
	return r.Target == ref.String()
}

var targetRank = map[port.TargetKind]int{port.TargetSource: 0, port.TargetPublisher: 1, port.TargetListing: 2}

func (v View) level(subjects map[port.SubjectKind][]string, ref port.ListingRef, l port.Listing) Level {
	var best []port.CatalogRule
	top := -1
	for _, r := range v.Rules {
		if r.Tenant != v.Tenant || !covers(r, ref, l) || !slices.Contains(subjects[r.SubjectKind], r.SubjectID) {
			continue
		}
		rank := targetRank[r.TargetKind] * 2
		if r.SubjectKind == port.SubjectUser {
			rank++
		}
		if rank > top {
			top, best = rank, best[:0]
		}
		if rank == top {
			best = append(best, r)
		}
	}
	if len(best) == 0 {
		return Level{}
	}
	a := Level{Answered: true, Show: true, Install: true, Ceiling: port.CeilingFull}
	for _, r := range best {
		a.Show = a.Show && r.Show
		a.Install = a.Install && r.Install
		a.Named = a.Named || r.Audience == port.AudienceNamed
		a.Ceiling = a.Ceiling.Lower(r.Ceiling)
		a.Rules = append(a.Rules, r.ID)
	}
	return a
}

// Answer is the resolution of one listing for a workspace and the people considered.
type Answer struct {
	Visible     bool         `json:"visible"`
	Installable bool         `json:"installable"`
	Ceiling     port.Ceiling `json:"ceiling"`
	Version     string       `json:"version"`
	Tenant      Level        `json:"tenant"`
	Workspace   Level        `json:"workspace"`
	People      []Level      `json:"people,omitempty"`
}

// Resolve applies V1 to V8 to one listing for a workspace and the people considered (owner of the agent, person acted for, or the caller).
func (v View) Resolve(workspace port.WorkspaceID, people []port.PersonID, ref port.ListingRef) Answer {
	l, ok := v.Offered(ref)
	version, exists := v.Newest(ref)
	if !ok || !exists {
		return Answer{}
	}
	out := Answer{Version: version, Tenant: v.level(map[port.SubjectKind][]string{port.SubjectTenant: {string(v.Tenant)}}, ref, l)}
	if !out.Tenant.Answered {
		return out
	}
	out.Workspace = v.level(map[port.SubjectKind][]string{port.SubjectWorkspace: {string(workspace)}}, ref, l)
	if !out.Workspace.Answered && v.Hidden[workspace] {
		return out
	}
	named := len(people) > 0
	for _, p := range people {
		groups := make([]string, 0, len(v.Groups[p]))
		for _, g := range v.Groups[p] {
			groups = append(groups, string(g))
		}
		a := v.level(map[port.SubjectKind][]string{port.SubjectUser: {string(p)}, port.SubjectGroup: groups}, ref, l)
		named = named && a.Shows()
		out.People = append(out.People, a)
	}
	if (out.Workspace.Named && !named) || (out.Tenant.Named && !named && !out.Workspace.Shows()) {
		return out
	}
	show, install, ceiling := true, true, port.CeilingFull
	for _, a := range append([]Level{out.Tenant, out.Workspace}, out.People...) {
		if a.Answered {
			show, install, ceiling = show && a.Show, install && a.Install, ceiling.Lower(a.Ceiling)
		}
	}
	out.Visible, out.Ceiling = show, ceiling
	out.Installable = show && install && v.Status(ref, version) == port.ListingActive
	return out
}

// Visible lists the listings visible to a workspace and the people considered, sorted by ref.
func (v View) Visible(workspace port.WorkspaceID, people []port.PersonID) []port.ListingRef {
	var out []port.ListingRef
	for ref := range v.Listings {
		if v.Resolve(workspace, people, ref).Visible {
			out = append(out, ref)
		}
	}
	slices.SortFunc(out, func(a, b port.ListingRef) int { return cmp.Compare(a.String(), b.String()) })
	return out
}

// ValidatePolicy refuses a policy whose subject is not a workspace: there is no tenant policy.
func ValidatePolicy(kind port.SubjectKind) error {
	if kind != port.SubjectWorkspace {
		return port.Errorf(port.CodePolicyRefused, "a catalog policy names a workspace, never the tenant")
	}
	return nil
}

// ValidateRule refuses a rule with an unknown ceiling or a named audience below the workspace level.
func ValidateRule(r port.CatalogRule) error {
	switch {
	case !r.Ceiling.Valid():
		return port.Errorf(port.CodeInvalidArgument, "unknown ceiling %q", r.Ceiling)
	case r.Audience == port.AudienceNamed && r.SubjectKind != port.SubjectTenant && r.SubjectKind != port.SubjectWorkspace:
		return port.Errorf(port.CodeInvalidArgument, "audience named is on tenant and workspace rules only")
	case r.Tenant == "" || r.SubjectID == "" || r.Target == "":
		return port.Errorf(port.CodeInvalidArgument, "a rule names its tenant, subject and target")
	}
	return nil
}

// StarterRules are the rules createTenant writes under catalog.tenantDefault = starter: show, install, ceiling read.
func StarterRules(tenant port.TenantID, listings []port.Listing) []port.CatalogRule {
	var out []port.CatalogRule
	for _, l := range listings {
		if l.Starter && l.Ref.Owner == port.Installation {
			out = append(out, port.CatalogRule{
				Tenant: tenant, SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing,
				Target: l.Ref.String(), Show: true, Install: true, Ceiling: port.CeilingRead, Audience: port.AudienceAll, CreatedBy: port.StarterCreator,
			})
		}
	}
	slices.SortFunc(out, func(a, b port.CatalogRule) int { return cmp.Compare(a.Target, b.Target) })
	return out
}

// PrivateRule is the tenant rule createListing writes for a private listing, at catalog.defaultCeiling.
func PrivateRule(ref port.ListingRef, ceiling port.Ceiling) port.CatalogRule {
	return port.CatalogRule{
		Tenant: ref.Owner, SubjectKind: port.SubjectTenant, SubjectID: string(ref.Owner), TargetKind: port.TargetListing,
		Target: ref.String(), Show: true, Install: true, Ceiling: ceiling, Audience: port.AudienceAll,
	}
}
