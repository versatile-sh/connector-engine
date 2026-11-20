package console

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

type items[T any] struct {
	Items []T `json:"items"`
}

type connectorView struct {
	Connector port.Connector  `json:"connector"`
	Functions []port.Function `json:"functions,omitempty"`
	Blocked   string          `json:"blocked"`
}

type approvalDetail struct {
	Approval       port.Approval `json:"approval"`
	Arguments      []byte        `json:"arguments,omitempty"`
	MayHaveRun     bool          `json:"may_have_run"`
	ReachesOutside bool          `json:"reaches_outside"`
	SessionUntil   time.Time     `json:"session_until,omitzero"`
}

func q(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return "?" + v.Encode()
}

func esc(s string) string { return url.PathEscape(s) }

func connectorPath(conn string, rest ...string) string {
	p := "/v1/connectors/" + esc(conn)
	for _, r := range rest {
		p += r
	}
	return p
}

// IsCode reports whether err is an engine refusal with this code.
func IsCode(err error, code string) bool {
	var ee *EngineError
	return errors.As(err, &ee) && ee.Code == code
}

func list[T any](ctx context.Context, e *Engine, path string) ([]T, error) {
	var out items[T]
	err := e.Call(ctx, http.MethodGet, path, nil, &out)
	return out.Items, err
}

func (e *Engine) connectors(ctx context.Context) ([]connectorView, error) {
	return list[connectorView](ctx, e, "/v1/connectors"+q("workspace", e.Workspace))
}

func (e *Engine) liveConnectors(ctx context.Context) ([]port.Connector, error) {
	views, err := e.connectors(ctx)
	var out []port.Connector
	for _, v := range views {
		if v.Connector.RemovedAt.IsZero() {
			out = append(out, v.Connector)
		}
	}
	return out, err
}

func (e *Engine) connector(ctx context.Context, conn string) (port.Connector, error) {
	var v connectorView
	err := e.Call(ctx, http.MethodGet, connectorPath(conn), nil, &v)
	return v.Connector, err
}

func (e *Engine) install(ctx context.Context, alias string, ref port.VersionRef) (string, error) {
	var v connectorView
	err := e.Call(ctx, http.MethodPost, "/v1/connectors", map[string]any{"workspace": e.Workspace, "alias": alias, "listing": ref.Listing, "version": ref.Version}, &v)
	return string(v.Connector.ID), err
}

func (e *Engine) removeConnector(ctx context.Context, conn string) error {
	return e.Call(ctx, http.MethodDelete, connectorPath(conn), nil, nil)
}

func (e *Engine) version(ctx context.Context, ref port.VersionRef) (port.ListingVersion, error) {
	var lv port.ListingVersion
	err := e.Call(ctx, http.MethodGet, "/v1/listings/version"+q("workspace", e.Workspace, "owner", string(ref.Listing.Owner), "name", ref.Listing.Name, "version", ref.Version), nil, &lv)
	return lv, err
}

func (e *Engine) manifest(ctx context.Context, conn port.Connector) (port.Manifest, error) {
	lv, err := e.version(ctx, conn.Listing)
	return lv.Manifest, err
}

func (e *Engine) createListing(ctx context.Context, m port.Manifest) (port.ListingVersion, error) {
	var lv port.ListingVersion
	err := e.Call(ctx, http.MethodPost, "/v1/listings", map[string]any{"manifest": m}, &lv)
	if IsCode(err, "conflict") {
		return e.version(ctx, port.VersionRef{Listing: port.ListingRef{Owner: port.TenantID(e.Tenant), Name: m.Name}, Version: m.Version})
	}
	return lv, err
}

func (e *Engine) functions(ctx context.Context, conn string) ([]port.Function, error) {
	return list[port.Function](ctx, e, connectorPath(conn, "/functions"))
}

func (e *Engine) accept(ctx context.Context, conn string, seen map[string]port.Hash) error {
	return e.Call(ctx, http.MethodPost, connectorPath(conn, "/functions:accept"), map[string]any{"seen": seen}, nil)
}

func (e *Engine) setLevel(ctx context.Context, conn string, level port.Level) error {
	return e.Call(ctx, http.MethodPut, connectorPath(conn, "/level"), map[string]any{"level": level}, nil)
}

func (e *Engine) setClass(ctx context.Context, conn, function string, class port.Class) (port.Function, error) {
	var f port.Function
	err := e.Call(ctx, http.MethodPut, connectorPath(conn, "/functions/", esc(function), "/class"), map[string]any{"class": class}, &f)
	return f, err
}

func (e *Engine) bind(ctx context.Context, conn, agent string) error {
	return e.Call(ctx, http.MethodPut, connectorPath(conn, "/bindings/", esc(agent)), map[string]any{"credential_policy": port.CredentialShared, "ask_requester": true}, nil)
}

func (e *Engine) unbind(ctx context.Context, conn, agent string) error {
	if err := e.Call(ctx, http.MethodDelete, connectorPath(conn, "/bindings/", esc(agent)), nil, nil); err != nil && !IsCode(err, "not_found") {
		return err
	}
	return nil
}

func (e *Engine) bindings(ctx context.Context, conn string) ([]port.Binding, error) {
	return list[port.Binding](ctx, e, connectorPath(conn, "/bindings"))
}

func (e *Engine) grants(ctx context.Context, kind port.ScopeKind, scopeID, conn string) ([]port.Grant, error) {
	return list[port.Grant](ctx, e, "/v1/grants"+q("scope_kind", string(kind), "scope_id", scopeID, "connector", conn))
}

func (e *Engine) createGrant(ctx context.Context, g port.Grant) error {
	return e.Call(ctx, http.MethodPost, "/v1/grants", g, nil)
}

func (e *Engine) deleteGrants(ctx context.Context, rows []port.Grant) error {
	for _, g := range rows {
		if err := e.Call(ctx, http.MethodDelete, "/v1/grants/"+esc(string(g.ID)), nil, nil); err != nil && !IsCode(err, "not_found") {
			return err
		}
	}
	return nil
}

func (e *Engine) confirmGrant(ctx context.Context, id port.GrantID) error {
	return e.Call(ctx, http.MethodPost, "/v1/grants/"+esc(string(id))+":confirm", nil, nil)
}

func (e *Engine) explain(ctx context.Context, conn, agent, function string) (port.AccessDecision, error) {
	var d port.AccessDecision
	err := e.Call(ctx, http.MethodPost, "/v1/access:explain", map[string]any{"workspace": e.Workspace, "agent": agent, "on_behalf_of": e.Person, "connector": conn, "function": function}, &d)
	return d, err
}

func (e *Engine) catalogRules(ctx context.Context) ([]port.CatalogRule, error) {
	return list[port.CatalogRule](ctx, e, "/v1/catalog/rules")
}

func (e *Engine) putCatalogRule(ctx context.Context, r port.CatalogRule) error {
	return e.Call(ctx, http.MethodPut, "/v1/catalog/rules", r, nil)
}

func (e *Engine) credentials(ctx context.Context, conn string) ([]port.Credential, error) {
	return list[port.Credential](ctx, e, "/v1/credentials"+q("connector", conn))
}

func (e *Engine) pendingApprovals(ctx context.Context, agent, session string) ([]port.Approval, error) {
	return list[port.Approval](ctx, e, "/v1/approvals"+q("state", string(port.ApprovalPending), "agent", agent, "session", session))
}

func (e *Engine) approval(ctx context.Context, id string) (approvalDetail, error) {
	var d approvalDetail
	err := e.Call(ctx, http.MethodGet, "/v1/approvals/"+esc(id), nil, &d)
	return d, err
}

func (e *Engine) decide(ctx context.Context, id string, body map[string]any) (port.Approval, error) {
	var out port.Approval
	err := e.Call(ctx, http.MethodPost, "/v1/approvals/"+esc(id)+":decide", body, &out)
	return out, err
}
