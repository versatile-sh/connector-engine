package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	byKey     = "k"
	byCreated = "cn_time(json_extract(doc, '$.created_at')), k"
	byStarted = "cn_time(json_extract(doc, '$.started_at')), k"
)

func (t *tx) GetTenant(id port.TenantID) (port.Tenant, error) {
	return get[port.Tenant](t, kindTenant, string(id))
}

func (t *tx) ListTenants() ([]port.Tenant, error) {
	return list[port.Tenant](t, kindTenant, nil, byKey, nil)
}

func (t *tx) PutTenant(v port.Tenant) error { return t.set(kindTenant, string(v.ID), v.ID, v) }

func (t *tx) GetListing(ref port.ListingRef) (port.Listing, error) {
	return get[port.Listing](t, kindListing, ref.String())
}

func (t *tx) ListListings() ([]port.Listing, error) {
	return list[port.Listing](t, kindListing, nil, byKey, nil)
}

func (t *tx) PutListing(l port.Listing) error {
	return t.set(kindListing, l.Ref.String(), l.Ref.Owner, l)
}

func versionKey(r port.VersionRef) string { return key(r.Listing.String(), r.Version) }

func (t *tx) GetVersion(ref port.VersionRef) (port.ListingVersion, error) {
	return get[port.ListingVersion](t, kindVersion, versionKey(ref))
}

func (t *tx) ListVersions(ref port.ListingRef) ([]port.ListingVersion, error) {
	return list(t, kindVersion, nil, byKey, func(v port.ListingVersion) bool { return v.Ref.Listing == ref })
}

func (t *tx) PutVersion(v port.ListingVersion) error {
	return t.set(kindVersion, versionKey(v.Ref), v.Ref.Listing.Owner, v)
}

func (t *tx) ListOffers(tenant port.TenantID) ([]port.Offer, error) {
	return list[port.Offer](t, kindOffer, want("tenant", string(tenant)), byKey, nil)
}

func (t *tx) PutOffer(o port.Offer) error {
	return t.set(kindOffer, key(string(o.Tenant), string(o.TargetKind), o.Target), o.Tenant, o)
}

func (t *tx) GetPolicy(ws port.WorkspaceID) (port.CatalogPolicy, bool, error) {
	return maybe[port.CatalogPolicy](t, kindPolicy, key(string(t.scope.Tenant), string(ws)))
}

func (t *tx) PutPolicy(p port.CatalogPolicy) error {
	return t.set(kindPolicy, key(string(p.Tenant), string(p.Workspace)), p.Tenant, p)
}

func (t *tx) ListRules() ([]port.CatalogRule, error) {
	return list[port.CatalogRule](t, kindRule, nil, byKey, nil)
}

func (t *tx) PutRule(r port.CatalogRule) error { return t.set(kindRule, string(r.ID), r.Tenant, r) }

func (t *tx) DeleteRule(id port.RuleID) error { return t.del(kindRule, string(id)) }

func (t *tx) GetConnector(id port.ConnectorID) (port.Connector, error) {
	return get[port.Connector](t, kindConnector, string(id))
}

func (t *tx) ListConnectors(f port.ConnectorFilter) ([]port.Connector, error) {
	return list(t, kindConnector, want("workspace", string(f.Workspace)), byKey, f.Match)
}

func (t *tx) PutConnector(c port.Connector, expect int64) (port.Connector, error) {
	rev, err := t.put(kindConnector, string(c.ID), c.Tenant, nil, c, expect)
	c.Revision = rev
	return c, err
}

func (t *tx) GetFunction(connector port.ConnectorID, name string) (port.Function, error) {
	return get[port.Function](t, kindFunction, key(string(connector), name))
}

func (t *tx) ListFunctions(connector port.ConnectorID) ([]port.Function, error) {
	return list[port.Function](t, kindFunction, want("connector", string(connector)), byKey, nil)
}

func (t *tx) PutFunction(f port.Function) error {
	return t.set(kindFunction, key(string(f.Connector), f.Name), f.Tenant, f)
}

func (t *tx) GetBinding(connector port.ConnectorID, agent port.AgentID) (port.Binding, error) {
	return get[port.Binding](t, kindBinding, key(string(connector), string(agent)))
}

func (t *tx) ListBindings(f port.BindingFilter) ([]port.Binding, error) {
	return list(t, kindBinding, want("connector", string(f.Connector), "agent", string(f.Agent)), byKey, f.Match)
}

func (t *tx) PutBinding(b port.Binding) error {
	return t.set(kindBinding, key(string(b.Connector), string(b.Agent)), b.Tenant, b)
}

func (t *tx) DeleteBinding(connector port.ConnectorID, agent port.AgentID) error {
	return t.del(kindBinding, key(string(connector), string(agent)))
}

func (t *tx) GetGrant(id port.GrantID) (port.Grant, error) {
	return get[port.Grant](t, kindGrant, string(id))
}

func (t *tx) ListGrants(f port.GrantFilter) ([]port.Grant, error) {
	return list(t, kindGrant, want("scope_kind", string(f.ScopeKind), "scope_id", f.ScopeID, "agent", string(f.Agent), "connector", string(f.Connector)), byKey, f.Match)
}

func (t *tx) PutGrant(g port.Grant) error {
	g.Uses = 0
	sum := sha256.Sum256([]byte(g.Key()))
	uniq := hex.EncodeToString(sum[:])
	_, err := t.put(kindGrant, string(g.ID), g.Tenant, &uniq, g, port.AnyRevision)
	return err
}

func (t *tx) DeleteGrant(id port.GrantID) error { return t.del(kindGrant, string(id)) }

type usage struct {
	ID   string
	Max  int
	Uses int
}

func (t *tx) ReserveGrants(ids []port.GrantID) error {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = string(id)
	}
	rows, err := scan(t, `SELECT k, coalesce(json_extract(doc, '$.max_uses'), 0), coalesce(json_extract(doc, '$.uses'), 0)
		FROM records WHERE kind = 'grant' AND k IN (SELECT value FROM json_each(?3)) AND `+visible+` ORDER BY k`,
		func(r *sql.Rows) (u usage, err error) { return u, r.Scan(&u.ID, &u.Max, &u.Uses) }, jsonList(keys))
	if err != nil {
		return err
	}
	byID := map[string]usage{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	var counted []string
	for _, id := range keys {
		g, ok := byID[id]
		switch {
		case !ok:
			return port.Errorf(port.CodeNotFound, "grant %s not found", id)
		case g.Max == 0:
		case g.Uses >= g.Max:
			return port.Errorf(port.CodeGrantExhausted, "grant %s used %d of %d", id, g.Uses, g.Max)
		default:
			counted = append(counted, id)
		}
	}
	_, err = t.exec(`UPDATE records SET doc = json_set(doc, '$.uses', coalesce(json_extract(doc, '$.uses'), 0) + 1), rev = rev + 1
		WHERE kind = 'grant' AND k IN (SELECT value FROM json_each(?3)) AND `+writable, jsonList(counted))
	return err
}

func (t *tx) GetApproval(id port.ApprovalID) (port.Approval, error) {
	return get[port.Approval](t, kindApproval, string(id))
}

func (t *tx) ListApprovals(f port.ApprovalFilter) ([]port.Approval, error) {
	return list(t, kindApproval, want("session", string(f.Session), "agent", string(f.Agent), "connector", string(f.Connector), "call_hash", string(f.CallHash)), byCreated, f.Match)
}

func openTwin(a port.Approval) *string {
	if !a.State.Open() {
		return nil
	}
	k := key(string(a.Tenant), string(a.Session), string(a.CallHash))
	return &k
}

func (t *tx) CreateApproval(a port.Approval) error {
	return t.insert(kindApproval, string(a.ID), a.Tenant, openTwin(a), a)
}

func (t *tx) UpdateApproval(a port.Approval, from port.ApprovalState) error {
	return t.update(kindApproval, string(a.ID), a.Tenant, openTwin(a), a, string(from))
}

func pauseKey(tenant port.TenantID, s port.SessionID, a port.AgentID, c port.ConnectorID) string {
	return key(string(tenant), string(s), string(a), string(c))
}

func (t *tx) GetPause(s port.SessionID, a port.AgentID, c port.ConnectorID) (port.Pause, bool, error) {
	return maybe[port.Pause](t, kindPause, pauseKey(t.scope.Tenant, s, a, c))
}

func (t *tx) ListPauses() ([]port.Pause, error) {
	return list[port.Pause](t, kindPause, nil, byKey, nil)
}

func (t *tx) PutPause(p port.Pause) error {
	return t.set(kindPause, pauseKey(p.Tenant, p.Session, p.Agent, p.Connector), p.Tenant, p)
}

func (t *tx) DeletePause(id port.PauseID) error {
	keys, err := many[string](t, `DELETE FROM records WHERE kind = 'pause' AND json_extract(doc, '$.id') = ?3 AND `+writable+` RETURNING k`, string(id))
	if err == nil && len(keys) == 0 {
		return port.Errorf(port.CodeNotFound, "pause %s not found", id)
	}
	for _, k := range keys {
		if err == nil {
			err = t.untrack(kindPause, k)
		}
	}
	return err
}

func tombstoneKey(tenant port.TenantID, kind port.TombstoneKind, subject string, ws port.WorkspaceID) string {
	return key(string(tenant), string(kind), subject, string(ws))
}

func (t *tx) ListTombstones(kind port.TombstoneKind) ([]port.Tombstone, error) {
	return list[port.Tombstone](t, kindTombstone, want("kind", string(kind)), byKey, nil)
}

func (t *tx) PutTombstone(s port.Tombstone) error {
	return t.set(kindTombstone, tombstoneKey(s.Tenant, s.Kind, s.Subject, s.Workspace), s.Tenant, s)
}

func (t *tx) DeleteTombstone(kind port.TombstoneKind, subject string, ws port.WorkspaceID) error {
	return t.del(kindTombstone, tombstoneKey(t.scope.Tenant, kind, subject, ws))
}

func (t *tx) GetCredential(id port.CredentialID) (port.Credential, error) {
	return get[port.Credential](t, kindCredential, string(id))
}

func (t *tx) ListCredentials(f port.CredentialFilter) ([]port.Credential, error) {
	return list(t, kindCredential, want("owner_kind", string(f.OwnerKind), "owner_id", f.OwnerID, "connector", string(f.Connector)), byKey, f.Match)
}

func (t *tx) PutCredential(c port.Credential, expect int64) (port.Credential, error) {
	rev, err := t.put(kindCredential, string(c.ID), c.Tenant, nil, c, expect)
	c.Revision = rev
	return c, err
}

func (t *tx) DeleteCredential(id port.CredentialID) error { return t.del(kindCredential, string(id)) }

func overlayKey(o port.Overlay) string {
	target := string(o.Connector)
	if o.Listing != nil {
		target = "listing:" + o.Listing.String()
	}
	return key(string(o.Tenant), target, o.Function, strconv.Itoa(o.Version))
}

func (t *tx) ListOverlays(connector port.ConnectorID, listing *port.ListingRef, function string) ([]port.Overlay, error) {
	return list(t, kindOverlay, want("function", function), "CAST(json_extract(doc, '$.overlay_version') AS INTEGER), k",
		func(o port.Overlay) bool { return o.Targets(connector, listing, function) })
}

func (t *tx) PutOverlay(o port.Overlay) error { return t.set(kindOverlay, overlayKey(o), o.Tenant, o) }

func (t *tx) GetCall(id port.CallID) (port.Call, error) {
	return get[port.Call](t, kindCall, string(id))
}

func (t *tx) LatestCall(session port.SessionID, callHash port.Hash) (port.Call, bool, error) {
	calls, err := list[port.Call](t, kindCall, want("session", string(session), "call_hash", string(callHash)), "cn_time(json_extract(doc, '$.started_at')) DESC, k LIMIT 1", nil)
	if err != nil || len(calls) == 0 {
		return port.Call{}, false, err
	}
	return calls[0], true, nil
}

func (t *tx) CreateCall(c port.Call) error { return t.insert(kindCall, string(c.ID), c.Tenant, nil, c) }

func (t *tx) UpdateCall(c port.Call, from port.CallState) error {
	return t.update(kindCall, string(c.ID), c.Tenant, nil, c, string(from))
}

func (t *tx) ListCalls(f port.CallFilter) ([]port.Call, error) {
	return list[port.Call](t, kindCall, want("agent", string(f.Agent), "session", string(f.Session), "connector", string(f.Connector), "state", string(f.State)), byStarted, f.Match)
}

func (t *tx) GetClient(issuer string) (port.ClientRegistration, bool, error) {
	return maybe[port.ClientRegistration](t, kindClient, key(string(t.scope.Tenant), issuer))
}

func (t *tx) PutClient(c port.ClientRegistration) error {
	return t.set(kindClient, key(string(c.Tenant), c.Issuer), c.Tenant, c)
}

func (t *tx) ListSourceSyncs() ([]port.SourceSync, error) {
	return list[port.SourceSync](t, kindSourceSync, nil, byKey, nil)
}

func (t *tx) PutSourceSync(s port.SourceSync) error {
	return t.set(kindSourceSync, s.Source, port.Installation, s)
}

func (t *tx) GetExposure(agent port.AgentID) (port.Exposure, bool, error) {
	return maybe[port.Exposure](t, kindExposure, key(string(t.scope.Tenant), string(agent)))
}

func (t *tx) PutExposure(e port.Exposure) error {
	return t.set(kindExposure, key(string(e.Tenant), string(e.Agent)), e.Tenant, e)
}

func (t *tx) GetOperation(id port.OperationID) (port.Operation, error) {
	return get[port.Operation](t, kindOperation, string(id))
}

func (t *tx) PutOperation(op port.Operation) error {
	return t.set(kindOperation, string(op.ID), op.Tenant, op)
}

func (t *tx) ListFanOuts() ([]port.FanOut, error) {
	return list[port.FanOut](t, kindFanOut, nil, byKey, nil)
}

func (t *tx) PutFanOut(f port.FanOut) error {
	return t.set(kindFanOut, string(f.ID), port.Installation, f)
}

func (t *tx) ListRoles(f port.RoleFilter) ([]port.RoleAssignment, error) {
	return list[port.RoleAssignment](t, kindRole, want("person", string(f.Person)), byKey, nil)
}

func (t *tx) PutRole(r port.RoleAssignment) error {
	return t.set(kindRole, key(string(r.Tenant), string(r.Person), r.Role, r.Scope, r.Issuer), r.Tenant, r)
}

func (t *tx) DeleteRole(r port.RoleAssignment) error {
	return t.del(kindRole, key(string(r.Tenant), string(r.Person), r.Role, r.Scope, r.Issuer))
}

func (t *tx) ListMembers(f port.MemberFilter) ([]port.GroupMember, error) {
	return list[port.GroupMember](t, kindMember, want("person", string(f.Person), "group", string(f.Group)), byKey, nil)
}

func (t *tx) PutMember(m port.GroupMember) error {
	return t.set(kindMember, key(string(m.Tenant), string(m.Person), string(m.Group)), m.Tenant, m)
}

func (t *tx) DeleteMember(m port.GroupMember) error {
	return t.del(kindMember, key(string(m.Tenant), string(m.Person), string(m.Group)))
}
