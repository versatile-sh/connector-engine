package memory

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
)

type tx struct {
	d     *data
	scope port.Scope
	write bool
}

func (t *tx) reads(a access, owner port.TenantID) bool {
	switch {
	case owner == port.Installation:
		return t.scope.Installation || a == sharedRows
	case a == aboutTenantRows && t.scope.Installation:
		return true
	}
	return owner == t.scope.Tenant
}

func (t *tx) allow(a access, owner port.TenantID) error {
	switch {
	case !t.write:
		return port.Errorf(port.CodeInternal, "write in a read transaction")
	case owner == port.Installation || a == aboutTenantRows:
		if !t.scope.Installation {
			return port.Errorf(port.CodePermissionDenied, "installation rows are written in the installation scope")
		}
	case owner != t.scope.Tenant:
		return port.Errorf(port.CodePermissionDenied, "record belongs to another tenant")
	}
	return nil
}

func key(parts ...string) string { return strings.Join(parts, "\x00") }

func get[V any](t *tx, tb table[V], k string) (V, int64, error) {
	r, ok := tb.rows[k]
	if !ok || !t.reads(tb.access, r.tenant) {
		var zero V
		return zero, 0, port.Errorf(port.CodeNotFound, "%s not found", strings.ReplaceAll(k, "\x00", "/"))
	}
	v, err := tb.copy(r.v)
	return v, r.rev, err
}

func list[V any](t *tx, tb table[V], keep func(V) bool) ([]V, error) {
	var out []V
	for _, k := range slices.Sorted(maps.Keys(tb.rows)) {
		r := tb.rows[k]
		if !t.reads(tb.access, r.tenant) || (keep != nil && !keep(r.v)) {
			continue
		}
		c, err := tb.copy(r.v)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func put[V any](t *tx, tb table[V], k string, owner port.TenantID, v V, expect int64) (int64, error) {
	old, exists := tb.rows[k]
	if exists && !t.reads(tb.access, old.tenant) {
		return 0, port.Errorf(port.CodePermissionDenied, "record belongs to another tenant")
	}
	if err := t.allow(tb.access, owner); err != nil {
		return 0, err
	}
	if exists && old.tenant != owner {
		return 0, port.Errorf(port.CodeConflict, "record changes tenant")
	}
	if expect != port.AnyRevision && expect != old.rev {
		return 0, port.Errorf(port.CodeRevisionMismatch, "record is at revision %d, not %d", old.rev, expect)
	}
	c, err := dup(v)
	if err != nil {
		return 0, err
	}
	tb.rows[k] = row[V]{tenant: owner, rev: old.rev + 1, v: c}
	return old.rev + 1, nil
}

func revised[V any](t *tx, tb table[V], k string, owner port.TenantID, v V, expect int64, stamp func(*V, int64)) (V, error) {
	stamp(&v, tb.rows[k].rev+1)
	_, err := put(t, tb, k, owner, v, expect)
	return v, err
}

func del[V any](t *tx, tb table[V], k string) error {
	r, ok := tb.rows[k]
	if !ok || !t.reads(tb.access, r.tenant) {
		return port.Errorf(port.CodeNotFound, "%s not found", strings.ReplaceAll(k, "\x00", "/"))
	}
	if err := t.allow(tb.access, r.tenant); err != nil {
		return err
	}
	delete(tb.rows, k)
	return nil
}

func (t *tx) GetTenant(id port.TenantID) (port.Tenant, error) {
	v, _, err := get(t, t.d.tenants, string(id))
	return v, err
}

func (t *tx) ListTenants() ([]port.Tenant, error) { return list(t, t.d.tenants, nil) }

func (t *tx) PutTenant(v port.Tenant) error {
	_, err := put(t, t.d.tenants, string(v.ID), v.ID, v, port.AnyRevision)
	return err
}

func (t *tx) GetListing(ref port.ListingRef) (port.Listing, error) {
	v, _, err := get(t, t.d.listings, ref.String())
	return v, err
}

func (t *tx) ListListings() ([]port.Listing, error) { return list(t, t.d.listings, nil) }

func (t *tx) PutListing(l port.Listing) error {
	_, err := put(t, t.d.listings, l.Ref.String(), l.Ref.Owner, l, port.AnyRevision)
	return err
}

func versionKey(r port.VersionRef) string { return key(r.Listing.String(), r.Version) }

func (t *tx) GetVersion(ref port.VersionRef) (port.ListingVersion, error) {
	v, _, err := get(t, t.d.versions, versionKey(ref))
	return v, err
}

func (t *tx) ListVersions(ref port.ListingRef) ([]port.ListingVersion, error) {
	return list(t, t.d.versions, func(v port.ListingVersion) bool { return v.Ref.Listing == ref })
}

func (t *tx) PutVersion(v port.ListingVersion) error {
	_, err := put(t, t.d.versions, versionKey(v.Ref), v.Ref.Listing.Owner, v, port.AnyRevision)
	return err
}

func (t *tx) ListOffers(tenant port.TenantID) ([]port.Offer, error) {
	return list(t, t.d.offers, func(o port.Offer) bool { return o.Tenant == tenant })
}

func (t *tx) PutOffer(o port.Offer) error {
	_, err := put(t, t.d.offers, key(string(o.Tenant), string(o.TargetKind), o.Target), o.Tenant, o, port.AnyRevision)
	return err
}

func (t *tx) GetPolicy(ws port.WorkspaceID) (port.CatalogPolicy, bool, error) {
	v, _, err := get(t, t.d.policies, key(string(t.scope.Tenant), string(ws)))
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func (t *tx) PutPolicy(p port.CatalogPolicy) error {
	_, err := put(t, t.d.policies, key(string(p.Tenant), string(p.Workspace)), p.Tenant, p, port.AnyRevision)
	return err
}

func (t *tx) ListRules() ([]port.CatalogRule, error) { return list(t, t.d.rules, nil) }

func (t *tx) PutRule(r port.CatalogRule) error {
	_, err := put(t, t.d.rules, string(r.ID), r.Tenant, r, port.AnyRevision)
	return err
}

func (t *tx) DeleteRule(id port.RuleID) error { return del(t, t.d.rules, string(id)) }

func (t *tx) GetConnector(id port.ConnectorID) (port.Connector, error) {
	v, _, err := get(t, t.d.connectors, string(id))
	return v, err
}

func (t *tx) ListConnectors(f port.ConnectorFilter) ([]port.Connector, error) {
	return list(t, t.d.connectors, f.Match)
}

func (t *tx) PutConnector(c port.Connector, expect int64) (port.Connector, error) {
	return revised(t, t.d.connectors, string(c.ID), c.Tenant, c, expect, func(c *port.Connector, rev int64) { c.Revision = rev })
}

func (t *tx) GetFunction(connector port.ConnectorID, name string) (port.Function, error) {
	v, _, err := get(t, t.d.functions, key(string(connector), name))
	return v, err
}

func (t *tx) ListFunctions(connector port.ConnectorID) ([]port.Function, error) {
	return list(t, t.d.functions, func(f port.Function) bool { return f.Connector == connector })
}

func (t *tx) PutFunction(f port.Function) error {
	_, err := put(t, t.d.functions, key(string(f.Connector), f.Name), f.Tenant, f, port.AnyRevision)
	return err
}

func (t *tx) GetBinding(connector port.ConnectorID, agent port.AgentID) (port.Binding, error) {
	v, _, err := get(t, t.d.bindings, key(string(connector), string(agent)))
	return v, err
}

func (t *tx) ListBindings(f port.BindingFilter) ([]port.Binding, error) {
	return list(t, t.d.bindings, f.Match)
}

func (t *tx) PutBinding(b port.Binding) error {
	_, err := put(t, t.d.bindings, key(string(b.Connector), string(b.Agent)), b.Tenant, b, port.AnyRevision)
	return err
}

func (t *tx) DeleteBinding(connector port.ConnectorID, agent port.AgentID) error {
	return del(t, t.d.bindings, key(string(connector), string(agent)))
}

func (t *tx) GetGrant(id port.GrantID) (port.Grant, error) {
	v, _, err := get(t, t.d.grants, string(id))
	return v, err
}

func (t *tx) ListGrants(f port.GrantFilter) ([]port.Grant, error) {
	return list(t, t.d.grants, f.Match)
}

func (t *tx) PutGrant(g port.Grant) error {
	for id, r := range t.d.grants.rows {
		if id != string(g.ID) && r.tenant == g.Tenant && r.v.Key() == g.Key() {
			return port.Errorf(port.CodeConflict, "grant %s has the key of %s", g.ID, id)
		}
	}
	_, err := put(t, t.d.grants, string(g.ID), g.Tenant, g, port.AnyRevision)
	return err
}

func (t *tx) DeleteGrant(id port.GrantID) error { return del(t, t.d.grants, string(id)) }

func (t *tx) ReserveGrants(ids []port.GrantID) error {
	var counted []port.Grant
	for _, id := range ids {
		g, _, err := get(t, t.d.grants, string(id))
		if err != nil {
			return err
		}
		if g.MaxUses == 0 {
			continue
		}
		if g.Uses >= g.MaxUses {
			return port.Errorf(port.CodeGrantExhausted, "grant %s used %d of %d", id, g.Uses, g.MaxUses)
		}
		counted = append(counted, g)
	}
	for _, g := range counted {
		g.Uses++
		if _, err := put(t, t.d.grants, string(g.ID), g.Tenant, g, port.AnyRevision); err != nil {
			return err
		}
	}
	return nil
}

func (t *tx) GetApproval(id port.ApprovalID) (port.Approval, error) {
	v, _, err := get(t, t.d.approvals, string(id))
	return v, err
}

func (t *tx) ListApprovals(f port.ApprovalFilter) ([]port.Approval, error) {
	out, err := list(t, t.d.approvals, f.Match)
	slices.SortStableFunc(out, func(a, b port.Approval) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, err
}

func (t *tx) openTwin(a port.Approval) error {
	if !a.State.Open() {
		return nil
	}
	for id, r := range t.d.approvals.rows {
		if id != string(a.ID) && r.tenant == a.Tenant && r.v.State.Open() && r.v.Session == a.Session && r.v.CallHash == a.CallHash {
			return port.Errorf(port.CodeConflict, "approval %s is open for this session and call hash", id)
		}
	}
	return nil
}

func (t *tx) CreateApproval(a port.Approval) error {
	if _, ok := t.d.approvals.rows[string(a.ID)]; ok {
		return port.Errorf(port.CodeConflict, "approval %s exists", a.ID)
	}
	if err := t.openTwin(a); err != nil {
		return err
	}
	_, err := put(t, t.d.approvals, string(a.ID), a.Tenant, a, port.AnyRevision)
	return err
}

func (t *tx) UpdateApproval(a port.Approval, from port.ApprovalState) error {
	cur, _, err := get(t, t.d.approvals, string(a.ID))
	if err != nil {
		return err
	}
	if cur.State != from {
		return port.Errorf(port.CodeConflict, "approval %s is %s, not %s", a.ID, cur.State, from)
	}
	if err := t.openTwin(a); err != nil {
		return err
	}
	_, err = put(t, t.d.approvals, string(a.ID), a.Tenant, a, port.AnyRevision)
	return err
}

func pauseKey(s port.SessionID, a port.AgentID, c port.ConnectorID) string {
	return key(string(s), string(a), string(c))
}

func (t *tx) GetPause(s port.SessionID, a port.AgentID, c port.ConnectorID) (port.Pause, bool, error) {
	v, _, err := get(t, t.d.pauses, key(string(t.scope.Tenant), pauseKey(s, a, c)))
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func (t *tx) ListPauses() ([]port.Pause, error) { return list(t, t.d.pauses, nil) }

func (t *tx) PutPause(p port.Pause) error {
	_, err := put(t, t.d.pauses, key(string(p.Tenant), pauseKey(p.Session, p.Agent, p.Connector)), p.Tenant, p, port.AnyRevision)
	return err
}

func (t *tx) DeletePause(id port.PauseID) error {
	for k, r := range t.d.pauses.rows {
		if r.v.ID == id && t.reads(tenantRows, r.tenant) {
			return del(t, t.d.pauses, k)
		}
	}
	return port.Errorf(port.CodeNotFound, "pause %s not found", id)
}

func tombKey(tenant port.TenantID, kind port.TombstoneKind, subject string, ws port.WorkspaceID) string {
	return key(string(tenant), string(kind), subject, string(ws))
}

func (t *tx) ListTombstones(kind port.TombstoneKind) ([]port.Tombstone, error) {
	return list(t, t.d.tombstones, func(s port.Tombstone) bool { return s.Kind == kind })
}

func (t *tx) PutTombstone(s port.Tombstone) error {
	_, err := put(t, t.d.tombstones, tombKey(s.Tenant, s.Kind, s.Subject, s.Workspace), s.Tenant, s, port.AnyRevision)
	return err
}

func (t *tx) DeleteTombstone(kind port.TombstoneKind, subject string, ws port.WorkspaceID) error {
	return del(t, t.d.tombstones, tombKey(t.scope.Tenant, kind, subject, ws))
}

func (t *tx) GetCredential(id port.CredentialID) (port.Credential, error) {
	v, _, err := get(t, t.d.credentials, string(id))
	return v, err
}

func (t *tx) ListCredentials(f port.CredentialFilter) ([]port.Credential, error) {
	return list(t, t.d.credentials, f.Match)
}

func (t *tx) PutCredential(c port.Credential, expect int64) (port.Credential, error) {
	return revised(t, t.d.credentials, string(c.ID), c.Tenant, c, expect, func(c *port.Credential, rev int64) { c.Revision = rev })
}

func (t *tx) DeleteCredential(id port.CredentialID) error { return del(t, t.d.credentials, string(id)) }

func overlayKey(o port.Overlay) string {
	target := string(o.Connector)
	if o.Listing != nil {
		target = "listing:" + o.Listing.String()
	}
	return key(string(o.Tenant), target, o.Function, strconv.Itoa(o.Version))
}

func (t *tx) ListOverlays(connector port.ConnectorID, listing *port.ListingRef, function string) ([]port.Overlay, error) {
	out, err := list(t, t.d.overlays, func(o port.Overlay) bool { return o.Targets(connector, listing, function) })
	slices.SortFunc(out, func(a, b port.Overlay) int { return cmp.Compare(a.Version, b.Version) })
	return out, err
}

func (t *tx) PutOverlay(o port.Overlay) error {
	_, err := put(t, t.d.overlays, overlayKey(o), o.Tenant, o, port.AnyRevision)
	return err
}

func (t *tx) GetCall(id port.CallID) (port.Call, error) {
	v, _, err := get(t, t.d.calls, string(id))
	return v, err
}

func (t *tx) LatestCall(session port.SessionID, callHash port.Hash) (port.Call, bool, error) {
	calls, err := list(t, t.d.calls, func(c port.Call) bool { return c.Session == session && c.CallHash == callHash })
	if err != nil || len(calls) == 0 {
		return port.Call{}, false, err
	}
	return slices.MaxFunc(calls, func(a, b port.Call) int { return a.StartedAt.Compare(b.StartedAt) }), true, nil
}

func (t *tx) CreateCall(c port.Call) error {
	if _, ok := t.d.calls.rows[string(c.ID)]; ok {
		return port.Errorf(port.CodeConflict, "call %s exists", c.ID)
	}
	_, err := put(t, t.d.calls, string(c.ID), c.Tenant, c, port.AnyRevision)
	return err
}

func (t *tx) UpdateCall(c port.Call, from port.CallState) error {
	cur, _, err := get(t, t.d.calls, string(c.ID))
	if err != nil {
		return err
	}
	if cur.State != from {
		return port.Errorf(port.CodeConflict, "call %s is %s, not %s", c.ID, cur.State, from)
	}
	_, err = put(t, t.d.calls, string(c.ID), c.Tenant, c, port.AnyRevision)
	return err
}

func (t *tx) ListCalls(f port.CallFilter) ([]port.Call, error) {
	out, err := list(t, t.d.calls, f.Match)
	slices.SortStableFunc(out, func(a, b port.Call) int { return a.StartedAt.Compare(b.StartedAt) })
	return out, err
}

func (t *tx) GetClient(issuer string) (port.ClientRegistration, bool, error) {
	v, _, err := get(t, t.d.clients, key(string(t.scope.Tenant), issuer))
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func (t *tx) PutClient(c port.ClientRegistration) error {
	_, err := put(t, t.d.clients, key(string(c.Tenant), c.Issuer), c.Tenant, c, port.AnyRevision)
	return err
}

func (t *tx) ListSourceSyncs() ([]port.SourceSync, error) {
	out, err := list(t, t.d.syncs, nil)
	slices.SortFunc(out, func(a, b port.SourceSync) int { return cmp.Compare(a.Source, b.Source) })
	return out, err
}

func (t *tx) PutSourceSync(s port.SourceSync) error {
	_, err := put(t, t.d.syncs, s.Source, port.Installation, s, port.AnyRevision)
	return err
}

func (t *tx) GetExposure(agent port.AgentID) (port.Exposure, bool, error) {
	v, _, err := get(t, t.d.exposures, key(string(t.scope.Tenant), string(agent)))
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func (t *tx) PutExposure(e port.Exposure) error {
	_, err := put(t, t.d.exposures, key(string(e.Tenant), string(e.Agent)), e.Tenant, e, port.AnyRevision)
	return err
}

func (t *tx) GetOperation(id port.OperationID) (port.Operation, error) {
	v, _, err := get(t, t.d.operations, string(id))
	return v, err
}

func (t *tx) PutOperation(op port.Operation) error {
	_, err := put(t, t.d.operations, string(op.ID), op.Tenant, op, port.AnyRevision)
	return err
}

func (t *tx) ListFanOuts() ([]port.FanOut, error) { return list(t, t.d.fanouts, nil) }

func (t *tx) PutFanOut(f port.FanOut) error {
	_, err := put(t, t.d.fanouts, string(f.ID), port.Installation, f, port.AnyRevision)
	return err
}

// DueWork is the one view across tenants, by the times stored on each row; the worker re-checks each item
// against its own rule (approval.executeGrace is added by core/approval), so an item may be listed early.
func (t *tx) DueWork(now time.Time, limit int) ([]port.WorkItem, error) {
	if !t.scope.Installation {
		return nil, port.Errorf(port.CodePermissionDenied, "due work is read in the installation scope")
	}
	var out []port.WorkItem
	due := func(tenant port.TenantID, kind port.WorkKind, id string, at time.Time) {
		if !at.IsZero() && !at.After(now) {
			out = append(out, port.WorkItem{Tenant: tenant, Kind: kind, ID: id, Due: at})
		}
	}
	for id, r := range t.d.approvals.rows {
		a := r.v
		switch {
		case a.State == port.ApprovalPending:
			due(r.tenant, port.WorkApprovalExpiry, id, a.ExpiresAt)
		case a.State == port.ApprovalApproved:
			due(r.tenant, port.WorkApprovedRecord, id, a.DecidedAt)
		case a.State == port.ApprovalExecuting:
			due(r.tenant, port.WorkExecuteDeadline, id, a.ExecutionStartedAt.Add(a.CallDeadline))
		case a.State.Terminal() && (a.ResultRef != nil || a.Arguments != nil):
			due(r.tenant, port.WorkResultRetention, id, a.ResultExpiresAt)
		}
	}
	for id, r := range t.d.calls.rows {
		if r.v.State == port.CallStarted {
			due(r.tenant, port.WorkCallDeadline, id, r.v.Deadline)
		}
	}
	for id, r := range t.d.grants.rows {
		due(r.tenant, port.WorkGrantExpiry, id, r.v.ExpiresAt)
	}
	for k, r := range t.d.pauses.rows {
		if r.v.Paused {
			due(r.tenant, port.WorkPauseExpiry, k, r.v.ExpiresAt)
		}
	}
	slices.SortFunc(out, func(a, b port.WorkItem) int {
		return cmp.Or(a.Due.Compare(b.Due), cmp.Compare(a.Tenant, b.Tenant), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (t *tx) Audit(r port.AuditRow) error {
	if err := t.allow(tenantRows, r.Tenant); err != nil {
		return err
	}
	c, err := dup(r)
	if err != nil {
		return err
	}
	t.d.audit = append(t.d.audit, row[port.AuditRow]{tenant: r.Tenant, v: c})
	return nil
}

func (t *tx) ListAudit(subject string) ([]port.AuditRow, error) {
	var out []port.AuditRow
	for _, r := range t.d.audit {
		if t.reads(tenantRows, r.tenant) && (subject == "" || r.v.Subject == subject) {
			c, err := dup(r.v)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (t *tx) Emit(e port.Event) (port.Event, error) {
	if err := t.allow(tenantRows, e.Tenant); err != nil {
		return e, err
	}
	if e.Type == "" {
		return e, port.Errorf(port.CodeInvalidArgument, "an event has a type")
	}
	e.ID, e.Source = newID(), port.SourceOf(e.Tenant)
	e.Stream = e.Source
	t.d.streams[e.Stream]++
	e.Sequence = t.d.streams[e.Stream]
	c, err := dup(e)
	if err != nil {
		return e, err
	}
	t.d.events = append(t.d.events, event{e: c})
	return e, nil
}

func (t *tx) EventsAfter(stream string, seq int64, limit int) ([]port.Event, error) {
	var out []port.Event
	for _, ev := range t.d.events {
		if ev.e.Stream != stream || ev.e.Sequence <= seq || !t.reads(tenantRows, ev.e.Tenant) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		c, err := dup(ev.e)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (t *tx) inboxKey(id string) string { return key(string(t.scope.Tenant), id) }

func (t *tx) SeenInbox(id string) (bool, error) {
	_, ok := t.d.inbox[t.inboxKey(id)]
	return ok, nil
}

func (t *tx) MarkInbox(id string, at time.Time) (bool, error) {
	if !t.write {
		return false, port.Errorf(port.CodeInternal, "write in a read transaction")
	}
	k := t.inboxKey(id)
	if _, ok := t.d.inbox[k]; ok {
		return false, nil
	}
	t.d.inbox[k] = at
	return true, nil
}

func roleKey(r port.RoleAssignment) string {
	return key(string(r.Tenant), string(r.Person), r.Role, r.Scope, r.Issuer)
}

func (t *tx) ListRoles(f port.RoleFilter) ([]port.RoleAssignment, error) {
	return list(t, t.d.roles, f.Match)
}

func (t *tx) PutRole(r port.RoleAssignment) error {
	_, err := put(t, t.d.roles, roleKey(r), r.Tenant, r, port.AnyRevision)
	return err
}

func (t *tx) DeleteRole(r port.RoleAssignment) error { return del(t, t.d.roles, roleKey(r)) }

func memberKey(m port.GroupMember) string {
	return key(string(m.Tenant), string(m.Person), string(m.Group))
}

func (t *tx) ListMembers(f port.MemberFilter) ([]port.GroupMember, error) {
	return list(t, t.d.members, f.Match)
}

func (t *tx) PutMember(m port.GroupMember) error {
	_, err := put(t, t.d.members, memberKey(m), m.Tenant, m, port.AnyRevision)
	return err
}

func (t *tx) DeleteMember(m port.GroupMember) error { return del(t, t.d.members, memberKey(m)) }
