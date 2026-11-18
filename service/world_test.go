package service_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

const (
	tenant    = port.TenantID("t1")
	workspace = port.WorkspaceID("w1")
	listing   = "acme.dev/s"
)

var ref = port.ListingRef{Name: listing}

type world struct {
	*st.Env
	t      *testing.T
	admin  port.Principal
	alice  port.Principal
	conn   port.Connector
	hashes map[string]port.Hash
	defs   map[string]port.RawDefinition
}

func definitions(search, issue string) (map[string]port.RawDefinition, map[string]port.Hash, []port.RawDefinition) {
	defs, hashes := map[string]port.RawDefinition{}, map[string]port.Hash{}
	var list []port.RawDefinition
	for _, d := range []struct{ name, desc, extra string }{
		{"search", search, `"annotations":{"readOnlyHint":true}`},
		{"create_issue", issue, ""},
		{"delete_repo", "Deletes a repository", `"annotations":{"destructiveHint":true}`},
		{"export", "Exports data", ""},
	} {
		def, h := st.Definition(d.name, d.desc, d.extra)
		defs[d.name], hashes[d.name] = def, h
		list = append(list, def)
	}
	return defs, hashes, list
}

type opts struct {
	ceiling  port.Ceiling
	reviewed bool
	auth     bool
	tune     []func(*service.Config, *service.Deps)
}

func newWorld(t *testing.T, o opts) *world {
	t.Helper()
	if o.ceiling == "" {
		o.ceiling = port.CeilingFull
	}
	w := &world{Env: st.New(t, o.tune...), t: t}
	w.defs, w.hashes, _ = definitions("Searches", "Creates an issue")
	_, _, list := definitions("Searches", "Creates an issue")
	w.Runtime.SetDefinitions(listing, list...)
	op := w.Operator()
	st.Ok(w.Svc.CreateTenant(w.Ctx, op, tenant))
	m := st.Manifest(listing, "1.0.0")
	if o.auth {
		m.Auth = []port.AuthSpec{{Kind: port.CredentialAPIKey, OwnerKindsAllowed: []port.OwnerKind{port.OwnerUser}}}
	}
	st.Ok(w.Svc.CreateListing(w.Ctx, op, service.CreateListingInput{Manifest: m}))
	if o.reviewed {
		in := service.ReviewInput{Functions: []service.ReviewFunction{
			{Name: "search", Hash: w.hashes["search"], Class: port.ClassRead},
			{Name: "create_issue", Hash: w.hashes["create_issue"], Class: port.ClassWrite},
			{Name: "delete_repo", Hash: w.hashes["delete_repo"], Class: port.ClassDestructive},
		}}
		st.Ok(w.Svc.ReviewListing(w.Ctx, op, port.VersionRef{Listing: ref, Version: "1.0.0"}, in))
	}
	w.admin = w.TenantAdmin(tenant)
	st.Ok(w.Svc.PutCatalogRule(w.Ctx, w.admin, port.CatalogRule{SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing, Target: ref.String(),
		Show: true, Install: true, Ceiling: o.ceiling}))
	st.Ok(w.Svc.PutCatalogRule(w.Ctx, w.admin, port.CatalogRule{ID: "r-ws", SubjectKind: port.SubjectWorkspace, SubjectID: string(workspace), TargetKind: port.TargetListing, Target: ref.String(),
		Show: true, Install: true, Ceiling: port.CeilingFull}))
	v := st.Ok(w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "s", Listing: ref}))
	w.conn = v.Connector
	w.alice = w.Person(tenant, "alice")
	return w
}

func (w *world) agent(agent, session string) port.Principal {
	return w.AgentClaims(port.Claims{Tenant: tenant, Workspace: workspace, Agent: port.AgentID(agent), Session: port.SessionID(session), OnBehalfOf: "alice"})
}

func (w *world) acceptAll() {
	w.t.Helper()
	seen := map[string]port.Hash{}
	for _, f := range st.Ok(w.Svc.ListFunctionsOf(w.Ctx, w.admin, w.conn.ID)) {
		if f.State == port.FunctionQuarantined {
			seen[f.Name] = f.DefinitionHash
		}
	}
	if len(seen) > 0 {
		st.Ok(w.Svc.AcceptFunctions(w.Ctx, w.admin, w.conn.ID, seen))
	}
}

func (w *world) bind(agent string) {
	w.t.Helper()
	st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: port.AgentID(agent), AskRequester: true}))
}

func (w *world) customWorkspace() {
	w.t.Helper()
	st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, w.conn.ID, port.LevelCustom))
}

func (w *world) row(scope port.ScopeKind, scopeID, selector string, mode port.Mode) port.Grant {
	w.t.Helper()
	g := port.Grant{ScopeKind: scope, ScopeID: scopeID, Connector: w.conn.ID, Selector: selector, Mode: mode}
	if scope == port.ScopeTenant {
		g.Connector, g.Listing = "", &ref
	}
	if len(selector) > 3 && selector[:3] == port.SelectorFunction && scope != port.ScopeTenant {
		g.DefinitionHash = w.hashes[selector[3:]]
	}
	if mode == port.ModeAllow && (scope == port.ScopeAgent || scope == port.ScopeUser) {
		g.ExpiresAt = st.Epoch.Add(30 * 24 * time.Hour)
	}
	return st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, g)).Grant
}

func (w *world) allowAll(agent string) {
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind(agent)
	w.row(port.ScopeAgent, agent, "*", port.ModeAllow)
}

func (w *world) names(p port.Principal) []string {
	w.t.Helper()
	l := st.Ok(w.Svc.ListFunctions(w.Ctx, p))
	out := []string{}
	for _, f := range l.Functions {
		out = append(out, f.Name)
	}
	return out
}

func (w *world) call(p port.Principal, name, args string) service.Outcome {
	w.t.Helper()
	return st.Ok(w.Svc.Call(w.Ctx, p, service.CallRequest{Name: name, Arguments: json.RawMessage(args)}))
}

func (w *world) decide(id port.ApprovalID, d port.Decision) port.Approval {
	w.t.Helper()
	return st.Ok(w.Svc.DecideApproval(w.Ctx, w.alice, id, d, "", "test"))
}

func (w *world) sweep() {
	w.t.Helper()
	if _, err := w.Worker.Sweep(w.Ctx); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) redefine(name, description string) []port.RawDefinition {
	_, _, list := definitions("Searches", "Creates an issue")
	for i, d := range list {
		if d.Name == name {
			list[i], _ = st.Definition(name, description, "")
		}
	}
	return list
}
