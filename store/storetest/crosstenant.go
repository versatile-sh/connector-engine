package storetest

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func crossTenantMatrix(e *env) {
	snapper, ok := e.s.(port.Snapshotter)
	if !ok {
		e.t.Skip("the store takes no snapshot")
	}
	private := port.ListingRef{Owner: "ta", Name: "io.x/crm"}
	call := port.Call{ID: "k1", Tenant: "ta", Agent: "x", Session: "s1", Connector: "c1", CallHash: "h", State: port.CallStarted, StartedAt: t0, Deadline: t0.Add(time.Hour)}
	cred := port.Credential{ID: "cr1", Tenant: "ta", OwnerKind: port.OwnerUser, OwnerID: "u1", Connector: "c1", Kind: port.CredentialAPIKey, Secret: port.Sealed{Ciphertext: []byte("z"), KeyRef: "k"}}
	role := port.RoleAssignment{Tenant: "ta", Person: "ana", Role: "tenant_admin", Scope: "t/ta"}
	member := port.GroupMember{Tenant: "ta", Person: "ana", Group: "ops"}
	e.must(e.write(scopeA, func(tx port.Tx) error {
		_, err1 := tx.PutConnector(connector("ta", "c1"), port.AnyRevision)
		_, err2 := tx.PutCredential(cred, port.AnyRevision)
		_, err3 := tx.Emit(port.Event{Type: "x", Tenant: "ta", Time: t0})
		_, err5 := tx.Emit(port.Event{Type: "y", Tenant: "ta", Time: t0})
		_, err4 := tx.MarkInbox("m1", t0)
		return errors.Join(err1, err2, err3, err4, err5,
			tx.PutListing(port.Listing{Ref: private, Source: port.PrivateSource, Publisher: "ta", Status: port.ListingActive}),
			tx.PutFunction(port.Function{Tenant: "ta", Connector: "c1", Name: "f", DefinitionHash: "h", State: port.FunctionActive}),
			tx.PutBinding(port.Binding{Tenant: "ta", Workspace: "w", Connector: "c1", Agent: "x", AgentOwner: "u1", CredentialPolicy: port.CredentialPerUser}),
			tx.PutGrant(agentGrant("ta", "g1", "*", 3)),
			tx.CreateApproval(pending("ta", "a1", "h")),
			tx.PutPause(port.Pause{ID: "p1", Tenant: "ta", Session: "s1", Agent: "x", Connector: "c1", Paused: true, ExpiresAt: t0.Add(time.Hour)}),
			tx.PutTombstone(port.Tombstone{Tenant: "ta", Kind: port.TombstoneSession, Subject: "s1", At: t0, ExpiresAt: t0.Add(time.Hour)}),
			tx.PutOverlay(port.Overlay{Tenant: "ta", Connector: "c1", Function: "f", Version: 1, State: port.OverlayActive}),
			tx.CreateCall(call),
			tx.CreateCall(port.Call{ID: "k0", Tenant: "ta", Agent: "x", Session: "s0", Connector: "c1", State: port.CallFinished, StartedAt: t0, FinishedAt: t0}),
			tx.PutClient(port.ClientRegistration{Tenant: "ta", Issuer: "https://idp", ClientID: "a"}),
			tx.PutExposure(port.Exposure{Tenant: "ta", Agent: "x", Exposed: true}),
			tx.PutOperation(port.Operation{ID: "o1", Tenant: "ta", Kind: "installConnector", CreatedAt: t0}),
			tx.PutPolicy(port.CatalogPolicy{Tenant: "ta", Workspace: "w", Default: port.PolicyHidden}),
			tx.PutRule(port.CatalogRule{ID: "r1", Tenant: "ta", SubjectKind: port.SubjectTenant, SubjectID: "ta", TargetKind: port.TargetListing, Target: "/io.x/svc", Show: true, Ceiling: port.CeilingRead}),
			tx.PutRole(role), tx.PutMember(member),
			tx.Audit(port.AuditRow{At: t0, Tenant: "ta", Actor: "u1", Action: "x", Subject: "s1"}))
	}))
	e.appendChanges(scopeA, change("ta-g", `{"mode":"allow"}`))
	e.appendChanges(inst, change("inst-g", `{"mode":"allow"}`))
	e.must(e.write(inst, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "inst-event", Time: t0})
		return errors.Join(err, tx.PutOperation(port.Operation{ID: "inst-o2", Kind: "removeTenant", CreatedAt: t0}),
			tx.PutFanOut(port.FanOut{ID: "inst-f1", Kind: "status", CreatedAt: t0}),
			tx.Audit(port.AuditRow{At: t0, Actor: "inst-actor", Action: "x", Subject: "s1"}))
	}))
	events, err := e.o.Pending(ctx, 0)
	e.must(err)
	e.must(e.o.MarkDelivered(ctx, []string{events[0].ID}))
	before, due := e.everything(snapper)
	var seen []any
	e.must(e.read(scopeB, func(tx port.Tx) error {
		look := func(v any, _ ...any) { seen = append(seen, v) }
		look(tx.GetTenant("ta"))
		look(tx.ListTenants())
		look(tx.GetListing(private))
		look(tx.ListListings())
		look(tx.ListVersions(private))
		look(tx.ListOffers("ta"))
		look(tx.ListRules())
		look(tx.GetConnector("c1"))
		look(tx.ListConnectors(port.ConnectorFilter{}))
		look(tx.GetFunction("c1", "f"))
		look(tx.ListFunctions("c1"))
		look(tx.GetBinding("c1", "x"))
		look(tx.ListBindings(port.BindingFilter{}))
		look(tx.GetGrant("g1"))
		look(tx.ListGrants(port.GrantFilter{}))
		look(tx.GetApproval("a1"))
		look(tx.ListApprovals(port.ApprovalFilter{}))
		look(tx.ListPauses())
		look(tx.ListTombstones(port.TombstoneSession))
		look(tx.GetCredential("cr1"))
		look(tx.ListCredentials(port.CredentialFilter{}))
		look(tx.ListOverlays("c1", nil, "f"))
		look(tx.GetCall("k1"))
		look(tx.LatestCall("s1", "h"))
		look(tx.ListCalls(port.CallFilter{}))
		look(tx.GetOperation("o1"))
		look(tx.ListRoles(port.RoleFilter{}))
		look(tx.ListMembers(port.MemberFilter{}))
		look(tx.ListAudit(""))
		look(tx.EventsAfter(port.SourceOf("ta"), 0, 0))
		if seen, err := tx.SeenInbox("m1"); seen || err != nil {
			e.t.Fatalf("tenant tb sees the inbox of ta: %v %v", seen, err)
		}
		look(tx.GetOperation("inst-o2"))
		look(tx.ListFanOuts())
		look(tx.EventsAfter(port.SourceOf(port.Installation), 0, 0))
		look(tx.DueWork(t0.Add(48*time.Hour), 0))
		look(tx.ListPermissionChanges(0, 0, 0))
		return nil
	}))
	raw, err := json.Marshal(seen)
	e.must(err)
	if bytes.Contains(raw, []byte(`"ta"`)) || bytes.Contains(raw, []byte("inst-")) {
		e.t.Fatalf("tenant tb reads rows of tenant ta or of the installation: %s", raw)
	}
	for _, attack := range []func(port.Tx) error{
		func(tx port.Tx) error { _, err := tx.PutConnector(connector("tb", "c1"), port.AnyRevision); return err },
		func(tx port.Tx) error { _, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision); return err },
		func(tx port.Tx) error {
			_, err := tx.PutCredential(port.Credential{ID: "cr1", Tenant: "tb"}, port.AnyRevision)
			return err
		},
		func(tx port.Tx) error { return tx.DeleteCredential("cr1") },
		func(tx port.Tx) error {
			return tx.PutFunction(port.Function{Tenant: "tb", Connector: "c1", Name: "f", State: port.FunctionActive})
		},
		func(tx port.Tx) error { return tx.PutBinding(port.Binding{Tenant: "tb", Connector: "c1", Agent: "x"}) },
		func(tx port.Tx) error { return tx.DeleteBinding("c1", "x") },
		func(tx port.Tx) error { return tx.PutGrant(agentGrant("tb", "g1", "*", 0)) },
		func(tx port.Tx) error { return tx.DeleteGrant("g1") },
		func(tx port.Tx) error { return tx.ReserveGrants([]port.GrantID{"g1"}) },
		func(tx port.Tx) error { return tx.CreateApproval(pending("tb", "a1", "h")) },
		func(tx port.Tx) error { return tx.UpdateApproval(pending("ta", "a1", "h"), port.ApprovalPending) },
		func(tx port.Tx) error { return tx.UpdateApproval(pending("tb", "a1", "h"), port.ApprovalPending) },
		func(tx port.Tx) error { return tx.DeletePause("p1") },
		func(tx port.Tx) error {
			return tx.PutTombstone(port.Tombstone{Tenant: "ta", Kind: port.TombstoneSession, Subject: "s1"})
		},
		func(tx port.Tx) error {
			return tx.PutOverlay(port.Overlay{Tenant: "ta", Connector: "c1", Function: "f", Version: 1})
		},
		func(tx port.Tx) error { c := call; c.Tenant = "tb"; return tx.CreateCall(c) },
		func(tx port.Tx) error { return tx.UpdateCall(call, port.CallStarted) },
		func(tx port.Tx) error { c := call; c.Tenant = "tb"; return tx.UpdateCall(c, port.CallStarted) },
		func(tx port.Tx) error {
			return tx.PutClient(port.ClientRegistration{Tenant: "ta", Issuer: "https://idp"})
		},
		func(tx port.Tx) error { return tx.PutExposure(port.Exposure{Tenant: "ta", Agent: "x"}) },
		func(tx port.Tx) error { return tx.PutOperation(port.Operation{ID: "o1", Tenant: "tb", Kind: "x"}) },
		func(tx port.Tx) error { return tx.PutOperation(port.Operation{ID: "inst-o2", Tenant: "tb", Kind: "x"}) },
		func(tx port.Tx) error { return tx.PutFanOut(port.FanOut{ID: "inst-f1", Kind: "x"}) },
		func(tx port.Tx) error { _, err := tx.Emit(port.Event{Type: "x", Time: t0}); return err },
		func(tx port.Tx) error { return tx.PutPolicy(port.CatalogPolicy{Tenant: "ta", Workspace: "w"}) },
		func(tx port.Tx) error { return tx.PutRule(port.CatalogRule{ID: "r1", Tenant: "tb"}) },
		func(tx port.Tx) error { return tx.DeleteRule("r1") },
		func(tx port.Tx) error { return tx.DeleteRole(role) },
		func(tx port.Tx) error { return tx.DeleteMember(member) },
		func(tx port.Tx) error { return tx.PutRole(role) },
		func(tx port.Tx) error { return tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantRemoved}) },
		func(tx port.Tx) error {
			return tx.PutOffer(port.Offer{Tenant: "ta", TargetKind: port.TargetSource, Target: "own"})
		},
		func(tx port.Tx) error { return tx.PutListing(port.Listing{Ref: private, Status: port.ListingActive}) },
		func(tx port.Tx) error { _, err := tx.Emit(port.Event{Type: "x", Tenant: "ta", Time: t0}); return err },
		func(tx port.Tx) error { return tx.Audit(port.AuditRow{At: t0, Tenant: "ta", Action: "x"}) },
		func(tx port.Tx) error { _, err := tx.LockPermissions(); return err },
		func(tx port.Tx) error {
			_, err := tx.LockPermissions()
			_, err2 := tx.AppendPermissionChanges([]port.PermissionChange{{Tenant: "ta", Kind: "grant", Key: "ta-x", After: json.RawMessage(`1`)}})
			return errors.Join(err, err2)
		},
	} {
		_ = e.write(scopeB, attack)
	}
	if p, ok := e.s.(port.Pruner); ok {
		_, err := p.Prune(ctx, "tb", t0.Add(365*24*time.Hour), port.Retention{Delivered: time.Nanosecond, Inbox: time.Nanosecond, Audit: time.Nanosecond, Calls: time.Nanosecond})
		e.must(err)
	}
	if p, ok := e.s.(port.TenantPurger); ok {
		_, err := p.PurgeTenant(ctx, "tb")
		e.must(err)
	}
	after, dueAfter := e.everything(snapper)
	e.same(before, after)
	e.same(due, dueAfter)
}

func (e *env) everything(s port.Snapshotter) (map[string][]string, []port.WorkItem) {
	snap, err := s.Snapshot(ctx)
	e.must(err)
	rows := map[string][]string{}
	for name, table := range snap.Tables {
		for _, raw := range table {
			rows[name] = append(rows[name], string(raw))
		}
		slices.Sort(rows[name])
	}
	var due []port.WorkItem
	e.must(e.read(inst, func(tx port.Tx) (err error) {
		due, err = tx.DueWork(t0.Add(48*time.Hour), 0)
		return err
	}))
	return rows, due
}
