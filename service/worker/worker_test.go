package worker_test

import (
	"slices"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

type fixture struct {
	*st.Env
	admin  port.Principal
	conn   port.Connector
	hashes map[string]port.Hash
}

func newFixture(t *testing.T) *fixture {
	e := st.New(t)
	op, ref := e.Operator(), port.ListingRef{Name: "acme.dev/s"}
	st.Ok(e.Svc.CreateTenant(e.Ctx, op, "t1"))
	def, h := st.Definition("search", "Searches", "")
	e.Runtime.SetDefinitions("acme.dev/s", def)
	st.Ok(e.Svc.CreateListing(e.Ctx, op, service.CreateListingInput{Manifest: st.Manifest("acme.dev/s", "1")}))
	st.Ok(e.Svc.ReviewListing(e.Ctx, op, port.VersionRef{Listing: ref, Version: "1"}, service.ReviewInput{Functions: []service.ReviewFunction{{Name: "search", Hash: h, Class: port.ClassRead}}}))
	admin := e.TenantAdmin("t1")
	for _, sub := range []struct {
		kind port.SubjectKind
		id   string
	}{{port.SubjectTenant, "t1"}, {port.SubjectWorkspace, "w1"}} {
		st.Ok(e.Svc.PutCatalogRule(e.Ctx, admin, port.CatalogRule{SubjectKind: sub.kind, SubjectID: sub.id, TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull}))
	}
	v := st.Ok(e.Svc.InstallConnector(e.Ctx, admin, service.InstallInput{Workspace: "w1", Alias: "s", Listing: ref}))
	return &fixture{Env: e, admin: admin, conn: v.Connector, hashes: map[string]port.Hash{"search": h}}
}

func TestSweepRelistsDueConnectorsAndQuarantinesDrift(t *testing.T) {
	f := newFixture(t)
	drift, _ := st.Definition("search", "Searches, but different", "")
	f.Runtime.SetDefinitions("acme.dev/s", drift)

	stats := st.Ok(f.Worker.Sweep(f.Ctx))
	if stats.Relists != 0 {
		t.Fatalf("nothing is due yet: %+v", stats)
	}
	f.Clock.Advance(301 * time.Second)
	stats = st.Ok(f.Worker.Sweep(f.Ctx))
	if stats.Relists != 1 {
		t.Fatalf("one connector is due: %+v", stats)
	}
	fns := st.Ok(f.Svc.ListFunctionsOf(f.Ctx, f.admin, f.conn.ID))
	if fns[0].State != port.FunctionQuarantined {
		t.Fatalf("drift must quarantine: %s", fns[0].State)
	}
	if !slices.Contains(f.Types(0), "connector.function.quarantined") {
		t.Fatal("event 6 missing")
	}
	stats = st.Ok(f.Worker.Sweep(f.Ctx))
	if stats.Relists != 0 {
		t.Fatalf("a re-list moves the next one out: %+v", stats)
	}
}

func TestSweepExpiresGrantsWithEvent(t *testing.T) {
	f := newFixture(t)
	g := st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: f.conn.ID, Selector: "class:read", Mode: port.ModeAllow,
		ExpiresAt: st.Epoch.Add(time.Hour)})).Grant
	st.Ok(f.Worker.Sweep(f.Ctx))
	rows := st.Ok(f.Svc.ListGrants(f.Ctx, f.admin, port.GrantFilter{ScopeKind: port.ScopeAgent}))
	if !slices.ContainsFunc(rows, func(r port.Grant) bool { return r.ID == g.ID }) {
		t.Fatal("the row lives until it expires")
	}
	f.Clock.Advance(time.Hour)
	st.Ok(f.Worker.Sweep(f.Ctx))
	rows = st.Ok(f.Svc.ListGrants(f.Ctx, f.admin, port.GrantFilter{ScopeKind: port.ScopeAgent}))
	if slices.ContainsFunc(rows, func(r port.Grant) bool { return r.ID == g.ID }) {
		t.Fatal("the row must be gone")
	}
	if !slices.Contains(f.Types(0), "connector.grant.expired") {
		t.Fatal("event 12 missing")
	}
}

func TestSweepDestroysResultsAfterRetention(t *testing.T) {
	f := newFixture(t)
	st.Ok(f.Svc.SetConnectorLevel(f.Ctx, f.admin, f.conn.ID, port.LevelCustom))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: "w1", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAllow}))
	st.Ok(f.Svc.PutBinding(f.Ctx, f.admin, port.Binding{Connector: f.conn.ID, Agent: "x", AskRequester: true}))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAsk}))
	x := f.AgentClaims(port.Claims{Tenant: "t1", Workspace: "w1", Agent: "x", Session: "s1", OnBehalfOf: "alice"})
	alice := f.Person("t1", "alice")
	held := st.Ok(f.Svc.Call(f.Ctx, x, service.CallRequest{Name: "s__search", Arguments: []byte(`{"q":"a"}`)}))
	st.Ok(f.Svc.DecideApproval(f.Ctx, alice, held.Approval.ID, port.DecisionApprove, "", "test"))
	st.Ok(f.Worker.Sweep(f.Ctx))
	if got := st.Ok(f.Svc.ApprovalStatus(f.Ctx, x, held.Approval.ID)); got.Status != service.OutcomeResult {
		t.Fatalf("result expected: %+v", got)
	}
	f.Clock.Advance(time.Hour + time.Second)
	st.Ok(f.Worker.Sweep(f.Ctx))
	got := st.Ok(f.Svc.ApprovalStatus(f.Ctx, x, held.Approval.ID))
	if got.Code != service.CodeResultNotKept {
		t.Fatalf("A13 must destroy the stored result: %+v", got)
	}
	detail := st.Ok(f.Svc.GetApproval(f.Ctx, alice, held.Approval.ID))
	if detail.Arguments != nil || detail.Approval.State != port.ApprovalExecuted {
		t.Fatalf("sealed arguments are destroyed, the state stays: %+v", detail)
	}
}
