package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

type contentRuntime struct{ *Runtime }

func (contentRuntime) ListContent(context.Context, port.Handle) ([]port.RawDefinition, error) {
	return []port.RawDefinition{{Name: port.ItemPrompt + "greet", Raw: json.RawMessage(`{"name":"greet","description":"Greets"}`)}}, nil
}

func (contentRuntime) Content(context.Context, port.Handle, string, json.RawMessage, time.Time) (port.Result, error) {
	return port.Result{Content: json.RawMessage(`[{"type":"text","text":"hi"}]`)}, nil
}

func (contentRuntime) Complete(context.Context, port.Handle, string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func withContent(c *service.Config, d *service.Deps) {
	c.UpstreamContent = true
	d.Runtime = contentRuntime{d.Runtime.(*Runtime)}
}

// RestoreKeepsDecisions writes grant rows through every service path and in the older shape on src, then fails unless dst restores src's snapshot and decides each request alike.
func RestoreKeepsDecisions(t *testing.T, src, dst Store) {
	t.Helper()
	const tenant, ws, listing = port.TenantID("t1"), port.WorkspaceID("w1"), "acme.dev/s"
	ref := port.ListingRef{Name: listing}
	e := NewOn(t, src, withContent)
	var defs []port.RawDefinition
	hashes := map[string]port.Hash{}
	for _, d := range [][3]string{{"search", "Searches", `"annotations":{"readOnlyHint":true}`}, {"create_issue", "Creates an issue", ""},
		{"delete_repo", "Deletes a repository", `"annotations":{"destructiveHint":true}`}, {"export", "Exports data", ""}} {
		def, h := Definition(d[0], d[1], d[2])
		defs, hashes[d[0]] = append(defs, def), h
	}
	e.Runtime.SetDefinitions(listing, defs...)
	op := e.Operator()
	Ok(e.Svc.CreateTenant(e.Ctx, op, tenant))
	Ok(e.Svc.CreateListing(e.Ctx, op, service.CreateListingInput{Manifest: Manifest(listing, "1.0.0")}))
	Ok(e.Svc.ReviewListing(e.Ctx, op, port.VersionRef{Listing: ref, Version: "1.0.0"}, service.ReviewInput{Functions: []service.ReviewFunction{
		{Name: "search", Hash: hashes["search"], Class: port.ClassRead}, {Name: "create_issue", Hash: hashes["create_issue"], Class: port.ClassWrite},
		{Name: "delete_repo", Hash: hashes["delete_repo"], Class: port.ClassDestructive}}}))
	admin := e.TenantAdmin(tenant)
	for _, r := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
		{ID: "r-ws", SubjectKind: port.SubjectWorkspace, SubjectID: string(ws), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		Ok(e.Svc.PutCatalogRule(e.Ctx, admin, r))
	}
	conn := Ok(e.Svc.InstallConnector(e.Ctx, admin, service.InstallInput{Workspace: ws, Alias: "s", Listing: ref})).Connector
	accept := func() {
		seen := map[string]port.Hash{}
		for _, f := range Ok(e.Svc.ListFunctionsOf(e.Ctx, admin, conn.ID)) {
			hashes[f.Name] = f.DefinitionHash
			if f.State == port.FunctionQuarantined {
				seen[f.Name] = f.DefinitionHash
			}
		}
		if len(seen) > 0 {
			Ok(e.Svc.AcceptFunctions(e.Ctx, admin, conn.ID, seen))
		}
	}
	accept()
	for _, a := range []port.AgentID{"a1", "a2"} {
		Ok(e.Svc.PutBinding(e.Ctx, admin, port.Binding{Connector: conn.ID, Agent: a, AskRequester: true}))
	}
	Ok(e.Svc.SetConnectorLevel(e.Ctx, admin, conn.ID, port.LevelCustom))
	Ok(e.Svc.SetAgentLevel(e.Ctx, admin, conn.ID, "a2", port.LevelRead))
	month := Epoch.Add(30 * 24 * time.Hour)
	row := func(g port.Grant) port.Grant {
		if g.ScopeKind == port.ScopeTenant {
			g.Listing = &ref
		} else {
			g.Connector = conn.ID
		}
		if name, ok := cut(g.Selector); ok && g.ScopeKind != port.ScopeTenant {
			g.DefinitionHash = hashes[name]
		}
		return Ok(e.Svc.CreateGrant(e.Ctx, admin, g)).Grant
	}
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: "*", Mode: port.ModeAllow})
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: "class:destructive", Mode: port.ModeAsk})
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: "fn:create_issue", Mode: port.ModeAsk})
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: "fn:search", Mode: port.ModeAllow})
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: "fn:export", Mode: port.ModeAllow})
	row(port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(ws), Selector: port.ItemPrompt + "greet", Mode: port.ModeAskOnce})
	row(port.Grant{ScopeKind: port.ScopeTenant, ScopeID: string(tenant), Selector: "fn:delete_repo", Mode: port.ModeAsk})
	row(port.Grant{ScopeKind: port.ScopeTenant, ScopeID: string(tenant), Selector: "fn:export", Mode: port.ModeInherit})
	row(port.Grant{ScopeKind: port.ScopeUser, ScopeID: "alice", Selector: "fn:delete_repo", Mode: port.ModeDeny})
	counted := row(port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a1", Selector: "fn:search", Mode: port.ModeAllow, MaxUses: 3, ExpiresAt: month})
	row(port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a1", Selector: "fn:export", Mode: port.ModeAskOnce})
	row(port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a1", Selector: "*", Mode: port.ModeAllow, ExpiresAt: month})
	row(port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a1", Selector: port.ItemPrompt + "*", Mode: port.ModeAllow, ExpiresAt: month})
	agent := func(a port.AgentID, s port.SessionID) port.Principal {
		return e.AgentClaims(port.Claims{Tenant: tenant, Workspace: ws, Agent: a, Session: s, OnBehalfOf: "alice"})
	}
	alice := e.Person(tenant, "alice")
	call := func(p port.Principal, req service.CallRequest) service.Outcome {
		if req.Arguments == nil {
			req.Arguments = json.RawMessage(`{}`)
		}
		return Ok(e.Svc.Call(e.Ctx, p, req))
	}
	held := func(p port.Principal, req service.CallRequest, d port.Decision) {
		out := call(p, req)
		if out.Status != service.OutcomePending {
			t.Fatalf("%s was not held: %+v", req.Name, out)
		}
		Ok(e.Svc.DecideApproval(e.Ctx, alice, out.Approval.ID, d, "", "test"))
		if _, err := e.Worker.Sweep(e.Ctx); err != nil {
			t.Fatal(err)
		}
	}
	a1 := agent("a1", "s1")
	if out := call(a1, service.CallRequest{Name: "s__search"}); out.Status != service.OutcomeResult {
		t.Fatalf("search: %+v", out)
	}
	held(a1, service.CallRequest{Name: "s__create_issue"}, port.DecisionApprove)
	held(a1, service.CallRequest{Name: "s__export"}, port.DecisionApproveForSession)
	held(agent("a1", "s2"), service.CallRequest{Name: "s__greet", Item: port.ItemPrompt}, port.DecisionApproveForSession)
	row(port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a1", Selector: "class:write", Mode: port.ModeAllow, ExpiresAt: e.Clock.Now().Add(time.Hour)})
	defs[0], _ = Definition("search", "Searches, differently", `"annotations":{"readOnlyHint":true}`)
	e.Runtime.SetDefinitions(listing, defs...)
	Ok(e.Svc.Relist(e.Ctx, tenant, conn.ID))
	accept()
	Ok(e.Svc.ConfirmGrant(e.Ctx, admin, counted.ID))
	e.Write(tenant, func(tx port.Tx) error {
		user := port.Grant{ID: "legacy-user", Tenant: tenant, ScopeKind: port.ScopeUser, ScopeID: "alice", Agent: "a1", Connector: conn.ID,
			Selector: "fn:export", Mode: port.ModeDeny, DefinitionHash: hashes["export"], CreatedAt: Epoch, Origin: port.OriginSettings}
		own := port.Grant{ID: "legacy-agent", Tenant: tenant, ScopeKind: port.ScopeAgent, ScopeID: "a2", Agent: "a2", Connector: conn.ID,
			Selector: "fn:create_issue", Mode: port.ModeDeny, DefinitionHash: hashes["create_issue"], CreatedAt: Epoch, Origin: port.OriginSettings}
		return errors.Join(tx.PutGrant(user), tx.PutGrant(own))
	})
	e.Clock.Advance(2 * time.Hour)
	shapes := map[string]bool{}
	e.Read(tenant, func(tx port.Tx) {
		rows, err := tx.ListGrants(port.GrantFilter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range rows {
			name, _ := cut(g.Selector)
			shapes["session "+g.Selector] = shapes["session "+g.Selector] || g.ScopeKind == port.ScopeSession
			shapes["used"] = shapes["used"] || g.Uses > 0
			shapes["expired"] = shapes["expired"] || (!g.ExpiresAt.IsZero() && g.ExpiresAt.Before(e.Clock.Now()))
			shapes["stale"] = shapes["stale"] || (g.Selector == "fn:search" && g.DefinitionHash != hashes[name])
			shapes["agent outside a session"] = shapes["agent outside a session"] || (g.Agent != "" && g.ScopeKind != port.ScopeSession)
			shapes["tenant"] = shapes["tenant"] || g.ScopeKind == port.ScopeTenant
			shapes["level"] = shapes["level"] || g.Origin == port.OriginLevel
		}
	})
	for _, want := range []string{"session fn:export", "session " + port.ItemPrompt + "greet", "used", "expired", "stale", "agent outside a session", "tenant", "level"} {
		if !shapes[want] {
			t.Fatalf("the scenario wrote no %s row: %v", want, shapes)
		}
	}

	snap := Ok(src.(port.Snapshotter).Snapshot(e.Ctx))
	out, err := dst.(port.Snapshotter).Restore(e.Ctx, snap, "ops")
	if err != nil || len(out.Warnings) != 0 {
		t.Fatalf("a snapshot of rows the service wrote: %v, warnings %v", err, out.Warnings)
	}
	modes := map[port.Mode]bool{}
	d := NewOn(t, dst, withContent)
	d.Clock.Advance(e.Clock.Now().Sub(Epoch))
	for _, later := range []time.Duration{0, 40 * 24 * time.Hour} {
		e.Clock.Advance(later)
		d.Clock.Advance(later)
		for _, a := range []port.AgentID{"a1", "a2", "a3"} {
			for _, s := range []port.SessionID{"s1", "s2", "s3"} {
				for _, f := range []string{"search", "create_issue", "delete_repo", "export", port.ItemPrompt + "greet"} {
					sub := port.Subject{Agent: a, Session: s, OnBehalfOf: "alice"}
					was := Ok(e.Svc.ExplainAccess(e.Ctx, admin, sub, conn.ID, f))
					is := Ok(d.Svc.ExplainAccess(d.Ctx, d.TenantAdmin(tenant), sub, conn.ID, f))
					modes[was.Mode] = true
					if fmt.Sprint(was.Mode, was.Reason, was.Step) != fmt.Sprint(is.Mode, is.Reason, is.Step) {
						t.Errorf("%s %s %s after %s: %s %s %d before the restore, %s %s %d after", a, s, f, later, was.Mode, was.Reason, was.Step, is.Mode, is.Reason, is.Step)
					}
				}
			}
		}
	}
	if len(modes) < 4 {
		t.Fatalf("the requests answer only %v", modes)
	}
}

func cut(selector string) (string, bool) {
	if port.ItemKind(selector) != "" {
		return selector, true
	}
	if len(selector) > len(port.SelectorFunction) && selector[:len(port.SelectorFunction)] == port.SelectorFunction {
		return selector[len(port.SelectorFunction):], true
	}
	return "", false
}
