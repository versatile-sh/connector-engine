// Package memory is an in-memory port.Store and port.Outbox: tenant-partitioned, copy-on-write, atomic per Write.
package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/port"
)

type access int

const (
	tenantRows access = iota
	sharedRows
	aboutTenantRows
)

type row[V any] struct {
	tenant port.TenantID
	rev    int64
	v      V
}

type table[V any] struct {
	rows   map[string]row[V]
	access access
}

func newTable[V any](a access) table[V] { return table[V]{rows: map[string]row[V]{}, access: a} }

func (tb table[V]) clone() table[V] {
	tb.rows = maps.Clone(tb.rows)
	return tb
}

func dup[V any](v V) (V, error) {
	var c V
	b, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil {
		return c, port.Errorf(port.CodeInvalidArgument, "record is not serialisable: %v", err)
	}
	return c, nil
}

type event struct {
	delivered bool
	e         port.Event
}

type data struct {
	tenants     table[port.Tenant]
	listings    table[port.Listing]
	versions    table[port.ListingVersion]
	offers      table[port.Offer]
	policies    table[port.CatalogPolicy]
	rules       table[port.CatalogRule]
	connectors  table[port.Connector]
	functions   table[port.Function]
	bindings    table[port.Binding]
	grants      table[port.Grant]
	approvals   table[port.Approval]
	pauses      table[port.Pause]
	tombstones  table[port.Tombstone]
	credentials table[port.Credential]
	overlays    table[port.Overlay]
	calls       table[port.Call]
	clients     table[port.ClientRegistration]
	syncs       table[port.SourceSync]
	exposures   table[port.Exposure]
	operations  table[port.Operation]
	fanouts     table[port.FanOut]
	roles       table[port.RoleAssignment]
	members     table[port.GroupMember]
	audit       []row[port.AuditRow]
	events      []event
	streams     map[string]int64
	inbox       map[string]time.Time
}

func (d data) clone() data {
	d.tenants, d.listings, d.versions, d.offers = d.tenants.clone(), d.listings.clone(), d.versions.clone(), d.offers.clone()
	d.policies, d.rules, d.connectors, d.functions = d.policies.clone(), d.rules.clone(), d.connectors.clone(), d.functions.clone()
	d.bindings, d.grants, d.approvals, d.pauses = d.bindings.clone(), d.grants.clone(), d.approvals.clone(), d.pauses.clone()
	d.tombstones, d.credentials, d.overlays, d.calls = d.tombstones.clone(), d.credentials.clone(), d.overlays.clone(), d.calls.clone()
	d.clients, d.syncs = d.clients.clone(), d.syncs.clone()
	d.exposures, d.operations, d.fanouts = d.exposures.clone(), d.operations.clone(), d.fanouts.clone()
	d.roles, d.members = d.roles.clone(), d.members.clone()
	d.audit, d.events = slices.Clone(d.audit), slices.Clone(d.events)
	d.streams, d.inbox = maps.Clone(d.streams), maps.Clone(d.inbox)
	return d
}

// Store is the in-memory port.Store and port.Outbox.
type Store struct {
	mu sync.RWMutex
	d  data
}

// New returns an empty store.
func New() *Store {
	return &Store{d: data{
		tenants: newTable[port.Tenant](aboutTenantRows), listings: newTable[port.Listing](sharedRows),
		versions: newTable[port.ListingVersion](sharedRows), offers: newTable[port.Offer](aboutTenantRows),
		policies: newTable[port.CatalogPolicy](tenantRows), rules: newTable[port.CatalogRule](tenantRows),
		connectors: newTable[port.Connector](tenantRows), functions: newTable[port.Function](tenantRows),
		bindings: newTable[port.Binding](tenantRows), grants: newTable[port.Grant](tenantRows),
		approvals: newTable[port.Approval](tenantRows), pauses: newTable[port.Pause](tenantRows),
		tombstones: newTable[port.Tombstone](tenantRows), credentials: newTable[port.Credential](tenantRows),
		overlays: newTable[port.Overlay](tenantRows), calls: newTable[port.Call](tenantRows), clients: newTable[port.ClientRegistration](tenantRows), syncs: newTable[port.SourceSync](sharedRows),
		exposures: newTable[port.Exposure](tenantRows), operations: newTable[port.Operation](tenantRows),
		fanouts: newTable[port.FanOut](tenantRows), roles: newTable[port.RoleAssignment](tenantRows),
		members: newTable[port.GroupMember](tenantRows), streams: map[string]int64{}, inbox: map[string]time.Time{},
	}}
}

// Read runs fn on a read-only view; every write in it fails.
func (s *Store) Read(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	if err := check(ctx, scope); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fn(&tx{d: &s.d, scope: scope})
}

// Write runs fn on a private copy and publishes it only when fn returns nil: state, audit and outbox rows commit together.
func (s *Store) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	if err := check(ctx, scope); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	work := s.d.clone()
	if err := fn(&tx{d: &work, scope: scope, write: true}); err != nil {
		return err
	}
	s.d = work
	return nil
}

// Pending returns up to limit undelivered events in emit order; limit <= 0 means all.
func (s *Store) Pending(_ context.Context, limit int) ([]port.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []port.Event
	for _, ev := range s.d.events {
		if limit > 0 && len(out) == limit {
			break
		}
		if !ev.delivered {
			c, err := dup(ev.e)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// MarkDelivered flags the events with the given ids; unknown ids are ignored.
func (s *Store) MarkDelivered(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.events {
		if slices.Contains(ids, s.d.events[i].e.ID) {
			s.d.events[i].delivered = true
		}
	}
	return nil
}

func check(ctx context.Context, scope port.Scope) error {
	if err := ctx.Err(); err != nil {
		return port.Errorf(port.CodeUnavailable, "%v", err)
	}
	if scope.Tenant == "" && !scope.Installation {
		return port.Errorf(port.CodeInvalidArgument, "scope names a tenant or the installation")
	}
	return nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
