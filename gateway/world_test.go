package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/auth/jwks"
	"github.com/versatile-sh/connector-engine/auth/local"
	"github.com/versatile-sh/connector-engine/filter/builtin"
	"github.com/versatile-sh/connector-engine/gateway"
	"github.com/versatile-sh/connector-engine/index/lexical"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/mux"
	rtskill "github.com/versatile-sh/connector-engine/runtime/skill"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

const (
	tenant    = port.TenantID("t1")
	workspace = port.WorkspaceID("w1")
	listing   = "acme.dev/s"
	audience  = "https://engine.test/mcp"
	cpf       = "529.982.247-25"
)

var ref = port.ListingRef{Name: listing}

type world struct {
	*st.Env
	t      *testing.T
	admin  port.Principal
	alice  port.Principal
	conn   port.Connector
	hashes map[string]port.Hash
	issuer *local.Issuer
	srv    *httptest.Server
	gw     *gateway.Gateway
}

type options struct {
	cfg func(*gateway.Config)
}

func definitions() ([]port.RawDefinition, map[string]port.Hash) {
	hashes := map[string]port.Hash{}
	var list []port.RawDefinition
	for _, d := range []struct{ name, desc, extra string }{
		{"search", "Searches the tracker", `"annotations":{"readOnlyHint":true}`},
		{"create_issue", "Creates an issue", ""},
		{"delete_repo", "Deletes a repository", `"annotations":{"destructiveHint":true}`},
		{"report", "Reports a customer", `"annotations":{"readOnlyHint":true},"outputSchema":{"type":"object","properties":{"name":{"type":"string"},"cpf":{"type":"string"},"note":{"type":"string"}}}`},
	} {
		def, h := st.Definition(d.name, d.desc, d.extra)
		hashes[d.name] = h
		list = append(list, def)
	}
	return list, hashes
}

func newWorld(t *testing.T, o options) *world {
	t.Helper()
	w := &world{t: t}
	w.Env = st.New(t, func(_ *service.Config, d *service.Deps) {
		d.Filter, _ = builtin.New(builtin.DefaultLocales...)
		d.Runtime, _ = mux.New(d.Runtime, rtskill.New())
	})
	list, hashes := definitions()
	w.hashes = hashes
	w.Runtime.SetDefinitions(listing, list...)
	w.Runtime.Handle("report", func(json.RawMessage) (port.Result, error) {
		return port.Result{Content: json.RawMessage(`[{"type":"text","text":"cpf ` + cpf + `"}]`),
			Structured: json.RawMessage(`{"name":"Ana","cpf":"` + cpf + `","note":"vip"}`)}, nil
	})
	op := w.Operator()
	st.Ok(w.Svc.CreateTenant(w.Ctx, op, tenant))
	st.Ok(w.Svc.CreateListing(w.Ctx, op, service.CreateListingInput{Manifest: st.Manifest(listing, "1.0.0")}))
	st.Ok(w.Svc.ReviewListing(w.Ctx, op, port.VersionRef{Listing: ref, Version: "1.0.0"}, service.ReviewInput{Functions: []service.ReviewFunction{
		{Name: "search", Hash: hashes["search"], Class: port.ClassRead},
		{Name: "create_issue", Hash: hashes["create_issue"], Class: port.ClassWrite},
		{Name: "delete_repo", Hash: hashes["delete_repo"], Class: port.ClassDestructive},
		{Name: "report", Hash: hashes["report"], Class: port.ClassRead},
	}}))
	w.admin = w.TenantAdmin(tenant)
	for _, r := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
		{ID: "r-ws", SubjectKind: port.SubjectWorkspace, SubjectID: string(workspace), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		st.Ok(w.Svc.PutCatalogRule(w.Ctx, w.admin, r))
	}
	v := st.Ok(w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "s", Listing: ref}))
	w.conn = v.Connector
	w.alice = w.Person(tenant, "alice")
	seen := map[string]port.Hash{}
	for _, f := range st.Ok(w.Svc.ListFunctionsOf(w.Ctx, w.admin, w.conn.ID)) {
		if f.State == port.FunctionQuarantined {
			seen[f.Name] = f.DefinitionHash
		}
	}
	st.Ok(w.Svc.AcceptFunctions(w.Ctx, w.admin, w.conn.ID, seen))
	st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelCustom))
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)

	var err error
	if w.issuer, err = local.New("iss", w.Clock.Now); err != nil {
		t.Fatal(err)
	}
	keys := httptest.NewServer(w.issuer.Handler())
	t.Cleanup(keys.Close)
	verifier, err := jwks.New(jwks.Config{Issuers: []jwks.Issuer{{Name: "iss", URL: keys.URL}}, Now: w.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	cfg := gateway.DefaultConfig(audience)
	cfg.HoldWindow, cfg.HoldPoll = 80*time.Millisecond, 5*time.Millisecond
	if o.cfg != nil {
		o.cfg(&cfg)
	}
	w.gw, err = gateway.New(w.Svc, verifier, lexical.New(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	w.srv = httptest.NewServer(w.gw.Handler())
	t.Cleanup(w.srv.Close)
	return w
}

func (w *world) row(scope port.ScopeKind, scopeID, selector string, mode port.Mode) port.Grant {
	w.t.Helper()
	g := port.Grant{ScopeKind: scope, ScopeID: scopeID, Connector: w.conn.ID, Selector: selector, Mode: mode}
	if len(selector) > 3 && selector[:3] == port.SelectorFunction && scope != port.ScopeTenant {
		g.DefinitionHash = w.hashes[selector[3:]]
	}
	if mode == port.ModeAllow && (scope == port.ScopeAgent || scope == port.ScopeUser) {
		g.ExpiresAt = st.Epoch.Add(30 * 24 * time.Hour)
	}
	return st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, g)).Grant
}

func (w *world) bind(agent string) {
	w.t.Helper()
	st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: port.AgentID(agent), AskRequester: true}))
}

func (w *world) claims(agent, session string) port.Claims {
	return port.Claims{Tenant: tenant, Workspace: workspace, Agent: port.AgentID(agent), Session: port.SessionID(session), OnBehalfOf: "alice",
		Audience: []string{audience}, ExpiresAt: st.Epoch.Add(time.Hour)}
}

func (w *world) token(c port.Claims) string {
	w.t.Helper()
	return st.Ok(w.issuer.Issue(c))
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (w *world) connect(token, version string) (*mcp.ClientSession, error) {
	c := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	return c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: w.srv.URL, HTTPClient: &http.Client{Transport: bearer{token}}}, &mcp.ClientSessionOptions{ProtocolVersion: version})
}

func (w *world) agent(agent, session, version string) (*mcp.ClientSession, string) {
	w.t.Helper()
	tok := w.token(w.claims(agent, session))
	cs, err := w.connect(tok, version)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _ = cs.Close() })
	return cs, tok
}

func names(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, tool := range res.Tools {
		out = append(out, tool.Name)
	}
	return out
}

func callTool(cs *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, error) {
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
}
