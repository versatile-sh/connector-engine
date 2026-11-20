package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
	"github.com/agents-control/connector-engine/service/worker"
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

func TestAFailingItemBacksOffAndDoesNotStarveTimers(t *testing.T) {
	f := newFixture(t)
	st.Ok(f.Svc.SetConnectorLevel(f.Ctx, f.admin, f.conn.ID, port.LevelCustom))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: "w1", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAllow}))
	st.Ok(f.Svc.PutBinding(f.Ctx, f.admin, port.Binding{Connector: f.conn.ID, Agent: "x", AskRequester: true}))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAsk}))
	x := f.AgentClaims(port.Claims{Tenant: "t1", Workspace: "w1", Agent: "x", Session: "s1", OnBehalfOf: "alice"})
	held := st.Ok(f.Svc.Call(f.Ctx, x, service.CallRequest{Name: "s__search", Arguments: []byte(`{"q":"a"}`)}))
	st.Ok(f.Svc.DecideApproval(f.Ctx, f.Person("t1", "alice"), held.Approval.ID, port.DecisionApprove, "", "test"))
	g := st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "y", Connector: f.conn.ID, Selector: "class:read", Mode: port.ModeAllow,
		ExpiresAt: st.Epoch.Add(time.Second)})).Grant
	f.Clock.Advance(2 * time.Second)
	f.Runtime.OnList = func() error { return port.Errorf(port.CodeUnavailable, "upstream down") }
	w := worker.New(f.Svc, f.Store, f.Clock, worker.Config{Batch: 1})
	for range 3 {
		_, _ = w.Sweep(f.Ctx)
	}
	rows := st.Ok(f.Svc.ListGrants(f.Ctx, f.admin, port.GrantFilter{ScopeKind: port.ScopeAgent}))
	if slices.ContainsFunc(rows, func(r port.Grant) bool { return r.ID == g.ID }) {
		t.Fatal("a failing approved record must not starve the grant timer behind it")
	}
	if f.Runtime.CallsTo("search") != 0 {
		t.Fatal("the record whose re-list fails is not forwarded")
	}
}

type brokenStore struct {
	port.Store
	panics bool
}

func (b brokenStore) Read(context.Context, port.Scope, func(port.Tx) error) error {
	if b.panics {
		panic("store bug")
	}
	return port.Errorf(port.CodeUnavailable, "store down")
}

func TestRunLogsCountsAndSurvivesFailingAndPanickingSweeps(t *testing.T) {
	for _, panics := range []bool{false, true} {
		f := newFixture(t)
		var logs bytes.Buffer
		var failures atomic.Int32
		ctx, cancel := context.WithCancel(f.Ctx)
		w := worker.New(f.Svc, brokenStore{Store: f.Store, panics: panics}, f.Clock, worker.Config{Interval: time.Millisecond,
			Log: slog.New(slog.NewTextHandler(&logs, nil)), Failed: func() {
				if failures.Add(1) == 3 {
					cancel()
				}
			}})
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("panics=%v: Run did not report three failed sweeps", panics)
		}
		if !strings.Contains(logs.String(), "worker sweep failed") {
			t.Fatalf("panics=%v: no log line: %q", panics, logs.String())
		}
	}
}

func heldAndApproved(t *testing.T, f *fixture) port.ApprovalID {
	t.Helper()
	st.Ok(f.Svc.SetConnectorLevel(f.Ctx, f.admin, f.conn.ID, port.LevelCustom))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: "w1", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAllow}))
	st.Ok(f.Svc.PutBinding(f.Ctx, f.admin, port.Binding{Connector: f.conn.ID, Agent: "x", AskRequester: true}))
	st.Ok(f.Svc.CreateGrant(f.Ctx, f.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: f.conn.ID, Selector: "*", Mode: port.ModeAsk}))
	x := f.AgentClaims(port.Claims{Tenant: "t1", Workspace: "w1", Agent: "x", Session: "s1", OnBehalfOf: "alice"})
	held := st.Ok(f.Svc.Call(f.Ctx, x, service.CallRequest{Name: "s__search", Arguments: []byte(`{"q":"a"}`)}))
	st.Ok(f.Svc.DecideApproval(f.Ctx, f.Person("t1", "alice"), held.Approval.ID, port.DecisionApprove, "", "test"))
	return held.Approval.ID
}

func TestShutdownLetsARunningApprovedExecutionFinishWithinTheGrace(t *testing.T) {
	for _, c := range []struct {
		grace time.Duration
		state port.ApprovalState
	}{{time.Minute, port.ApprovalExecuted}, {10 * time.Millisecond, port.ApprovalExecuting}} {
		f := newFixture(t)
		f.Clock.Advance(time.Since(st.Epoch))
		id := heldAndApproved(t, f)
		started, release := make(chan struct{}), make(chan struct{})
		f.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
			close(started)
			<-release
			return port.Result{Structured: json.RawMessage(`{"ok":true}`)}, nil
		})
		ctx, cancel := context.WithCancel(f.Ctx)
		w := worker.New(f.Svc, f.Store, f.Clock, worker.Config{Interval: time.Hour, Grace: c.grace})
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx) }()
		<-started
		cancel()
		if c.grace < time.Second {
			time.Sleep(100 * time.Millisecond)
		}
		close(release)
		<-done
		got := st.Ok(f.Svc.GetApproval(f.Ctx, f.Person("t1", "alice"), id)).Approval.State
		if got != c.state {
			t.Fatalf("grace %s: state %s, want %s", c.grace, got, c.state)
		}
	}
}
