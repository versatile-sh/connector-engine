package memory

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/port"
)

const snapshotStore = "memory"

type storedRow[V any] struct {
	K      string        `json:"k,omitempty"`
	Tenant port.TenantID `json:"tenant"`
	Rev    int64         `json:"rev,omitempty"`
	V      V             `json:"v"`
}

type storedEvent struct {
	Delivered bool       `json:"delivered,omitempty"`
	E         port.Event `json:"e"`
}

type snapshotTable interface {
	dump() ([]json.RawMessage, error)
	load(rows []json.RawMessage) error
}

func (tb *table[V]) dump() ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(tb.rows))
	for k, r := range tb.rows {
		raw, err := json.Marshal(storedRow[V]{K: k, Tenant: r.tenant, Rev: r.rev, V: r.v})
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func (tb *table[V]) load(rows []json.RawMessage) error {
	tb.rows = make(map[string]row[V], len(rows))
	for _, raw := range rows {
		var r storedRow[V]
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		tb.rows[r.K] = row[V]{tenant: r.Tenant, rev: r.Rev, v: r.V}
	}
	return nil
}

type slice[T any] struct{ p *[]T }

func (s slice[T]) dump() ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(*s.p))
	for _, v := range *s.p {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func (s slice[T]) load(rows []json.RawMessage) error {
	*s.p = make([]T, 0, len(rows))
	for _, raw := range rows {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		*s.p = append(*s.p, v)
	}
	return nil
}

type whole[T any] struct{ p *T }

func (w whole[T]) dump() ([]json.RawMessage, error) {
	raw, err := json.Marshal(*w.p)
	return []json.RawMessage{raw}, err
}

func (w whole[T]) load(rows []json.RawMessage) error {
	var v T
	if len(rows) == 1 {
		if err := json.Unmarshal(rows[0], &v); err != nil {
			return err
		}
	}
	*w.p = v
	return nil
}

type snapshotData struct {
	audit  []storedRow[port.AuditRow]
	events []storedEvent
}

func (d *data) tables(x *snapshotData) map[string]snapshotTable {
	return map[string]snapshotTable{
		"tenants": &d.tenants, "listings": &d.listings, "versions": &d.versions, "offers": &d.offers, "policies": &d.policies, "rules": &d.rules,
		"connectors": &d.connectors, "functions": &d.functions, "bindings": &d.bindings, "grants": &d.grants, "approvals": &d.approvals,
		"pauses": &d.pauses, "tombstones": &d.tombstones, "credentials": &d.credentials, "overlays": &d.overlays, "calls": &d.calls,
		"clients": &d.clients, "source_syncs": &d.syncs, "exposures": &d.exposures, "operations": &d.operations, "fanouts": &d.fanouts,
		"roles": &d.roles, "members": &d.members, "audit": slice[storedRow[port.AuditRow]]{&x.audit}, "events": slice[storedEvent]{&x.events},
		"streams": whole[map[string]int64]{&d.streams}, "inbox": whole[map[string]time.Time]{&d.inbox}, "inbox_data": whole[map[string][]byte]{&d.inboxData},
	}
}

// Snapshot copies every row under the read lock.
func (s *Store) Snapshot(ctx context.Context) (port.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return port.Snapshot{}, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	s.outside()
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := s.d
	var x snapshotData
	for _, r := range d.audit {
		x.audit = append(x.audit, storedRow[port.AuditRow]{Tenant: r.tenant, V: r.v})
	}
	for _, e := range d.events {
		x.events = append(x.events, storedEvent{Delivered: e.delivered, E: e.e})
	}
	snap := port.Snapshot{Store: snapshotStore, Tables: map[string][]json.RawMessage{}}
	for name, tb := range d.tables(&x) {
		rows, err := tb.dump()
		if err != nil {
			return port.Snapshot{}, port.Errorf(port.CodeInternal, "snapshot %s: %v", name, err)
		}
		snap.Tables[name] = rows
	}
	return snap, nil
}

// Restore replaces every row with the snapshot's, or changes nothing when any row does not decode.
func (s *Store) Restore(ctx context.Context, snap port.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return port.Errorf(port.CodeUnavailable, "%v", err)
	}
	if snap.Store != snapshotStore {
		return port.Errorf(port.CodeInvalidArgument, "a %q snapshot does not restore into a %s store", snap.Store, snapshotStore)
	}
	s.outside()
	s.mu.Lock()
	defer s.mu.Unlock()
	work := New().d
	var x snapshotData
	for name, tb := range work.tables(&x) {
		if err := tb.load(snap.Tables[name]); err != nil {
			return port.Errorf(port.CodeInvalidArgument, "snapshot %s: %v", name, err)
		}
	}
	for _, r := range x.audit {
		work.audit = append(work.audit, row[port.AuditRow]{tenant: r.Tenant, v: r.V})
	}
	for _, e := range x.events {
		work.events = append(work.events, event{delivered: e.Delivered, e: e.E})
	}
	if work.streams == nil {
		work.streams = map[string]int64{}
	}
	if work.inbox == nil {
		work.inbox = map[string]time.Time{}
	}
	if work.inboxData == nil {
		work.inboxData = map[string][]byte{}
	}
	var tenants []port.TenantID
	for _, r := range work.grants.rows {
		tenants = append(tenants, r.tenant)
	}
	slices.Sort(tenants)
	in := func(tenant port.TenantID) (port.Tx, error) {
		return &tx{d: &work, scope: port.Scope{Tenant: tenant}, write: true}, nil
	}
	if err := grant.RewriteTenants(slices.Compact(tenants), in, time.Now().UTC()); err != nil {
		return err
	}
	s.d = work
	return nil
}

var _ port.Snapshotter = (*Store)(nil)
