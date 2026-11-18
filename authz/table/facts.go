package table

import (
	"time"

	"github.com/agents-control/connector-engine/core/catalog"
	"github.com/agents-control/connector-engine/core/evaluate"
	"github.com/agents-control/connector-engine/core/exposure"
	"github.com/agents-control/connector-engine/port"
)

// Settings are the installation keys the evaluation reads: permission.destructiveMax and exposure.onTrifecta.
type Settings struct {
	DestructiveMax port.Mode
	ForceAsk       bool
}

// View loads the catalog facts of §5.5 for one tenant: listings, version statuses, offers, rules, hidden workspaces and groups.
func View(tx port.Tx, tenant port.TenantID, workspaces []port.WorkspaceID, groups map[port.PersonID][]port.GroupID) (catalog.View, error) {
	v := catalog.View{Tenant: tenant, Listings: map[port.ListingRef]port.Listing{}, Versions: map[port.VersionRef]port.ListingStatus{}, Hidden: map[port.WorkspaceID]bool{}, Groups: groups}
	listings, err := tx.ListListings()
	if err != nil {
		return v, err
	}
	for _, l := range listings {
		v.Listings[l.Ref] = l
		versions, err := tx.ListVersions(l.Ref)
		if err != nil {
			return v, err
		}
		for _, ver := range versions {
			v.Versions[ver.Ref] = ver.Status
		}
	}
	if v.Offers, err = tx.ListOffers(tenant); err != nil {
		return v, err
	}
	if v.Rules, err = tx.ListRules(); err != nil {
		return v, err
	}
	for _, w := range workspaces {
		p, ok, err := tx.GetPolicy(w)
		if err != nil {
			return v, err
		}
		v.Hidden[w] = ok && p.Default == port.PolicyHidden
	}
	return v, nil
}

func live(t port.Tombstone, now time.Time) bool {
	return t.ExpiresAt.IsZero() || now.Before(t.ExpiresAt)
}

// LoadFacts reads everything §7.4 lists for one agent on one connector; an absent connector yields facts that deny at step 1.
func LoadFacts(tx port.Tx, now time.Time, s port.Subject, connector port.ConnectorID, groups map[port.PersonID][]port.GroupID, set Settings) (evaluate.Facts, error) {
	c, err := tx.GetConnector(connector)
	if port.IsCode(err, port.CodeNotFound) {
		return evaluate.Facts{}, nil
	}
	if err != nil {
		return evaluate.Facts{}, err
	}
	f := evaluate.Facts{Connector: &c, DestructiveMax: set.DestructiveMax, ForceAsk: set.ForceAsk, Functions: map[string]port.Function{}}
	fns, err := tx.ListFunctions(connector)
	if err != nil {
		return f, err
	}
	for _, fn := range fns {
		f.Functions[fn.Name] = fn
	}
	b, err := tx.GetBinding(connector, s.Agent)
	switch {
	case port.IsCode(err, port.CodeNotFound):
	case err != nil:
		return f, err
	default:
		f.Bound, f.PersonRemoved = true, b.OwnerRemoved
	}
	if f.Catalog, err = View(tx, c.Tenant, []port.WorkspaceID{c.Workspace}, groups); err != nil {
		return f, err
	}
	if f.SessionEnded, f.PersonRemoved, err = tombstoned(tx, now, s, c, f.PersonRemoved); err != nil {
		return f, err
	}
	p, ok, err := tx.GetPause(s.Session, s.Agent, connector)
	if err != nil {
		return f, err
	}
	f.Paused = ok && p.Paused && live(port.Tombstone{ExpiresAt: p.ExpiresAt}, now)
	if f.Grants, err = tx.ListGrants(port.GrantFilter{}); err != nil {
		return f, err
	}
	e, ok, err := tx.GetExposure(s.Agent)
	if err != nil {
		return f, err
	}
	if ok && e.Exposed {
		f.LegC = map[string]bool{}
		for _, key := range e.LegC {
			if c, fn := exposure.ParseKey(key); c == connector {
				f.LegC[fn] = true
			}
		}
	}
	return f, nil
}

func tombstoned(tx port.Tx, now time.Time, s port.Subject, c port.Connector, removed bool) (ended, person bool, err error) {
	sessions, err := tx.ListTombstones(port.TombstoneSession)
	if err != nil {
		return false, removed, err
	}
	for _, t := range sessions {
		ended = ended || (t.Subject == string(s.Session) && live(t, now))
	}
	people, err := tx.ListTombstones(port.TombstonePerson)
	if err != nil {
		return ended, removed, err
	}
	for _, t := range people {
		for _, p := range evaluate.People(s) {
			removed = removed || (t.Subject == string(p) && (t.Workspace == "" || t.Workspace == c.Workspace) && live(t, now))
		}
	}
	return ended, removed, nil
}
