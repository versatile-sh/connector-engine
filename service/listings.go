package service

import (
	"context"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/authz/table"
	"github.com/versatile-sh/connector-engine/catalog"
	corecatalog "github.com/versatile-sh/connector-engine/core/catalog"
	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/port"
)

// CaptureFunc captures the function definitions of a manifest; Service.Capture is the runtime-backed one.
type CaptureFunc func(ctx context.Context, m port.Manifest) ([]port.RawDefinition, port.ListMeta, error)

// LintResult is what lintListing found: manifest findings, scanner findings and, with a capture, the definitions.
type LintResult struct {
	Findings    []port.Finding       `json:"findings,omitempty"`
	Definitions []port.RawDefinition `json:"definitions,omitempty"`
}

// Capture starts the manifest on the runtime without a credential, lists its functions and stops it.
func (s *Service) Capture(ctx context.Context, m port.Manifest) ([]port.RawDefinition, port.ListMeta, error) {
	c := port.Connector{ID: port.ConnectorID("lint-" + s.d.NewID()), Tenant: m.Owner, Listing: port.VersionRef{Listing: port.ListingRef{Owner: m.Owner, Name: m.Name}, Version: m.Version}}
	h, err := s.d.Runtime.Start(ctx, c, m, port.SecretsRef{})
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	defer s.d.Runtime.Stop(ctx, h)
	return s.d.Runtime.ListFunctions(ctx, h)
}

// LintListing validates a manifest; anyone may call it. Capturing and scanning the definitions starts the listing, so it needs the role that creates or reviews listings.
func (s *Service) LintListing(ctx context.Context, p port.Principal, m port.Manifest, capture CaptureFunc) (LintResult, error) {
	res := LintResult{Findings: listing.Lint(m, s.cfg.RequireDigest)}
	if capture == nil {
		return res, nil
	}
	if err := s.authorize(ctx, p, authz.ListingReview, s.scopeResource(p)); err != nil {
		return res, err
	}
	if len(res.Findings) > 0 {
		return res, nil
	}
	defs, _, err := capture(ctx, m)
	if err != nil {
		return res, err
	}
	res.Definitions = defs
	if s.d.Scanner == nil {
		return res, nil
	}
	for _, d := range defs {
		f, err := s.d.Scanner.Scan(ctx, d)
		if err != nil {
			return res, err
		}
		res.Findings = append(res.Findings, f...)
	}
	return res, nil
}

// SyncSource imports every listing of a source as unreviewed; an existing version is left as it is.
func (s *Service) SyncSource(ctx context.Context, p port.Principal, source string) (port.Operation, error) {
	if err := s.authorize(ctx, p, authz.SourceSync, authz.Installation); err != nil {
		return port.Operation{}, err
	}
	src, ok := s.d.Sources[source]
	if !ok {
		return port.Operation{}, port.Errorf(port.CodeNotFound, "source %s is not configured", source)
	}
	var fetched []port.Manifest
	for cursor := ""; ; {
		page, err := src.List(ctx, cursor)
		if err != nil {
			return port.Operation{}, err
		}
		fetched = append(fetched, page.Manifests...)
		if cursor = page.Next; cursor == "" {
			break
		}
	}
	op := port.Operation{ID: port.OperationID(s.d.NewID()), Kind: "sync_source", Done: true, Counts: map[string]int{"imported": 0, "skipped": 0}, CreatedAt: s.now(), UpdatedAt: s.now()}
	err := s.write(ctx, port.Scope{Installation: true}, "SyncSource", func(tx port.Tx) error {
		for _, m := range fetched {
			imported, err := s.importManifest(tx, source, m)
			if err != nil {
				return err
			}
			if imported {
				op.Counts["imported"]++
			} else {
				op.Counts["skipped"]++
			}
		}
		if err := tx.PutSourceSync(port.SourceSync{Source: source, LastSync: s.now(), Imported: op.Counts["imported"], Skipped: op.Counts["skipped"]}); err != nil {
			return err
		}
		return tx.PutOperation(op)
	})
	return op, err
}

func (s *Service) importManifest(tx port.Tx, source string, m port.Manifest) (bool, error) {
	m.Owner, m.Review, m.Signature = port.Installation, nil, nil
	if m.Source == nil {
		m.Source = &port.SourceInfo{Registry: source}
	}
	m.Source.ImportedAt = s.now()
	if listing.Validate(m, s.cfg.RequireDigest) != nil {
		return false, nil
	}
	ref := port.ListingRef{Name: m.Name}
	l, err := tx.GetListing(ref)
	switch {
	case port.IsCode(err, port.CodeNotFound):
		l = port.Listing{Ref: ref, Source: m.Source.Registry, Publisher: m.Publisher.Name, Status: port.ListingActive}
	case err != nil:
		return false, err
	}
	vref := port.VersionRef{Listing: ref, Version: m.Version}
	if _, err := tx.GetVersion(vref); err == nil {
		return false, nil
	}
	m.TrustLevel = listing.Trust(m, listing.Evidence{})
	l.Versions = append(l.Versions, m.Version)
	if err := tx.PutListing(l); err != nil {
		return false, err
	}
	status := port.ListingActive
	if m.Source.Status == "deprecated" {
		status = port.ListingDeprecated
	}
	if err := tx.PutVersion(port.ListingVersion{Ref: vref, Manifest: m, Status: status, ImportedAt: s.now()}); err != nil {
		return false, err
	}
	return true, s.record(tx, port.Installation, "system", ref.String()+"@"+m.Version, evListingImported,
		catalog.ListingImported{Name: m.Name, Version: m.Version, Source: l.Source, TrustLevel: string(m.TrustLevel)})
}

// CreateListingInput is a manifest to create; FirstParty marks an installation listing the operator built.
type CreateListingInput struct {
	Manifest   port.Manifest `json:"manifest"`
	FirstParty bool          `json:"first_party"`
}

// CreateListing creates a listing version: an installation listing for the operator, a private one for a tenant admin.
func (s *Service) CreateListing(ctx context.Context, p port.Principal, in CreateListingInput) (port.ListingVersion, error) {
	tenant := p.Claims.Tenant
	private := tenant != port.Installation
	action, resource, scope := authz.ListingCreateInst, authz.Installation, port.Scope{Installation: true}
	if private {
		if !s.cfg.PrivateListings {
			return port.ListingVersion{}, port.Errorf(port.CodePermissionDenied, "private listings are off")
		}
		action, resource, scope = authz.ListingCreatePriv, authz.Tenant(string(tenant)), port.Scope{Tenant: tenant}
	}
	if err := s.authorize(ctx, p, action, resource); err != nil {
		return port.ListingVersion{}, err
	}
	m := in.Manifest
	if private {
		m = listing.Private(m, tenant)
	} else {
		m.Owner = port.Installation
		if m.Source == nil {
			m.Source = &port.SourceInfo{Registry: "operator"}
		}
	}
	if err := listing.Validate(m, s.cfg.RequireDigest); err != nil {
		return port.ListingVersion{}, err
	}
	m.TrustLevel = listing.Trust(m, listing.Evidence{FirstParty: in.FirstParty && !private})
	ref := port.ListingRef{Owner: m.Owner, Name: m.Name}
	out := port.ListingVersion{Ref: port.VersionRef{Listing: ref, Version: m.Version}, Manifest: m, Status: port.ListingActive, ImportedAt: s.now()}
	err := s.write(ctx, scope, "CreateListing", func(tx port.Tx) error {
		l, err := tx.GetListing(ref)
		fresh := port.IsCode(err, port.CodeNotFound)
		switch {
		case fresh:
			l = port.Listing{Ref: ref, Source: m.Source.Registry, Publisher: m.Publisher.Name, Status: port.ListingActive}
		case err != nil:
			return err
		case slices.Contains(l.Versions, m.Version):
			return port.Errorf(port.CodeConflict, "%s@%s exists", ref, m.Version)
		}
		l.Versions = append(l.Versions, m.Version)
		if err := tx.PutListing(l); err != nil {
			return err
		}
		if err := tx.PutVersion(out); err != nil {
			return err
		}
		if fresh && private {
			if err := s.putRule(tx, actor(p), corecatalog.PrivateRule(ref, s.cfg.DefaultCeiling)); err != nil {
				return err
			}
		}
		return s.record(tx, tenant, actor(p), ref.String()+"@"+m.Version, evListingCreated,
			catalog.ListingCreated{Owner: string(m.Owner), Name: m.Name, Version: m.Version, TrustLevel: string(m.TrustLevel), By: actor(p)})
	})
	return out, err
}

type viewer struct {
	tenant port.TenantID
	people []port.PersonID
	groups map[port.PersonID][]port.GroupID
}

func (s *Service) viewerOf(ctx context.Context, p port.Principal) (viewer, error) {
	var people []port.PersonID
	if p.Person != "" {
		people = append(people, p.Person)
	}
	g, err := s.groups(ctx, p.Claims.Tenant, people...)
	return viewer{tenant: p.Claims.Tenant, people: people, groups: g}, err
}

func (v viewer) view(tx port.Tx, workspace port.WorkspaceID) (corecatalog.View, error) {
	return table.View(tx, v.tenant, []port.WorkspaceID{workspace}, v.groups)
}

// ListingFilter narrows ListListings; zero fields match all.
type ListingFilter struct {
	Source string          `json:"source"`
	Trust  port.TrustLevel `json:"trust"`
	Kind   port.Kind       `json:"kind"`
	Query  string          `json:"query"`
}

// ListListings returns the newest version of every listing visible to the caller in the workspace.
func (s *Service) ListListings(ctx context.Context, p port.Principal, workspace port.WorkspaceID, f ListingFilter) ([]port.ListingVersion, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, authz.Tenant(string(p.Claims.Tenant))); err != nil {
		return nil, err
	}
	who, err := s.viewerOf(ctx, p)
	if err != nil {
		return nil, err
	}
	var out []port.ListingVersion
	err = s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		v, err := who.view(tx, workspace)
		if err != nil {
			return err
		}
		for _, ref := range v.Visible(workspace, who.people) {
			ver, ok := v.Newest(ref)
			if !ok || ver == "" {
				continue
			}
			lv, err := tx.GetVersion(port.VersionRef{Listing: ref, Version: ver})
			if err != nil {
				return err
			}
			if matches(lv, v.Listings[ref], f) {
				out = append(out, lv)
			}
		}
		return nil
	})
	return out, err
}

func matches(lv port.ListingVersion, l port.Listing, f ListingFilter) bool {
	m := lv.Manifest
	text := strings.ToLower(m.Name + " " + m.Title + " " + m.Description)
	return (f.Source == "" || l.Source == f.Source) && (f.Trust == "" || m.TrustLevel == f.Trust) && (f.Kind == "" || m.Kind == f.Kind) &&
		(f.Query == "" || strings.Contains(text, strings.ToLower(f.Query)))
}

// GetListing returns one visible listing version; a listing the caller may not see is not found.
func (s *Service) GetListing(ctx context.Context, p port.Principal, workspace port.WorkspaceID, ref port.VersionRef) (port.ListingVersion, error) {
	var out port.ListingVersion
	if err := s.authorize(ctx, p, authz.ConnectorRead, authz.Tenant(string(p.Claims.Tenant))); err != nil {
		return out, err
	}
	who, err := s.viewerOf(ctx, p)
	if err != nil {
		return out, err
	}
	err = s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		v, err := who.view(tx, workspace)
		if err != nil {
			return err
		}
		if !v.Resolve(workspace, who.people, ref.Listing).Visible || !v.Status(ref.Listing, ref.Version).Exists() {
			return port.Errorf(port.CodeNotFound, "%s not found", ref.Listing)
		}
		out, err = tx.GetVersion(ref)
		return err
	})
	return out, err
}

// ListOfferedListings is the curation view of a tenant admin: every listing offered to the tenant, visible or not.
func (s *Service) ListOfferedListings(ctx context.Context, p port.Principal) ([]port.Listing, error) {
	if err := s.authorize(ctx, p, authz.CatalogRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []port.Listing
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		v, err := table.View(tx, p.Claims.Tenant, nil, nil)
		if err != nil {
			return err
		}
		for ref := range v.Listings {
			if l, ok := v.Offered(ref); ok {
				out = append(out, l)
			}
		}
		slices.SortFunc(out, func(a, b port.Listing) int { return strings.Compare(a.Ref.String(), b.Ref.String()) })
		return nil
	})
	return out, err
}

// ReviewFunction is a reviewer's class for one function at the hash the reviewer saw.
type ReviewFunction struct {
	Name  string     `json:"name"`
	Hash  port.Hash  `json:"hash"`
	Class port.Class `json:"class"`
}

// ReviewInput is a review: the reviewed classes and a note.
type ReviewInput struct {
	Functions []ReviewFunction `json:"functions,omitempty"`
	Note      string           `json:"note"`
}

// ReviewListing makes the class map of a version trusted at the reviewed hashes and recomputes its trust level.
func (s *Service) ReviewListing(ctx context.Context, p port.Principal, ref port.VersionRef, in ReviewInput) (port.ListingVersion, error) {
	var out port.ListingVersion
	scope, resource := port.Scope{Installation: true}, authz.Installation
	if ref.Listing.Owner != port.Installation {
		scope, resource = port.Scope{Tenant: ref.Listing.Owner}, authz.Tenant(string(ref.Listing.Owner))
		if p.Claims.Tenant != ref.Listing.Owner {
			return out, port.Errorf(port.CodeNotFound, "%s not found", ref.Listing)
		}
	}
	if err := s.authorize(ctx, p, authz.ListingReview, resource); err != nil {
		return out, err
	}
	err := s.write(ctx, scope, "ReviewListing", func(tx port.Tx) error {
		v, err := tx.GetVersion(ref)
		if err != nil {
			return err
		}
		m := v.Manifest
		review := port.Review{By: actor(p), Scope: ref.Listing.Owner, At: s.now(), Note: in.Note}
		if m.Review != nil {
			review.DefinitionHashes = slices.Clone(m.Review.DefinitionHashes)
		}
		counts := map[string]int{}
		for _, f := range in.Functions {
			i := slices.IndexFunc(m.Functions, func(d port.FunctionDef) bool { return d.Name == f.Name })
			if i < 0 {
				m.Functions = append(m.Functions, port.FunctionDef{Name: f.Name})
				i = len(m.Functions) - 1
			}
			m.Functions[i].DefinitionHash, m.Functions[i].Class, m.Functions[i].ClassSource = f.Hash, f.Class, port.ClassSourceReviewed
			if !slices.Contains(review.DefinitionHashes, f.Hash) {
				review.DefinitionHashes = append(review.DefinitionHashes, f.Hash)
			}
			counts[string(f.Class)]++
		}
		m.Review = &review
		m.TrustLevel = listing.Trust(m, listing.Evidence{FirstParty: v.Manifest.TrustLevel == port.TrustFirstParty})
		v.Manifest = m
		if err := tx.PutVersion(v); err != nil {
			return err
		}
		out = v
		return s.record(tx, ref.Listing.Owner, actor(p), ref.Listing.String()+"@"+ref.Version, evListingReviewed,
			catalog.ListingReviewed{Name: ref.Listing.Name, Version: ref.Version, By: actor(p), CountsByClass: counts})
	})
	return out, err
}

// SetListingStarter marks an installation listing for tenants created afterwards.
func (s *Service) SetListingStarter(ctx context.Context, p port.Principal, ref port.ListingRef, starter bool) (port.Listing, error) {
	var out port.Listing
	if err := s.authorize(ctx, p, authz.ListingStarter, authz.Installation); err != nil {
		return out, err
	}
	if ref.Owner != port.Installation {
		return out, port.Errorf(port.CodeInvalidArgument, "only an installation listing is a starter")
	}
	err := s.write(ctx, port.Scope{Installation: true}, "SetListingStarter", func(tx port.Tx) error {
		l, err := tx.GetListing(ref)
		if err != nil {
			return err
		}
		l.Starter = starter
		if err := tx.PutListing(l); err != nil {
			return err
		}
		out = l
		return s.record(tx, port.Installation, actor(p), ref.String(), evListingStatus,
			catalog.ListingStatusChanged{Owner: string(ref.Owner), Name: ref.Name, Status: string(l.Status), Starter: starter, By: actor(p)})
	})
	return out, err
}

// StatusChange is the result of a listing status change: the listing, and either the operation of the operator's fan-out
// or, for a private listing, the connectors it blocks.
type StatusChange struct {
	Listing   port.Listing       `json:"listing"`
	Operation *port.Operation    `json:"operation,omitempty"`
	Blocked   []port.ConnectorID `json:"blocked,omitempty"`
}

// ChangeListingStatus is quarantineListing, deprecateListing, withdrawListing and restoreListing: the status maps to
// quarantined, deprecated, deleted and active; version names one version, empty the listing.
func (s *Service) ChangeListingStatus(ctx context.Context, p port.Principal, ref port.ListingRef, version string, status port.ListingStatus, reason string) (StatusChange, error) {
	var out StatusChange
	scope, action, resource := port.Scope{Installation: true}, authz.ListingStatusInst, authz.Installation
	if ref.Owner != port.Installation {
		if p.Claims.Tenant != ref.Owner {
			return out, port.Errorf(port.CodeNotFound, "%s not found", ref)
		}
		scope, action, resource = port.Scope{Tenant: ref.Owner}, authz.ListingStatusPriv, authz.Tenant(string(ref.Owner))
	}
	if err := s.authorize(ctx, p, action, resource); err != nil {
		return out, err
	}
	err := s.write(ctx, scope, "ChangeListingStatus", func(tx port.Tx) error {
		l, err := tx.GetListing(ref)
		if err != nil {
			return err
		}
		if version == "" {
			l.Status = status
			err = tx.PutListing(l)
		} else {
			var v port.ListingVersion
			if v, err = tx.GetVersion(port.VersionRef{Listing: ref, Version: version}); err == nil {
				v.Status = status
				err = tx.PutVersion(v)
			}
		}
		if err != nil {
			return err
		}
		out.Listing = l
		if err := s.record(tx, ref.Owner, actor(p), ref.String(), evListingStatus, catalog.ListingStatusChanged{
			Owner: string(ref.Owner), Name: ref.Name, Version: version, Status: string(status), Starter: l.Starter, By: actor(p), Reason: reason}); err != nil {
			return err
		}
		if ref.Owner == port.Installation {
			op, err := s.queueFanOut(tx, "listing_status", &ref)
			out.Operation = &op
			return err
		}
		blocked, err := s.syncBlocked(tx, ref.Owner, &ref)
		out.Blocked = blocked
		return err
	})
	return out, err
}
