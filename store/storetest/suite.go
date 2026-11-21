// Package storetest is the conformance suite every port.Store and port.Outbox adapter must pass.
package storetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

var (
	ctx    = context.Background()
	scopeA = port.Scope{Tenant: "ta"}
	scopeB = port.Scope{Tenant: "tb"}
	inst   = port.Scope{Installation: true}
	t0     = time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
	errOK  = errors.New("rolled back on purpose")
)

type env struct {
	t     *testing.T
	s     port.Store
	o     port.Outbox
	fresh func() (port.Store, port.Outbox)
}

// Run executes the whole suite, each case on a fresh store from newStore.
func Run(t *testing.T, newStore func(t *testing.T) (port.Store, port.Outbox)) {
	cases := map[string]func(*env){
		"tenant isolation":                 isolation,
		"cross-tenant matrix":              crossTenantMatrix,
		"installation rows":                installationRows,
		"unit of work":                     unitOfWork,
		"read only":                        readOnly,
		"outbox order per stream":          outboxOrder,
		"concurrent emit":                  concurrentEmit,
		"conditional approval":             conditionalApproval,
		"one open record per call":         oneOpenRecord,
		"concurrent decisions":             concurrentDecisions,
		"conditional call":                 conditionalCall,
		"revisions":                        revisions,
		"grant key":                        grantKey,
		"counted grant reservation":        countedReservation,
		"concurrent reservation":           concurrentReservation,
		"grant row keeps use count":        writingAGrantRowKeepsItsUseCount,
		"due work":                         dueWork,
		"inbox":                            inbox,
		"scope validation":                 scopeValidation,
		"records":                          records,
		"skill version":                    skillVersion,
		"roles and groups":                 rolesAndGroups,
		"calls clients and syncs":          callsClientsAndSyncs,
		"purge tenant":                     purgeTenant,
		"prune history":                    pruneHistory,
		"snapshot and restore":             snapshotRestore,
		"restore normalizes agent":         restoreNormalizesAgent,
		"restore refuses broken snapshots": restoreRefusesBrokenSnapshots,
		"restore records itself":           restoreRecordsItself,
		"restore keeps the first warnings": restoreKeepsTheFirstWarnings,
		"permission revision":              revisionGrowsByOnePerWritingTransaction,
		"permission values":                permissionValuesAsGiven,
		"permission paging":                permissionChangesPage,
		"permission tenant scope":          permissionLogIsTenantScoped,
		"permission consistent read":       consistentReadSeesOneRevision,
		"permission log on restore":        restoreKeepsTheLivePermissionLog,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			s, o := newStore(t)
			fn(&env{t: t, s: s, o: o, fresh: func() (port.Store, port.Outbox) { return newStore(t) }})
		})
	}
}

func (e *env) write(scope port.Scope, fn func(port.Tx) error) error { return e.s.Write(ctx, scope, fn) }

func (e *env) read(scope port.Scope, fn func(port.Tx) error) error { return e.s.Read(ctx, scope, fn) }

func (e *env) must(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) code(err error, want port.Code) {
	e.t.Helper()
	if !port.IsCode(err, want) {
		e.t.Fatalf("got %v, want code %s", err, want)
	}
}

func (e *env) same(want, got any) {
	e.t.Helper()
	w, _ := json.Marshal(want)
	g, _ := json.Marshal(got)
	if string(w) != string(g) {
		e.t.Fatalf("got %s, want %s", g, w)
	}
}

func connector(tenant port.TenantID, id string) port.Connector {
	return port.Connector{ID: port.ConnectorID(id), Tenant: tenant, Workspace: "w", Alias: id, Enabled: true,
		Listing: port.VersionRef{Listing: port.ListingRef{Name: "io.x/svc"}, Version: "1"}}
}

func agentGrant(tenant port.TenantID, id, sel string, maxUses int) port.Grant {
	return port.Grant{ID: port.GrantID(id), Tenant: tenant, ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: "c1", Selector: sel,
		Mode: port.ModeAllow, MaxUses: maxUses, CreatedAt: t0, Origin: port.OriginSettings}
}

func pending(tenant port.TenantID, id, hash string) port.Approval {
	return port.Approval{ID: port.ApprovalID(id), Tenant: tenant, Workspace: "w", Agent: "x", Session: "s1", Connector: "c1",
		Function: "f", CallHash: port.Hash(hash), Mode: port.ModeAsk, Approvers: []string{"u1"}, State: port.ApprovalPending,
		CreatedAt: t0, ExpiresAt: t0.Add(time.Minute), Arguments: &port.Sealed{Ciphertext: []byte("x"), KeyRef: "k"}}
}

func isolation(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error {
		if _, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision); err != nil {
			return err
		}
		if err := tx.PutGrant(agentGrant("ta", "g1", "*", 0)); err != nil {
			return err
		}
		if err := tx.CreateApproval(pending("ta", "a1", "h")); err != nil {
			return err
		}
		return tx.PutRule(port.CatalogRule{ID: "r1", Tenant: "ta", SubjectKind: port.SubjectTenant, SubjectID: "ta", TargetKind: port.TargetListing, Target: "/io.x/svc", Show: true, Ceiling: port.CeilingRead})
	}))
	for _, scope := range []port.Scope{scopeB, inst} {
		e.must(e.read(scope, func(tx port.Tx) error {
			_, err := tx.GetConnector("c1")
			e.code(err, port.CodeNotFound)
			_, err = tx.GetGrant("g1")
			e.code(err, port.CodeNotFound)
			_, err = tx.GetApproval("a1")
			e.code(err, port.CodeNotFound)
			cs, _ := tx.ListConnectors(port.ConnectorFilter{})
			gs, _ := tx.ListGrants(port.GrantFilter{})
			rs, _ := tx.ListRules()
			as, _ := tx.ListApprovals(port.ApprovalFilter{})
			if len(cs)+len(gs)+len(rs)+len(as) != 0 {
				e.t.Fatalf("scope %+v reads rows of tenant ta", scope)
			}
			return nil
		}))
	}
	err := e.write(scopeB, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("tb", "c1"), port.AnyRevision)
		return err
	})
	if err == nil {
		e.t.Fatal("tenant tb overwrote a row of tenant ta")
	}
	e.code(e.write(scopeB, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("ta", "c2"), port.AnyRevision)
		return err
	}), port.CodePermissionDenied)
	e.code(e.write(scopeB, func(tx port.Tx) error { return tx.DeleteGrant("g1") }), port.CodeNotFound)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		c, err := tx.GetConnector("c1")
		e.same("ta", c.Tenant)
		return err
	}))
}

func installationRows(e *env) {
	shared := port.Listing{Ref: port.ListingRef{Name: "io.x/svc"}, Source: "own", Publisher: "x", Status: port.ListingActive}
	private := port.Listing{Ref: port.ListingRef{Owner: "ta", Name: "io.x/crm"}, Source: port.PrivateSource, Publisher: "ta", Status: port.ListingActive}
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.PutListing(shared) }), port.CodePermissionDenied)
	e.must(e.write(inst, func(tx port.Tx) error {
		if err := tx.PutListing(shared); err != nil {
			return err
		}
		if err := tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantActive, CreatedAt: t0}); err != nil {
			return err
		}
		if err := tx.PutTenant(port.Tenant{ID: "tb", State: port.TenantActive, CreatedAt: t0}); err != nil {
			return err
		}
		return tx.PutOffer(port.Offer{Tenant: "ta", TargetKind: port.TargetListing, Target: "/io.x/svc", Withheld: true})
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.PutListing(private) }))
	e.code(e.write(scopeA, func(tx port.Tx) error {
		return tx.PutOffer(port.Offer{Tenant: "ta", TargetKind: port.TargetSource, Target: "own"})
	}), port.CodePermissionDenied)
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantRemoved}) }), port.CodePermissionDenied)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		ls, _ := tx.ListListings()
		os, _ := tx.ListOffers("ta")
		ts, _ := tx.ListTenants()
		e.same([]string{"/io.x/svc", "ta/io.x/crm"}, refs(ls))
		e.same(1, len(os))
		e.same(1, len(ts))
		return nil
	}))
	e.must(e.read(scopeB, func(tx port.Tx) error {
		ls, _ := tx.ListListings()
		os, _ := tx.ListOffers("ta")
		_, err := tx.GetTenant("ta")
		e.code(err, port.CodeNotFound)
		_, err = tx.GetListing(private.Ref)
		e.code(err, port.CodeNotFound)
		e.same([]string{"/io.x/svc"}, refs(ls))
		e.same(0, len(os))
		return nil
	}))
	e.must(e.read(inst, func(tx port.Tx) error {
		ls, _ := tx.ListListings()
		ts, _ := tx.ListTenants()
		e.same([]string{"/io.x/svc"}, refs(ls))
		e.same(2, len(ts))
		return nil
	}))
	both := port.Scope{Tenant: "tc", Installation: true}
	e.must(e.write(both, func(tx port.Tx) error {
		if err := tx.PutTenant(port.Tenant{ID: "tc", State: port.TenantActive, CreatedAt: t0}); err != nil {
			return err
		}
		if err := tx.PutRule(port.CatalogRule{ID: "starter", Tenant: "tc", SubjectKind: port.SubjectTenant, SubjectID: "tc", TargetKind: port.TargetListing, Target: "/io.x/svc", Show: true, Install: true, Ceiling: port.CeilingRead, CreatedBy: port.StarterCreator}); err != nil {
			return err
		}
		if _, err := tx.Emit(port.Event{Type: "connector.tenant.created", Time: t0}); err != nil {
			return err
		}
		_, err := tx.Emit(port.Event{Type: "connector.catalog.rule_changed", Tenant: "tc", Time: t0})
		return err
	}))
	ev, err := e.o.Pending(ctx, 0)
	e.must(err)
	e.same([]string{port.SourceOf(port.Installation), port.SourceOf("tc")}, []string{ev[0].Source, ev[1].Source})
}

func refs(ls []port.Listing) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Ref.String())
	}
	slices.Sort(out)
	return out
}

func unitOfWork(e *env) {
	err := e.write(scopeA, func(tx port.Tx) error {
		if _, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision); err != nil {
			return err
		}
		if err := tx.Audit(port.AuditRow{At: t0, Tenant: "ta", Actor: "u", Action: "installConnector", Subject: "c1"}); err != nil {
			return err
		}
		if _, err := tx.Emit(port.Event{Type: "connector.connector.installed", Tenant: "ta", Time: t0}); err != nil {
			return err
		}
		return errOK
	})
	if !errors.Is(err, errOK) {
		e.t.Fatalf("the error of the unit of work is returned: %v", err)
	}
	e.must(e.read(scopeA, func(tx port.Tx) error {
		_, err := tx.GetConnector("c1")
		e.code(err, port.CodeNotFound)
		rows, _ := tx.ListAudit("")
		e.same(0, len(rows))
		return nil
	}))
	if ev, _ := e.o.Pending(ctx, 0); len(ev) != 0 {
		e.t.Fatal("an event of a rolled-back unit of work is pending")
	}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		if _, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision); err != nil {
			return err
		}
		if err := tx.Audit(port.AuditRow{At: t0, Tenant: "ta", Actor: "u", Action: "installConnector", Subject: "c1"}); err != nil {
			return err
		}
		_, err := tx.Emit(port.Event{Type: "connector.connector.installed", Tenant: "ta", Subject: "c1", Time: t0, Data: map[string]any{"connector": "c1"}})
		return err
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		rows, _ := tx.ListAudit("c1")
		e.same(1, len(rows))
		return nil
	}))
	ev, _ := e.o.Pending(ctx, 0)
	if len(ev) != 1 || ev[0].Data["connector"] != "c1" || ev[0].ID == "" || ev[0].Sequence != 1 {
		e.t.Fatalf("committed event: %+v", ev)
	}
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.Audit(port.AuditRow{Tenant: "tb", Action: "x"}) }), port.CodePermissionDenied)
	e.code(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "x", Tenant: "tb"})
		return err
	}), port.CodePermissionDenied)
}

func readOnly(e *env) {
	err := e.read(scopeA, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision)
		return err
	})
	if err == nil {
		e.t.Fatal("a read transaction wrote")
	}
	err = e.read(scopeA, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "x", Tenant: "ta"})
		return err
	})
	if err == nil {
		e.t.Fatal("a read transaction emitted")
	}
}

func emit(e *env, scope port.Scope, tenant port.TenantID, typ string) {
	e.t.Helper()
	e.must(e.write(scope, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: typ, Tenant: tenant, Time: t0})
		return err
	}))
}

func outboxOrder(e *env) {
	for i := range 3 {
		emit(e, scopeA, "ta", fmt.Sprintf("a%d", i))
		emit(e, scopeB, "tb", fmt.Sprintf("b%d", i))
		emit(e, inst, port.Installation, fmt.Sprintf("i%d", i))
	}
	ev, err := e.o.Pending(ctx, 0)
	e.must(err)
	var order []string
	for _, x := range ev {
		order = append(order, x.Type)
	}
	e.same([]string{"a0", "b0", "i0", "a1", "b1", "i1", "a2", "b2", "i2"}, order)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, err := tx.EventsAfter(port.SourceOf("ta"), 1, 0)
		if len(got) != 2 || got[0].Sequence != 2 || got[1].Sequence != 3 || got[0].Type != "a1" {
			e.t.Fatalf("events after 1 on the stream of ta: %+v", got)
		}
		other, _ := tx.EventsAfter(port.SourceOf("tb"), 0, 0)
		e.same(0, len(other))
		return err
	}))
	first, _ := e.o.Pending(ctx, 2)
	e.same(2, len(first))
	e.must(e.o.MarkDelivered(ctx, []string{first[0].ID, first[1].ID, "unknown"}))
	rest, _ := e.o.Pending(ctx, 0)
	e.same(7, len(rest))
	e.same("i0", rest[0].Type)
}

func concurrentEmit(e *env) {
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { emit(e, scopeA, "ta", "x") })
	}
	wg.Wait()
	ev, _ := e.o.Pending(ctx, 0)
	var seqs []int64
	for _, x := range ev {
		seqs = append(seqs, x.Sequence)
	}
	slices.Sort(seqs)
	for i, s := range seqs {
		if s != int64(i+1) {
			e.t.Fatalf("sequences of one stream are unique and gapless: %v", seqs)
		}
	}
}

func conditionalApproval(e *env) {
	a := pending("ta", "a1", "h")
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(a) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(a) }), port.CodeConflict)
	approved := a
	approved.State, approved.DecidedBy, approved.DecidedAt = port.ApprovalApproved, "u1", t0
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.UpdateApproval(approved, port.ApprovalPending) }))
	denied := a
	denied.State, denied.Arguments = port.ApprovalDenied, nil
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.UpdateApproval(denied, port.ApprovalPending) }), port.CodeConflict)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, err := tx.GetApproval("a1")
		e.same(port.ApprovalApproved, got.State)
		e.same("u1", got.DecidedBy)
		return err
	}))
}

func oneOpenRecord(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(pending("ta", "a1", "h")) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(pending("ta", "a2", "h")) }), port.CodeConflict)
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(pending("ta", "a3", "other")) }))
	e.must(e.write(scopeB, func(tx port.Tx) error { return tx.CreateApproval(pending("tb", "b1", "h")) }))
	done := pending("ta", "a1", "h")
	done.State, done.Arguments = port.ApprovalExpired, nil
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.UpdateApproval(done, port.ApprovalPending) }))
	again := pending("ta", "a4", "h")
	again.CreatedAt = t0.Add(time.Second)
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(again) }))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, err := tx.ListApprovals(port.ApprovalFilter{Session: "s1", CallHash: "h"})
		e.same(2, len(got))
		e.same([]port.ApprovalID{"a1", "a4"}, []port.ApprovalID{got[0].ID, got[1].ID})
		open, _ := tx.ListApprovals(port.ApprovalFilter{States: []port.ApprovalState{port.ApprovalPending}})
		e.same(2, len(open))
		later, _ := tx.ListApprovals(port.ApprovalFilter{Session: "s1", Since: t0.Add(time.Second)})
		e.same([]port.ApprovalID{"a4"}, []port.ApprovalID{later[0].ID})
		return err
	}))
}

func concurrentDecisions(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateApproval(pending("ta", "a1", "h")) }))
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			err := e.write(scopeA, func(tx port.Tx) error {
				a, err := tx.GetApproval("a1")
				if err != nil {
					return err
				}
				a.State, a.DecidedBy = port.ApprovalApproved, fmt.Sprint(i)
				return tx.UpdateApproval(a, port.ApprovalPending)
			})
			if err == nil {
				won.Add(1)
			} else if !port.IsCode(err, port.CodeConflict) {
				e.t.Errorf("a losing decision gets conflict: %v", err)
			}
		})
	}
	wg.Wait()
	e.same(int32(1), won.Load())
}

func conditionalCall(e *env) {
	c := port.Call{ID: "k1", Tenant: "ta", Session: "s1", CallHash: "h", State: port.CallStarted, StartedAt: t0, Deadline: t0.Add(time.Minute)}
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateCall(c) }))
	later := c
	later.ID, later.StartedAt, later.State = "k2", t0.Add(time.Second), port.CallFinished
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.CreateCall(later) }))
	finished := c
	finished.State = port.CallFinished
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.UpdateCall(finished, port.CallStarted) }))
	unknown := c
	unknown.State = port.CallOutcomeUnknown
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.UpdateCall(unknown, port.CallStarted) }), port.CodeConflict)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, ok, err := tx.LatestCall("s1", "h")
		if !ok || got.ID != "k2" {
			e.t.Fatalf("the repeat lookup reads the newest call of (session, call_hash): %+v", got)
		}
		_, ok, _ = tx.LatestCall("s2", "h")
		e.same(false, ok)
		return err
	}))
}

func revisions(e *env) {
	var c port.Connector
	e.must(e.write(scopeA, func(tx port.Tx) error {
		var err error
		c, err = tx.PutConnector(connector("ta", "c1"), port.AnyRevision)
		return err
	}))
	e.same(int64(1), c.Revision)
	e.must(e.write(scopeA, func(tx port.Tx) error {
		c.Enabled = false
		var err error
		c, err = tx.PutConnector(c, c.Revision)
		return err
	}))
	e.same(int64(2), c.Revision)
	stale := c
	stale.Revision = 1
	e.code(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.PutConnector(stale, 1)
		return err
	}), port.CodeRevisionMismatch)
	cred := port.Credential{ID: "k", Tenant: "ta", OwnerKind: port.OwnerAgent, OwnerID: "x", Connector: "c1", Kind: port.CredentialAPIKey, Status: port.CredentialActive, CreatedAt: t0}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		var err error
		cred, err = tx.PutCredential(cred, port.AnyRevision)
		return err
	}))
	e.code(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.PutCredential(cred, 0)
		return err
	}), port.CodeRevisionMismatch)
}

func grantKey(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "g1", "*", 0)) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "g2", "*", 0)) }), port.CodeConflict)
	e.must(e.write(scopeB, func(tx port.Tx) error { return tx.PutGrant(agentGrant("tb", "h1", "*", 0)) }))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		if err := tx.DeleteGrant("g1"); err != nil {
			return err
		}
		return tx.PutGrant(agentGrant("ta", "g2", "*", 0))
	}))
	ref := port.ListingRef{Name: "io.x/svc"}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		return tx.PutGrant(port.Grant{ID: "t1", Tenant: "ta", ScopeKind: port.ScopeTenant, ScopeID: "ta", Listing: &ref, Selector: "*", Mode: port.ModeDeny, Origin: port.OriginSettings})
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, err := tx.ListGrants(port.GrantFilter{Listing: &ref})
		e.same(1, len(got))
		got, _ = tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeAgent, Connector: "c1"})
		e.same(1, len(got))
		return err
	}))
}

func countedReservation(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error {
		if err := tx.PutGrant(agentGrant("ta", "g1", "fn:f", 1)); err != nil {
			return err
		}
		g := agentGrant("ta", "g2", "*", 5)
		g.ScopeKind = port.ScopeWorkspace
		if err := tx.PutGrant(g); err != nil {
			return err
		}
		return tx.PutGrant(agentGrant("ta", "g3", "class:read", 0))
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1", "g2", "g3"}) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g2", "g1"}) }), port.CodeGrantExhausted)
	e.must(e.read(scopeA, func(tx port.Tx) error {
		g1, _ := tx.GetGrant("g1")
		g2, _ := tx.GetGrant("g2")
		g3, err := tx.GetGrant("g3")
		e.same([]int{1, 1, 0}, []int{g1.Uses, g2.Uses, g3.Uses})
		return err
	}))
	e.code(e.write(scopeB, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g2"}) }), port.CodeNotFound)
}

func writingAGrantRowKeepsItsUseCount(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "g1", "fn:f", 3)) }))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		g := agentGrant("ta", "g1", "fn:f", 3)
		g.DefinitionHash = "h2"
		if err := tx.PutGrant(g); err != nil {
			return err
		}
		fresh := agentGrant("ta", "g2", "*", 3)
		fresh.Uses = 2
		return tx.PutGrant(fresh)
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		g1, _ := tx.GetGrant("g1")
		g2, err := tx.GetGrant("g2")
		e.same([]any{"h2", 2, 0}, []any{g1.DefinitionHash, g1.Uses, g2.Uses})
		return err
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }), port.CodeGrantExhausted)

	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "g3", "class:read", 50)) }))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			err := e.write(scopeA, func(tx port.Tx) error {
				if i%2 == 0 {
					return tx.ReserveGrants([]port.GrantID{"g3"})
				}
				return tx.PutGrant(agentGrant("ta", "g3", "class:read", 50))
			})
			if err != nil {
				e.t.Errorf("a write of g3 failed: %v", err)
			}
		})
	}
	wg.Wait()
	e.must(e.read(scopeA, func(tx port.Tx) error {
		g, err := tx.GetGrant("g3")
		e.same(10, g.Uses)
		return err
	}))
}

func concurrentReservation(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "g1", "fn:f", 3)) }))
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			err := e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) })
			switch {
			case err == nil:
				won.Add(1)
			case !port.IsCode(err, port.CodeGrantExhausted):
				e.t.Errorf("a losing reservation gets grant_exhausted: %v", err)
			}
		})
	}
	wg.Wait()
	e.same(int32(3), won.Load())
	e.must(e.read(scopeA, func(tx port.Tx) error {
		g, err := tx.GetGrant("g1")
		e.same(3, g.Uses)
		return err
	}))
}

func dueWork(e *env) {
	e.must(e.write(scopeA, func(tx port.Tx) error {
		if err := tx.CreateApproval(pending("ta", "a1", "h1")); err != nil {
			return err
		}
		late := pending("ta", "a2", "h2")
		late.ExpiresAt = t0.Add(time.Hour)
		if err := tx.CreateApproval(late); err != nil {
			return err
		}
		return tx.CreateCall(port.Call{ID: "k1", Tenant: "ta", Session: "s1", CallHash: "h", State: port.CallStarted, StartedAt: t0, Deadline: t0.Add(30 * time.Second)})
	}))
	e.must(e.write(scopeB, func(tx port.Tx) error {
		a := pending("tb", "b1", "h")
		a.State, a.DecidedAt = port.ApprovalApproved, t0
		return tx.CreateApproval(a)
	}))
	e.code(e.read(scopeA, func(tx port.Tx) error {
		_, err := tx.DueWork(t0, 0)
		return err
	}), port.CodePermissionDenied)
	e.must(e.read(inst, func(tx port.Tx) error {
		got, err := tx.DueWork(t0.Add(time.Minute), 0)
		var kinds []string
		for _, w := range got {
			kinds = append(kinds, string(w.Tenant)+":"+string(w.Kind)+":"+w.ID)
		}
		e.same([]string{"tb:approved_record:b1", "ta:call_deadline:k1", "ta:approval_expiry:a1"}, kinds)
		limited, _ := tx.DueWork(t0.Add(time.Minute), 1)
		e.same(1, len(limited))
		return err
	}))
}

func inbox(e *env) {
	var first, second bool
	e.must(e.write(scopeA, func(tx port.Tx) error {
		var err error
		first, err = tx.MarkInbox("m1", t0)
		return err
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		var err error
		second, err = tx.MarkInbox("m1", t0)
		return err
	}))
	e.same([]bool{true, false}, []bool{first, second})
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.SetInboxPayload("m1", []byte(`{"hash":"h"}`)) }))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		got, err := tx.InboxPayload("m1")
		e.same(`{"hash":"h"}`, string(got))
		return err
	}))
	e.must(e.read(scopeB, func(tx port.Tx) error {
		seen, err := tx.SeenInbox("m1")
		e.same(false, seen)
		got, err2 := tx.InboxPayload("m1")
		e.same(0, len(got))
		return errors.Join(err, err2)
	}))
}

func scopeValidation(e *env) {
	e.code(e.read(port.Scope{}, func(port.Tx) error { return nil }), port.CodeInvalidArgument)
	e.code(e.write(port.Scope{}, func(port.Tx) error { return nil }), port.CodeInvalidArgument)
}

func skillVersion(e *env) {
	ref := port.VersionRef{Listing: port.ListingRef{Owner: "ta", Name: "io.x/pdf"}, Version: strings.Repeat("a", 40)}
	content := []byte{'#', '!', 0x00, 0xff, '\n'}
	m := port.Manifest{Owner: "ta", Name: "io.x/pdf", Version: ref.Version, Kind: port.KindSkill, Skill: &port.Skill{Name: "pdf", Redistribution: port.RedistributionBundled,
		Files: []port.SkillFile{{Path: "scripts/run.sh", Bytes: int64(len(content)), SHA256: "s", Content: content}}}}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		return tx.PutVersion(port.ListingVersion{Ref: ref, Manifest: m, Status: port.ListingActive, ImportedAt: t0})
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		lv, err := tx.GetVersion(ref)
		if err != nil {
			return err
		}
		if lv.Manifest.Kind != port.KindSkill || lv.Manifest.Skill == nil || !bytes.Equal(lv.Manifest.Skill.Files[0].Content, content) {
			e.t.Fatalf("a skill version did not round trip: %+v", lv.Manifest.Skill)
		}
		return nil
	}))
}

func records(e *env) {
	ref := port.ListingRef{Name: "io.x/svc"}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		steps := []error{
			tx.PutFunction(port.Function{Tenant: "ta", Connector: "c1", Name: "f", DefinitionHash: "h", State: port.FunctionActive}),
			tx.PutBinding(port.Binding{Tenant: "ta", Workspace: "w", Connector: "c1", Agent: "x", AgentOwner: "u1", CredentialPolicy: port.CredentialPerUser}),
			tx.PutPolicy(port.CatalogPolicy{Tenant: "ta", Workspace: "w", Default: port.PolicyHidden}),
			tx.PutPause(port.Pause{ID: "p1", Tenant: "ta", Session: "s1", Agent: "x", Connector: "c1", Paused: true}),
			tx.PutTombstone(port.Tombstone{Tenant: "ta", Kind: port.TombstoneSession, Subject: "s1", At: t0, ExpiresAt: t0.Add(time.Hour)}),
			tx.PutOverlay(port.Overlay{Tenant: "ta", Connector: "c1", Function: "f", Version: 2, State: port.OverlayActive}),
			tx.PutOverlay(port.Overlay{Tenant: "ta", Connector: "c1", Function: "f", Version: 1, State: port.OverlaySuperseded}),
			tx.PutOverlay(port.Overlay{Tenant: "ta", Listing: &ref, Function: "f", Version: 1, State: port.OverlayActive}),
			tx.PutExposure(port.Exposure{Tenant: "ta", Agent: "x", Exposed: true, LegC: []string{"c1/f"}}),
			tx.PutOperation(port.Operation{ID: "o1", Tenant: "ta", Kind: "installConnector", CreatedAt: t0}),
		}
		return errors.Join(steps...)
	}))
	e.must(e.write(inst, func(tx port.Tx) error {
		return errors.Join(tx.PutFanOut(port.FanOut{ID: "f1", Kind: "status", Listing: &ref, CreatedAt: t0}),
			tx.PutOperation(port.Operation{ID: "o2", Kind: "removeTenant", CreatedAt: t0}))
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		f, err1 := tx.GetFunction("c1", "f")
		b, err2 := tx.GetBinding("c1", "x")
		bs, _ := tx.ListBindings(port.BindingFilter{Owner: "u1"})
		p, ok, err3 := tx.GetPolicy("w")
		pause, paused, err4 := tx.GetPause("s1", "x", "c1")
		ts, _ := tx.ListTombstones(port.TombstoneSession)
		ov, _ := tx.ListOverlays("c1", nil, "f")
		lov, _ := tx.ListOverlays("", &ref, "f")
		x, exposed, err5 := tx.GetExposure("x")
		_, err6 := tx.GetOperation("o2")
		_, err7 := tx.ListFanOuts()
		e.code(err6, port.CodeNotFound)
		fos, _ := tx.ListFanOuts()
		e.same(0, len(fos))
		e.same([]any{"h", "u1", 1, true, port.PolicyHidden, true, true, 1, 2, 1, 1, true, true},
			[]any{f.DefinitionHash, b.AgentOwner, len(bs), ok, p.Default, paused, pause.Paused, len(ts), len(ov), ov[0].Version, len(lov), exposed, x.Exposed})
		return errors.Join(err1, err2, err3, err4, err5, err7)
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		return errors.Join(tx.DeletePause("p1"), tx.DeleteTombstone(port.TombstoneSession, "s1", ""), tx.DeleteBinding("c1", "x"))
	}))
	e.must(e.read(scopeA, func(tx port.Tx) error {
		_, paused, err := tx.GetPause("s1", "x", "c1")
		ts, _ := tx.ListTombstones(port.TombstoneSession)
		e.same([]any{false, 0}, []any{paused, len(ts)})
		return err
	}))
}

func rolesAndGroups(e *env) {
	opRole := port.RoleAssignment{Issuer: "corp", Person: "op", Role: "operator", Scope: "installation"}
	adminA := port.RoleAssignment{Tenant: "ta", Person: "ana", Role: "tenant_admin", Scope: "t/ta"}
	adminB := port.RoleAssignment{Tenant: "tb", Person: "ana", Role: "tenant_admin", Scope: "t/tb"}
	member := port.GroupMember{Tenant: "ta", Person: "ana", Group: "ops"}
	e.must(e.write(inst, func(tx port.Tx) error { return errors.Join(tx.PutRole(opRole), tx.PutRole(opRole)) }))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		return errors.Join(tx.PutRole(adminA), tx.PutMember(member), tx.PutMember(member))
	}))
	e.must(e.write(scopeB, func(tx port.Tx) error { return tx.PutRole(adminB) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.PutRole(opRole) }), port.CodePermissionDenied)
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.PutRole(adminB) }), port.CodePermissionDenied)
	e.must(e.read(port.Scope{Tenant: "ta", Installation: true}, func(tx port.Tx) error {
		roles, err1 := tx.ListRoles(port.RoleFilter{})
		ana, err2 := tx.ListRoles(port.RoleFilter{Person: "ana"})
		members, err3 := tx.ListMembers(port.MemberFilter{Group: "ops"})
		e.same([]any{[]port.RoleAssignment{opRole, adminA}, []port.RoleAssignment{adminA}, []port.GroupMember{member}}, []any{roles, ana, members})
		return errors.Join(err1, err2, err3)
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error { return errors.Join(tx.DeleteRole(adminA), tx.DeleteMember(member)) }))
	e.code(e.write(scopeA, func(tx port.Tx) error { return tx.DeleteRole(adminA) }), port.CodeNotFound)
	e.must(e.read(scopeB, func(tx port.Tx) error {
		roles, err := tx.ListRoles(port.RoleFilter{})
		e.same([]port.RoleAssignment{adminB}, roles)
		return err
	}))
}

func callsClientsAndSyncs(e *env) {
	call := func(tenant port.TenantID, id, agent string, at time.Duration, state port.CallState) port.Call {
		return port.Call{ID: port.CallID(id), Tenant: tenant, Agent: port.AgentID(agent), Session: "s1", Connector: "c1", State: state, StartedAt: t0.Add(at)}
	}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		return errors.Join(tx.CreateCall(call("ta", "k2", "x", 2*time.Second, port.CallFinished)), tx.CreateCall(call("ta", "k1", "x", time.Second, port.CallStarted)),
			tx.CreateCall(call("ta", "k3", "y", 3*time.Second, port.CallFinished)),
			tx.PutClient(port.ClientRegistration{Tenant: "ta", Issuer: "https://idp", ClientID: "a", Secret: &port.Sealed{Ciphertext: []byte("z"), KeyRef: "k"}}))
	}))
	e.must(e.write(scopeB, func(tx port.Tx) error {
		return errors.Join(tx.CreateCall(call("tb", "k9", "x", 0, port.CallFinished)), tx.PutClient(port.ClientRegistration{Tenant: "tb", Issuer: "https://idp", ClientID: "b"}))
	}))
	e.must(e.write(inst, func(tx port.Tx) error {
		return errors.Join(tx.PutSourceSync(port.SourceSync{Source: "z", LastSync: t0, Imported: 1}), tx.PutSourceSync(port.SourceSync{Source: "a", LastSync: t0, Skipped: 2}),
			tx.PutSourceSync(port.SourceSync{Source: "z", LastSync: t0.Add(time.Hour), Imported: 3}))
	}))
	ids := func(cs []port.Call) []string {
		var out []string
		for _, c := range cs {
			out = append(out, string(c.ID))
		}
		return out
	}
	e.must(e.read(scopeA, func(tx port.Tx) error {
		all, err1 := tx.ListCalls(port.CallFilter{})
		byAgent, err2 := tx.ListCalls(port.CallFilter{Agent: "x"})
		byState, err3 := tx.ListCalls(port.CallFilter{State: port.CallFinished})
		since, err6 := tx.ListCalls(port.CallFilter{Since: t0.Add(2 * time.Second)})
		a, okA, err4 := tx.GetClient("https://idp")
		_, okMissing, err5 := tx.GetClient("https://other")
		e.same([]any{[]string{"k1", "k2", "k3"}, []string{"k1", "k2"}, []string{"k2", "k3"}, []string{"k2", "k3"}, "a", true, false, "z"},
			[]any{ids(all), ids(byAgent), ids(byState), ids(since), a.ClientID, okA, okMissing, string(a.Secret.Ciphertext)})
		return errors.Join(err1, err2, err3, err4, err5, err6)
	}))
	e.must(e.read(scopeB, func(tx port.Tx) error {
		all, err1 := tx.ListCalls(port.CallFilter{})
		b, _, err2 := tx.GetClient("https://idp")
		e.same([]any{[]string{"k9"}, "b"}, []any{ids(all), b.ClientID})
		return errors.Join(err1, err2)
	}))
	e.must(e.read(inst, func(tx port.Tx) error {
		syncs, err := tx.ListSourceSyncs()
		e.same([]any{2, "a", "z", 3}, []any{len(syncs), syncs[0].Source, syncs[1].Source, syncs[1].Imported})
		return err
	}))
}

func purgeTenant(e *env) {
	purger, ok := e.s.(port.TenantPurger)
	if !ok {
		e.t.Fatal("the store does not purge tenants")
	}
	fill := func(scope port.Scope) {
		tenant := scope.Tenant
		e.must(e.write(inst, func(tx port.Tx) error {
			return errors.Join(tx.PutTenant(port.Tenant{ID: tenant}), tx.PutOffer(port.Offer{Tenant: tenant, TargetKind: "listing", Target: "x"}))
		}))
		e.must(e.write(scope, func(tx port.Tx) error {
			_, err := tx.PutConnector(connector(tenant, "c-"+string(tenant)), port.AnyRevision)
			_, err2 := tx.MarkInbox("m", t0)
			return errors.Join(err, err2, tx.Audit(port.AuditRow{Tenant: tenant, Actor: "a", Action: "x", Subject: "s"}),
				tx.PutRole(port.RoleAssignment{Tenant: tenant, Person: "p", Role: "approver", Scope: "t/" + string(tenant)}),
				tx.PutListing(port.Listing{Ref: port.ListingRef{Owner: tenant, Name: "io.x/own"}}))
		}))
		e.appendChanges(scope, change("g", `{}`))
	}
	fill(scopeA)
	fill(scopeB)
	counts, err := purger.PurgeTenant(ctx, "ta")
	e.must(err)
	e.same([]int{1, 1, 1, 1, 1, 1, 1}, []int{counts["connectors"], counts["roles"], counts["listings"], counts["offers"], counts["audit"], counts["permission_changes"], counts["permission_head"]})
	e.same([]string{"tbg"}, append(keysOf(e.changes(scopeA, 0, 0, 0)), keysOf(e.changes(scopeB, 0, 0, 0))...))
	e.same(int64(0), e.lockedRevision(scopeA))
	for scope, want := range map[port.Scope]int{scopeA: 0, scopeB: 1} {
		e.must(e.read(port.Scope{Tenant: scope.Tenant, Installation: true}, func(tx port.Tx) error {
			conns, err1 := tx.ListConnectors(port.ConnectorFilter{})
			roles, err2 := tx.ListRoles(port.RoleFilter{Person: "p"})
			audit, err3 := tx.ListAudit("s")
			seen, err4 := tx.SeenInbox("m")
			_, err5 := tx.GetTenant(scope.Tenant)
			e.same([]any{want, want, want, want == 1}, []any{len(conns), len(roles), len(audit), seen})
			return errors.Join(err1, err2, err3, err4, err5)
		}))
	}
	_, err = purger.PurgeTenant(ctx, port.Installation)
	e.code(err, port.CodeInvalidArgument)
}

func pruneHistory(e *env) {
	pruner, ok := e.s.(port.Pruner)
	if !ok {
		e.t.Fatal("the store does not prune history")
	}
	fill := func(scope port.Scope) {
		tenant := scope.Tenant
		e.must(e.write(scope, func(tx port.Tx) error {
			_, err1 := tx.MarkInbox("idem:old", t0)
			_, err2 := tx.Emit(port.Event{Type: "delivered", Tenant: tenant, Time: t0})
			finished := port.Call{ID: port.CallID("done-" + string(tenant)), Tenant: tenant, State: port.CallFinished, StartedAt: t0, FinishedAt: t0}
			running := port.Call{ID: port.CallID("run-" + string(tenant)), Tenant: tenant, State: port.CallStarted, StartedAt: t0}
			return errors.Join(err1, err2, tx.Audit(port.AuditRow{At: t0, Tenant: tenant, Actor: "a", Action: "x", Subject: "s"}), tx.CreateCall(finished), tx.CreateCall(running))
		}))
	}
	fill(scopeA)
	fill(scopeB)
	pending, err := e.o.Pending(ctx, 0)
	e.must(err)
	ids := make([]string, len(pending))
	for i, p := range pending {
		ids[i] = p.ID
	}
	e.must(e.o.MarkDelivered(ctx, ids))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "pending", Tenant: "ta", Time: t0})
		return err
	}))
	later := time.Now().Add(30 * 24 * time.Hour)
	none, err := pruner.Prune(ctx, "ta", later, port.Retention{})
	e.must(err)
	e.same(0, none["events"]+none["inbox"]+none["audit"]+none["calls"])
	counts, err := pruner.Prune(ctx, "ta", later, port.Retention{Delivered: 7 * 24 * time.Hour, Inbox: 7 * 24 * time.Hour, Audit: time.Hour, Calls: time.Hour})
	e.must(err)
	e.same([]int{1, 1, 1, 1}, []int{counts["events"], counts["inbox"], counts["audit"], counts["calls"]})
	for scope, want := range map[port.Scope]int{scopeA: 0, scopeB: 1} {
		e.must(e.read(scope, func(tx port.Tx) error {
			audit, err1 := tx.ListAudit("s")
			seen, err2 := tx.SeenInbox("idem:old")
			calls, err3 := tx.ListCalls(port.CallFilter{})
			e.same([]any{want, want == 1, want + 1}, []any{len(audit), seen, len(calls)})
			return errors.Join(err1, err2, err3)
		}))
	}
	left, err := e.o.Pending(ctx, 0)
	e.must(err)
	e.same([]any{1, "pending"}, []any{len(left), left[0].Type})
}

type storeView struct {
	Connectors map[port.TenantID][]port.Connector
	Grants     []port.Grant
	Approvals  []port.Approval
	Audit      []port.AuditRow
	Events     []port.Event
	Pending    int
	Due        int
	Seen       bool
	Tenants    []port.Tenant
	Listings   []port.Listing
}

func viewOf(e *env, s port.Store, o port.Outbox) storeView {
	e.t.Helper()
	v := storeView{Connectors: map[port.TenantID][]port.Connector{}}
	for _, scope := range []port.Scope{scopeA, scopeB} {
		e.must(s.Read(ctx, scope, func(tx port.Tx) error {
			cs, err := tx.ListConnectors(port.ConnectorFilter{})
			v.Connectors[scope.Tenant] = cs
			return err
		}))
	}
	e.must(s.Read(ctx, scopeA, func(tx port.Tx) (err error) {
		if v.Grants, err = tx.ListGrants(port.GrantFilter{}); err != nil {
			return err
		}
		if v.Approvals, err = tx.ListApprovals(port.ApprovalFilter{}); err != nil {
			return err
		}
		if v.Audit, err = tx.ListAudit("c1"); err != nil {
			return err
		}
		if v.Events, err = tx.EventsAfter(port.SourceOf("ta"), 0, 0); err != nil {
			return err
		}
		v.Seen, err = tx.SeenInbox("m1")
		return err
	}))
	e.must(s.Read(ctx, inst, func(tx port.Tx) (err error) {
		if v.Tenants, err = tx.ListTenants(); err == nil {
			v.Listings, err = tx.ListListings()
		}
		due, err2 := tx.DueWork(t0.Add(24*time.Hour), 0)
		v.Due = len(due)
		return errors.Join(err, err2)
	}))
	pending, err := o.Pending(ctx, 0)
	e.must(err)
	v.Pending = len(pending)
	return v
}

func snapshotRestore(e *env) {
	src, ok := e.s.(port.Snapshotter)
	if !ok {
		e.t.Skip("the store takes no snapshot")
	}
	e.must(e.write(inst, func(tx port.Tx) error {
		return errors.Join(tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantActive, CreatedAt: t0}), tx.PutTenant(port.Tenant{ID: "tb", State: port.TenantActive, CreatedAt: t0}),
			tx.PutListing(port.Listing{Ref: port.ListingRef{Name: "io.x/svc"}, Source: "own", Publisher: "x", Status: port.ListingActive}))
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		g := agentGrant("ta", "g1", "*", 3)
		g.ExpiresAt = t0.Add(time.Hour)
		_, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision)
		_, err2 := tx.MarkInbox("m1", t0)
		_, err3 := tx.Emit(port.Event{Type: "connector.connector.installed", Tenant: "ta", Subject: "c1", Time: t0})
		_, err4 := tx.Emit(port.Event{Type: "connector.connector.updated", Tenant: "ta", Subject: "c1", Time: t0})
		return errors.Join(err, err2, err3, err4, tx.PutGrant(g), tx.CreateApproval(pending("ta", "a1", "h1")),
			tx.Audit(port.AuditRow{Tenant: "ta", Actor: "a", Action: "install", Subject: "c1", At: t0}))
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) }))
	e.must(e.write(scopeB, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("tb", "c2"), port.AnyRevision)
		return err
	}))
	first, err := e.o.Pending(ctx, 1)
	e.must(err)
	e.must(e.o.MarkDelivered(ctx, []string{first[0].ID}))
	want := viewOf(e, e.s, e.o)
	if want.Due == 0 || want.Pending != 1 || len(want.Grants) != 1 || want.Grants[0].Uses != 1 || len(want.Audit) != 1 || !want.Seen || len(want.Events) != 2 {
		e.t.Fatalf("the seed is not what the case checks: %+v", want)
	}
	snap, err := src.Snapshot(ctx)
	e.must(err)

	s, o := e.fresh()
	dst := s.(port.Snapshotter)
	restored := want
	restored.Pending++
	for range 2 {
		_, err := dst.Restore(ctx, snap, "operator")
		e.must(err)
		e.same(restored, viewOf(e, s, o))
	}
	e.must(s.Write(ctx, scopeA, func(tx port.Tx) error {
		ev, err := tx.Emit(port.Event{Type: "connector.connector.updated", Tenant: "ta", Subject: "c1", Time: t0})
		e.same(int64(3), ev.Sequence)
		return errors.Join(err, tx.Audit(port.AuditRow{Tenant: "ta", Actor: "a", Action: "again", Subject: "c1", At: t0}))
	}))

	e.must(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("ta", "c9"), port.AnyRevision)
		return err
	}))
	broken := port.Snapshot{Store: snap.Store, Tables: maps.Clone(snap.Tables)}
	for name, rows := range broken.Tables {
		broken.Tables[name] = append(slices.Clone(rows), json.RawMessage(`"not a row"`))
	}
	before := viewOf(e, e.s, e.o)
	if _, err := src.Restore(ctx, broken, "operator"); err == nil {
		e.t.Fatal("a broken snapshot was restored")
	}
	e.same(before, viewOf(e, e.s, e.o))
	if _, err := src.Restore(ctx, port.Snapshot{Store: "elsewhere"}, "operator"); !port.IsCode(err, port.CodeInvalidArgument) {
		e.t.Fatalf("a snapshot of another store: %v", err)
	}
	_, err = src.Restore(ctx, snap, "operator")
	e.must(err)
	e.same(restored, viewOf(e, e.s, e.o))
}

// LegacyGrants writes, in tenant ta, rows an older release could store: a user row naming an agent and three pairs of agent rows whose keys differ only by agent.
func LegacyGrants(t *testing.T, s port.Store) {
	t.Helper()
	err := s.Write(ctx, inst, func(tx port.Tx) error {
		return tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantActive, CreatedAt: t0})
	})
	if err == nil {
		err = s.Write(ctx, scopeA, func(tx port.Tx) error {
			user := agentGrant("ta", "u1", "*", 0)
			user.ScopeKind, user.ScopeID, user.Agent, user.Mode = port.ScopeUser, "alice", "x", port.ModeDeny
			named := agentGrant("ta", "a1", "class:read", 0)
			named.Agent, named.ExpiresAt = "x", t0.Add(time.Hour)
			bare := agentGrant("ta", "a2", "class:read", 0)
			bare.Mode = port.ModeDeny
			survivor := agentGrant("ta", "b1", "class:write", 0)
			survivor.Agent, survivor.Mode = "x", port.ModeDeny
			dropped := agentGrant("ta", "b2", "class:write", 0)
			dropped.ExpiresAt = t0.Add(time.Hour)
			counted := agentGrant("ta", "c1", "class:destructive", 1)
			counted.Agent = "x"
			twin := agentGrant("ta", "c2", "class:destructive", 1)
			return errors.Join(tx.PutGrant(user), tx.PutGrant(named), tx.PutGrant(bare), tx.PutGrant(survivor), tx.PutGrant(dropped), tx.PutGrant(counted), tx.PutGrant(twin))
		})
	}
	if err != nil {
		t.Fatal(err)
	}
}

// CheckCanonicalGrants fails unless tenant ta holds the canonical form of LegacyGrants: u1 and b1 without their agent, a2 alone, c1 as the deny of a conflict, audited.
func CheckCanonicalGrants(t *testing.T, s port.Store) {
	t.Helper()
	var got []port.Grant
	if err := s.Read(ctx, scopeA, func(tx port.Tx) (err error) {
		got, err = tx.ListGrants(port.GrantFilter{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b port.Grant) int { return strings.Compare(string(a.ID), string(b.ID)) })
	if len(got) != 4 {
		t.Fatalf("grants %+v", got)
	}
	for i, id := range []port.GrantID{"a2", "b1", "c1", "u1"} {
		if g := got[i]; g.ID != id || g.Agent != "" || g.Mode != port.ModeDeny || g.MaxUses != 0 || !g.ExpiresAt.IsZero() || (id == "c1") != (g.Reason == "rows_conflicted") {
			t.Fatalf("grants %+v", got)
		}
	}
	for subject, action := range map[string]string{"a2": "grant.merged", "b1": "grant.merged", "c1": "grant.merged", "grants": "grant.agent_cleared"} {
		var audit []port.AuditRow
		if err := s.Read(ctx, scopeA, func(tx port.Tx) (err error) {
			audit, err = tx.ListAudit(subject)
			return err
		}); err != nil || len(audit) != 1 || audit[0].Action != action {
			t.Fatalf("audit of %s: %+v %v", subject, audit, err)
		}
		if rows, _ := audit[0].Detail["rows"].([]any); action == "grant.merged" && len(rows) != 2 {
			t.Fatalf("audit of %s names %d original rows", subject, len(rows))
		}
	}
	if err := s.Write(ctx, scopeA, func(tx port.Tx) error { return tx.PutGrant(agentGrant("ta", "a3", "class:read", 0)) }); !port.IsCode(err, port.CodeConflict) {
		t.Fatalf("a row with the canonical key of a2: %v", err)
	}
}

func restoreNormalizesAgent(e *env) {
	src, ok := e.s.(port.Snapshotter)
	if !ok {
		e.t.Skip("the store takes no snapshot")
	}
	LegacyGrants(e.t, e.s)
	snap, err := src.Snapshot(ctx)
	e.must(err)
	s, _ := e.fresh()
	_, err = s.(port.Snapshotter).Restore(ctx, snap, "operator")
	e.must(err)
	CheckCanonicalGrants(e.t, s)
}
