package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/jackc/pgx/v5"
)

const (
	kindTenant     = "tenant"
	kindListing    = "listing"
	kindVersion    = "version"
	kindOffer      = "offer"
	kindPolicy     = "policy"
	kindRule       = "rule"
	kindConnector  = "connector"
	kindFunction   = "function"
	kindBinding    = "binding"
	kindGrant      = "grant"
	kindApproval   = "approval"
	kindPause      = "pause"
	kindTombstone  = "tombstone"
	kindCredential = "credential"
	kindOverlay    = "overlay"
	kindCall       = "call"
	kindExposure   = "exposure"
	kindOperation  = "operation"
	kindFanOut     = "fanout"
)

const (
	getSQL     = `SELECT doc || jsonb_build_object('revision', rev) FROM records WHERE kind = $1 AND k = $2`
	selectSQL  = `SELECT doc || jsonb_build_object('revision', rev) FROM records WHERE kind = $1 AND doc @> $2::jsonb ORDER BY `
	revisedSQL = `INSERT INTO records AS r (kind, k, tenant_id, uniq, rev, doc)
		SELECT $1, $2, $3, $4::text, 1, $5::jsonb
		WHERE $6::bigint <= 0 OR EXISTS (SELECT 1 FROM records WHERE kind = $1 AND k = $2 AND rev = $6)
		ON CONFLICT (kind, k) DO UPDATE SET doc = excluded.doc, uniq = excluded.uniq, rev = r.rev + 1
		WHERE ($6::bigint < 0 OR r.rev = $6) AND r.tenant_id = excluded.tenant_id
		RETURNING rev`
)

type tx struct {
	ctx   context.Context
	q     pgx.Tx
	scope port.Scope
}

func one[T any](t *tx, sql string, args ...any) (v T, err error) {
	return v, fail(t.q.QueryRow(t.ctx, sql, args...).Scan(&v))
}

func many[T any](t *tx, sql string, args ...any) ([]T, error) {
	return collect(t, pgx.RowTo[T], sql, args...)
}

func collect[T any](t *tx, row pgx.RowToFunc[T], sql string, args ...any) ([]T, error) {
	rows, err := t.q.Query(t.ctx, sql, args...)
	if err != nil {
		return nil, fail(err)
	}
	out, err := pgx.CollectRows(rows, row)
	return out, fail(err)
}

func (t *tx) exec(sql string, args ...any) (int64, error) {
	tag, err := t.q.Exec(t.ctx, sql, args...)
	return tag.RowsAffected(), fail(err)
}

func key(parts ...string) string { return strings.Join(parts, "\x1f") }

func want(pairs ...string) map[string]any {
	out := map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			out[pairs[i]] = pairs[i+1]
		}
	}
	return out
}

func get[V any](t *tx, kind, k string) (V, error) { return one[V](t, getSQL, kind, k) }

func maybe[V any](t *tx, kind, k string) (V, bool, error) {
	v, err := get[V](t, kind, k)
	if port.IsCode(err, port.CodeNotFound) {
		return v, false, nil
	}
	return v, err == nil, err
}

func list[V any](t *tx, kind string, contains map[string]any, tail string, keep func(V) bool) ([]V, error) {
	if contains == nil {
		contains = map[string]any{}
	}
	all, err := many[V](t, selectSQL+tail, kind, contains)
	if err != nil || keep == nil {
		return all, err
	}
	out := all[:0]
	for _, v := range all {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (t *tx) put(kind, k string, owner port.TenantID, uniq *string, v any, expect int64) (int64, error) {
	rev, err := one[int64](t, revisedSQL, kind, k, string(owner), uniq, v, expect)
	if port.IsCode(err, port.CodeNotFound) {
		if expect == port.AnyRevision {
			return 0, port.Errorf(port.CodeConflict, "%s %s changes tenant", kind, strings.ReplaceAll(k, "\x1f", "/"))
		}
		return 0, port.Errorf(port.CodeRevisionMismatch, "%s %s is not at revision %d", kind, strings.ReplaceAll(k, "\x1f", "/"), expect)
	}
	return rev, err
}

func (t *tx) set(kind, k string, owner port.TenantID, v any) error {
	_, err := t.put(kind, k, owner, nil, v, port.AnyRevision)
	return err
}

func (t *tx) insert(kind, k string, owner port.TenantID, uniq *string, v any) error {
	_, err := t.exec(`INSERT INTO records (kind, k, tenant_id, uniq, rev, doc) VALUES ($1, $2, $3, $4::text, 1, $5::jsonb)`, kind, k, string(owner), uniq, v)
	return err
}

func (t *tx) update(kind, k string, owner port.TenantID, uniq *string, v any, from string) error {
	n, err := t.exec(`UPDATE records SET doc = $5::jsonb, uniq = $4::text, rev = rev + 1
		WHERE kind = $1 AND k = $2 AND tenant_id = $3 AND doc->>'state' = $6`, kind, k, string(owner), uniq, v, from)
	if err != nil || n == 1 {
		return err
	}
	if _, err := one[int](t, `SELECT 1 FROM records WHERE kind = $1 AND k = $2`, kind, k); err != nil {
		return err
	}
	return port.Errorf(port.CodeConflict, "%s %s is not %s", kind, strings.ReplaceAll(k, "\x1f", "/"), from)
}

func (t *tx) del(kind, k string) error {
	n, err := t.exec(`DELETE FROM records WHERE kind = $1 AND k = $2`, kind, k)
	if err == nil && n == 0 {
		return port.Errorf(port.CodeNotFound, "%s %s not found", kind, strings.ReplaceAll(k, "\x1f", "/"))
	}
	return err
}

func (t *tx) GetTenant(id port.TenantID) (port.Tenant, error) {
	return get[port.Tenant](t, kindTenant, string(id))
}

func (t *tx) ListTenants() ([]port.Tenant, error) {
	return list[port.Tenant](t, kindTenant, nil, "k", nil)
}

func (t *tx) PutTenant(v port.Tenant) error { return t.set(kindTenant, string(v.ID), v.ID, v) }

func (t *tx) GetListing(ref port.ListingRef) (port.Listing, error) {
	return get[port.Listing](t, kindListing, ref.String())
}

func (t *tx) ListListings() ([]port.Listing, error) {
	return list[port.Listing](t, kindListing, nil, "k", nil)
}

func (t *tx) PutListing(l port.Listing) error {
	return t.set(kindListing, l.Ref.String(), l.Ref.Owner, l)
}

func versionKey(r port.VersionRef) string { return key(r.Listing.String(), r.Version) }

func (t *tx) GetVersion(ref port.VersionRef) (port.ListingVersion, error) {
	return get[port.ListingVersion](t, kindVersion, versionKey(ref))
}

func (t *tx) ListVersions(ref port.ListingRef) ([]port.ListingVersion, error) {
	return list(t, kindVersion, nil, "k", func(v port.ListingVersion) bool { return v.Ref.Listing == ref })
}

func (t *tx) PutVersion(v port.ListingVersion) error {
	return t.set(kindVersion, versionKey(v.Ref), v.Ref.Listing.Owner, v)
}

func (t *tx) ListOffers(tenant port.TenantID) ([]port.Offer, error) {
	return list[port.Offer](t, kindOffer, want("tenant", string(tenant)), "k", nil)
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
	return list[port.CatalogRule](t, kindRule, nil, "k", nil)
}

func (t *tx) PutRule(r port.CatalogRule) error { return t.set(kindRule, string(r.ID), r.Tenant, r) }

func (t *tx) DeleteRule(id port.RuleID) error { return t.del(kindRule, string(id)) }

func (t *tx) GetConnector(id port.ConnectorID) (port.Connector, error) {
	return get[port.Connector](t, kindConnector, string(id))
}

func (t *tx) ListConnectors(f port.ConnectorFilter) ([]port.Connector, error) {
	return list(t, kindConnector, want("workspace", string(f.Workspace)), "k", f.Match)
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
	return list[port.Function](t, kindFunction, want("connector", string(connector)), "k", nil)
}

func (t *tx) PutFunction(f port.Function) error {
	return t.set(kindFunction, key(string(f.Connector), f.Name), f.Tenant, f)
}

func (t *tx) GetBinding(connector port.ConnectorID, agent port.AgentID) (port.Binding, error) {
	return get[port.Binding](t, kindBinding, key(string(connector), string(agent)))
}

func (t *tx) ListBindings(f port.BindingFilter) ([]port.Binding, error) {
	return list(t, kindBinding, want("connector", string(f.Connector), "agent", string(f.Agent)), "k", f.Match)
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
	return list(t, kindGrant, want("scope_kind", string(f.ScopeKind), "scope_id", f.ScopeID, "agent", string(f.Agent), "connector", string(f.Connector)), "k", f.Match)
}

func (t *tx) PutGrant(g port.Grant) error {
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
	rows, err := collect(t, pgx.RowToStructByPos[usage], `SELECT k, coalesce((doc->>'max_uses')::int, 0), coalesce((doc->>'uses')::int, 0)
		FROM records WHERE kind = 'grant' AND k = ANY($1) ORDER BY k FOR UPDATE`, keys)
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
	_, err = t.exec(`UPDATE records SET doc = jsonb_set(doc, '{uses}', to_jsonb(coalesce((doc->>'uses')::int, 0) + 1)), rev = rev + 1
		WHERE kind = 'grant' AND k = ANY($1)`, counted)
	return err
}

func (t *tx) GetApproval(id port.ApprovalID) (port.Approval, error) {
	return get[port.Approval](t, kindApproval, string(id))
}

func (t *tx) ListApprovals(f port.ApprovalFilter) ([]port.Approval, error) {
	return list(t, kindApproval, want("session", string(f.Session), "agent", string(f.Agent), "connector", string(f.Connector), "call_hash", string(f.CallHash)),
		"(doc->>'created_at')::timestamptz, k", f.Match)
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

func (t *tx) ListPauses() ([]port.Pause, error) { return list[port.Pause](t, kindPause, nil, "k", nil) }

func (t *tx) PutPause(p port.Pause) error {
	return t.set(kindPause, pauseKey(p.Tenant, p.Session, p.Agent, p.Connector), p.Tenant, p)
}

func (t *tx) DeletePause(id port.PauseID) error {
	n, err := t.exec(`DELETE FROM records WHERE kind = 'pause' AND doc->>'id' = $1`, string(id))
	if err == nil && n == 0 {
		return port.Errorf(port.CodeNotFound, "pause %s not found", id)
	}
	return err
}

func tombstoneKey(tenant port.TenantID, kind port.TombstoneKind, subject string, ws port.WorkspaceID) string {
	return key(string(tenant), string(kind), subject, string(ws))
}

func (t *tx) ListTombstones(kind port.TombstoneKind) ([]port.Tombstone, error) {
	return list[port.Tombstone](t, kindTombstone, want("kind", string(kind)), "k", nil)
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
	return list(t, kindCredential, want("owner_kind", string(f.OwnerKind), "owner_id", f.OwnerID, "connector", string(f.Connector)), "k", f.Match)
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
	return list(t, kindOverlay, want("function", function), "(doc->>'overlay_version')::int, k",
		func(o port.Overlay) bool { return o.Targets(connector, listing, function) })
}

func (t *tx) PutOverlay(o port.Overlay) error { return t.set(kindOverlay, overlayKey(o), o.Tenant, o) }

func (t *tx) GetCall(id port.CallID) (port.Call, error) {
	return get[port.Call](t, kindCall, string(id))
}

func (t *tx) LatestCall(session port.SessionID, callHash port.Hash) (port.Call, bool, error) {
	calls, err := list[port.Call](t, kindCall, want("session", string(session), "call_hash", string(callHash)), "(doc->>'started_at')::timestamptz DESC, k LIMIT 1", nil)
	if err != nil || len(calls) == 0 {
		return port.Call{}, false, err
	}
	return calls[0], true, nil
}

func (t *tx) CreateCall(c port.Call) error { return t.insert(kindCall, string(c.ID), c.Tenant, nil, c) }

func (t *tx) UpdateCall(c port.Call, from port.CallState) error {
	return t.update(kindCall, string(c.ID), c.Tenant, nil, c, string(from))
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
	return list[port.FanOut](t, kindFanOut, nil, "k", nil)
}

func (t *tx) PutFanOut(f port.FanOut) error {
	return t.set(kindFanOut, string(f.ID), port.Installation, f)
}

func (t *tx) DueWork(now time.Time, limit int) ([]port.WorkItem, error) {
	if !t.scope.Installation {
		return nil, port.Errorf(port.CodePermissionDenied, "due work is read in the installation scope")
	}
	return collect(t, pgx.RowToStructByPos[port.WorkItem], `SELECT tenant_id, kind, id, due FROM due_work WHERE due <= $1 ORDER BY due, tenant_id, kind, id LIMIT nullif($2::int, 0)`, now, max(limit, 0))
}

func (t *tx) Audit(r port.AuditRow) error {
	_, err := t.exec(`INSERT INTO audit (tenant_id, subject, doc) VALUES ($1, $2, $3::jsonb)`, string(r.Tenant), r.Subject, r)
	return err
}

func (t *tx) ListAudit(subject string) ([]port.AuditRow, error) {
	return many[port.AuditRow](t, `SELECT doc FROM audit WHERE $1 = '' OR subject = $1 ORDER BY id`, subject)
}

func (t *tx) Emit(e port.Event) (port.Event, error) {
	if e.Type == "" {
		return e, port.Errorf(port.CodeInvalidArgument, "an event has a type")
	}
	e.Source = port.SourceOf(e.Tenant)
	e.Stream = e.Source
	var id string
	err := t.q.QueryRow(t.ctx, `INSERT INTO streams AS s (stream, tenant_id, last) VALUES ($1, $2, 1)
		ON CONFLICT (stream) DO UPDATE SET last = s.last + 1 RETURNING last, gen_random_uuid()::text`, e.Stream, string(e.Tenant)).Scan(&e.Sequence, &id)
	if err != nil {
		return e, fail(err)
	}
	e.ID = id
	_, err = t.exec(`INSERT INTO outbox (id, tenant_id, stream, sequence, doc) VALUES ($1, $2, $3, $4, $5::jsonb)`, e.ID, string(e.Tenant), e.Stream, e.Sequence, e)
	return e, err
}

func (t *tx) EventsAfter(stream string, sequence int64, limit int) ([]port.Event, error) {
	return many[port.Event](t, `SELECT doc FROM outbox WHERE stream = $1 AND sequence > $2 ORDER BY sequence LIMIT nullif($3::int, 0)`, stream, sequence, max(limit, 0))
}

func (t *tx) SeenInbox(id string) (bool, error) {
	return one[bool](t, `SELECT EXISTS (SELECT 1 FROM inbox WHERE tenant_id = $1 AND id = $2)`, string(t.scope.Tenant), id)
}

func (t *tx) MarkInbox(id string, at time.Time) (bool, error) {
	n, err := t.exec(`INSERT INTO inbox (tenant_id, id, at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, string(t.scope.Tenant), id, at)
	return n == 1, err
}
