package service

import (
	"context"
	"slices"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/catalog"
	"github.com/versatile-sh/connector-engine/core/pinning"
	"github.com/versatile-sh/connector-engine/port"
)

// OverlayTarget names what an overlay applies to: one connector, or a listing in every workspace of the tenant.
type OverlayTarget struct {
	Connector port.ConnectorID `json:"connector"`
	Listing   *port.ListingRef `json:"listing,omitempty"`
}

func (s *Service) overlayAuth(ctx context.Context, p port.Principal, t OverlayTarget) error {
	if t.Listing != nil {
		return s.authorize(ctx, p, authz.CatalogWrite, s.tenantResource(p))
	}
	_, err := s.connectorOf(ctx, p, t.Connector, authz.OverlayManage)
	return err
}

func (s *Service) overlayTx(ctx context.Context, p port.Principal, t OverlayTarget, op string, fn func(tx port.Tx) error) error {
	if err := s.overlayAuth(ctx, p, t); err != nil {
		return err
	}
	return s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, op, fn)
}

func (s *Service) upstreamOf(tx port.Tx, t OverlayTarget, function string) (port.Hash, port.RawDefinition, error) {
	if t.Listing != nil {
		return "", port.RawDefinition{}, nil
	}
	f, err := tx.GetFunction(t.Connector, function)
	return f.DefinitionHash, port.RawDefinition{Name: function, Raw: f.Definition}, err
}

func (s *Service) nextVersion(tx port.Tx, t OverlayTarget, function string) (int, []port.Overlay, error) {
	all, err := tx.ListOverlays(t.Connector, t.Listing, function)
	next := 1
	for _, o := range all {
		next = max(next, o.Version+1)
	}
	return next, all, err
}

func (s *Service) overlayEvent(tx port.Tx, tenant port.TenantID, o port.Overlay, by string) error {
	ev := catalog.OverlayChanged{Function: o.Function, OverlayVersion: o.Version, OverlayHash: string(o.OverlayHash), UpstreamHash: string(o.UpstreamHash), Source: string(o.Source), By: by}
	subject := string(o.Connector)
	if o.Listing != nil {
		ev.Listing, subject = o.Listing.String(), o.Listing.String()
	} else {
		ev.Connector = string(o.Connector)
	}
	return s.record(tx, tenant, by, subject+"/"+o.Function, evOverlayChanged, ev)
}

// ListOverlays returns every version of the overlay of a function, oldest first.
func (s *Service) ListOverlays(ctx context.Context, p port.Principal, t OverlayTarget, function string) ([]port.Overlay, error) {
	if err := s.overlayAuth(ctx, p, t); err != nil {
		return nil, err
	}
	var out []port.Overlay
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		out, err = tx.ListOverlays(t.Connector, t.Listing, function)
		return err
	})
	return out, err
}

// SuggestOverlay asks the DescriptionSuggester for a text and stores it as an untrusted suggestion.
func (s *Service) SuggestOverlay(ctx context.Context, p port.Principal, id port.ConnectorID, function string) (port.Overlay, error) {
	var out port.Overlay
	t := OverlayTarget{Connector: id}
	if s.d.Suggester == nil {
		return out, port.Errorf(port.CodeUnavailable, "no description suggester is configured: a build registers one on the suggester port and adapters.yaml selects it")
	}
	if err := s.overlayAuth(ctx, p, t); err != nil {
		return out, err
	}
	var raw port.RawDefinition
	var hash port.Hash
	if err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		hash, raw, err = s.upstreamOf(tx, t, function)
		return err
	}); err != nil {
		return out, err
	}
	sug, err := s.d.Suggester.Suggest(ctx, raw)
	if err != nil {
		return out, err
	}
	err = s.write(ctx, port.Scope{Tenant: p.Claims.Tenant}, "SuggestOverlay", func(tx port.Tx) error {
		v, _, err := s.nextVersion(tx, t, function)
		if err != nil {
			return err
		}
		out = port.Overlay{Tenant: p.Claims.Tenant, Connector: id, Function: function, Version: v, Title: sug.Title, Description: sug.Description, UpstreamHash: hash,
			OverlayHash: pinning.OverlayHash(sug.Title, sug.Description), State: port.OverlaySuggested, By: "model", Source: port.OverlayByModel, At: s.now()}
		if err := tx.PutOverlay(out); err != nil {
			return err
		}
		return s.emit(tx, out.Tenant, string(id), evOverlaySuggested, catalog.OverlaySuggested{Connector: string(id), Function: function, OverlayVersion: v})
	})
	return out, err
}

// ProposeOverlay writes a person's text as a proposed version against the upstream hash the admin saw; a model's suggestion is turned into a proposal by sending its text, edited or not.
func (s *Service) ProposeOverlay(ctx context.Context, p port.Principal, t OverlayTarget, function, title, description string, upstream port.Hash) (port.Overlay, error) {
	var out port.Overlay
	err := s.overlayTx(ctx, p, t, "ProposeOverlay", func(tx port.Tx) error {
		cur, _, err := s.upstreamOf(tx, t, function)
		if err != nil {
			return err
		}
		if t.Listing == nil && upstream != cur {
			return port.Errorf(port.CodeDefinitionChanged, "function %s is at another hash than the one seen", function)
		}
		v, _, err := s.nextVersion(tx, t, function)
		if err != nil {
			return err
		}
		out = port.Overlay{Tenant: p.Claims.Tenant, Connector: t.Connector, Listing: t.Listing, Function: function, Version: v, Title: title, Description: description,
			UpstreamHash: upstream, OverlayHash: pinning.OverlayHash(title, description), State: port.OverlayProposed, By: actor(p), Source: port.OverlayByPerson, At: s.now()}
		return tx.PutOverlay(out)
	})
	return out, err
}

// ActivateOverlay is acceptOverlay and restoreOverlay: a version becomes the served text, pinned to the upstream hash the admin saw, which for a connector is its current one.
func (s *Service) ActivateOverlay(ctx context.Context, p port.Principal, t OverlayTarget, function string, version int, upstreamSeen port.Hash) (port.Overlay, error) {
	var out port.Overlay
	err := s.overlayTx(ctx, p, t, "ActivateOverlay", func(tx port.Tx) error {
		cur, _, err := s.upstreamOf(tx, t, function)
		if err != nil {
			return err
		}
		_, all, err := s.nextVersion(tx, t, function)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(all, func(o port.Overlay) bool { return o.Version == version })
		switch {
		case i < 0:
			return port.Errorf(port.CodeNotFound, "overlay version %d not found", version)
		case t.Listing == nil && upstreamSeen != cur, t.Listing != nil && upstreamSeen != all[i].UpstreamHash:
			return port.Errorf(port.CodeDefinitionChanged, "the upstream definition is at another hash than the one seen for overlay %d", version)
		}
		for _, o := range all {
			if o.State == port.OverlayActive && o.Version != version {
				o.State = port.OverlaySuperseded
				if err := tx.PutOverlay(o); err != nil {
					return err
				}
			}
		}
		out = all[i]
		out.State, out.OverlayHash, out.UpstreamHash = port.OverlayActive, pinning.OverlayHash(out.Title, out.Description), upstreamSeen
		if err := tx.PutOverlay(out); err != nil {
			return err
		}
		return s.overlayEvent(tx, p.Claims.Tenant, out, actor(p))
	})
	return out, err
}

// ClearOverlay returns a function to the upstream text.
func (s *Service) ClearOverlay(ctx context.Context, p port.Principal, t OverlayTarget, function string) error {
	return s.overlayTx(ctx, p, t, "ClearOverlay", func(tx port.Tx) error {
		_, all, err := s.nextVersion(tx, t, function)
		if err != nil {
			return err
		}
		for _, o := range all {
			if o.State != port.OverlayActive {
				continue
			}
			o.State = port.OverlaySuperseded
			if err := tx.PutOverlay(o); err != nil {
				return err
			}
			o.Version = 0
			return s.overlayEvent(tx, p.Claims.Tenant, o, actor(p))
		}
		return nil
	})
}

func (s *Service) servedOverlay(tx port.Tx, c port.Connector, f port.Function) (*port.Overlay, error) {
	own, err := tx.ListOverlays(c.ID, nil, f.Name)
	if err != nil {
		return nil, err
	}
	shared, err := tx.ListOverlays("", &c.Listing.Listing, f.Name)
	if err != nil {
		return nil, err
	}
	for _, o := range append(own, shared...) {
		if o.State == port.OverlayActive && o.UpstreamHash == f.DefinitionHash {
			return &o, nil
		}
	}
	return nil, nil
}

func (s *Service) carryOverlay(tx port.Tx, f port.Function, keep bool, by string) error {
	all, err := tx.ListOverlays(f.Connector, nil, f.Name)
	if err != nil {
		return err
	}
	for _, o := range all {
		if o.State != port.OverlayActive || o.UpstreamHash == f.DefinitionHash {
			continue
		}
		if o.UpstreamHash, o.State = f.DefinitionHash, port.OverlaySuperseded; keep {
			o.State = port.OverlayActive
		}
		if err := tx.PutOverlay(o); err != nil {
			return err
		}
		if !keep {
			o.Version = 0
		}
		if err := s.overlayEvent(tx, f.Tenant, o, by); err != nil {
			return err
		}
	}
	return nil
}
