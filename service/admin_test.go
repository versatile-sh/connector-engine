package service_test

import (
	"context"
	"slices"
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

type fakeSource struct{ manifests []port.Manifest }

func (f fakeSource) Name() string { return "fake" }
func (f fakeSource) List(context.Context, string) (port.ListingPage, error) {
	return port.ListingPage{Manifests: f.manifests}, nil
}
func (f fakeSource) Fetch(context.Context, string, string) (port.Manifest, error) {
	return port.Manifest{}, nil
}

func TestAgentPrincipalIsRefusedOnEveryManagementOperation(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	x := w.agent("x", "s1")
	_, err := w.Svc.InstallConnector(w.Ctx, x, service.InstallInput{Workspace: workspace, Alias: "n", Listing: ref})
	want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "install")
	_, err = w.Svc.DecideApproval(w.Ctx, x, "any", port.DecisionApprove, "", "")
	want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "decideApproval")
	_, err = w.Svc.CreateGrant(w.Ctx, x, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: w.conn.ID, Selector: "*", Mode: port.ModeAllow})
	want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "createGrant")
	_, err = w.Svc.Call(w.Ctx, w.alice, service.CallRequest{Name: "s__search"})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a person has no business on the agent endpoint")
}

func TestPermissionDeniedWithoutRole(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	_, err := w.Svc.InstallConnector(w.Ctx, w.alice, service.InstallInput{Workspace: workspace, Alias: "n", Listing: ref})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a member is no workspace admin")
	_, err = w.Svc.PutCatalogRule(w.Ctx, w.alice, port.CatalogRule{})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a member writes no catalog rule")
	_, err = w.Svc.CreateTenant(w.Ctx, w.admin, "t9")
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a tenant admin does not register tenants")
}

func TestEveryMutationEmitsItsEventWithItsAuditRow(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(_ *service.Config, d *service.Deps) {
		d.Sources = map[string]port.ListingSource{"fake": fakeSource{manifests: []port.Manifest{st.Manifest("other.dev/tool", "1.0.0")}}}
	}}})
	w.acceptAll()
	op := w.Operator()
	audits := func() int {
		n := 0
		w.Read(tenant, func(tx port.Tx) { rows, _ := tx.ListAudit(""); n = len(rows) })
		return n
	}
	step := func(name, event string, fn func() error) {
		t.Helper()
		ev, au := len(w.Events()), audits()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Contains(w.Types(ev), event) {
			t.Fatalf("%s: event %s missing in %v", name, event, w.Types(ev))
		}
		if event != "connector.listing.imported" && event != "connector.catalog.offer_changed" && event != "connector.listing.status_changed" && audits() <= au {
			t.Fatalf("%s: no audit row written", name)
		}
	}
	mine := port.Manifest{Name: "mine.dev/crm", Version: "1", Kind: port.KindMCPRemote, Remote: &port.Remote{URL: "https://x.test"}}
	step("syncSource", "connector.listing.imported", func() error { _, err := w.Svc.SyncSource(w.Ctx, op, "fake"); return err })
	step("createListing", "connector.listing.created", func() error {
		_, err := w.Svc.CreateListing(w.Ctx, w.admin, service.CreateListingInput{Manifest: mine})
		return err
	})
	step("reviewListing", "connector.listing.reviewed", func() error {
		_, err := w.Svc.ReviewListing(w.Ctx, w.admin, port.VersionRef{Listing: port.ListingRef{Owner: tenant, Name: "mine.dev/crm"}, Version: "1"}, service.ReviewInput{})
		return err
	})
	step("deprecateListing", "connector.listing.status_changed", func() error {
		_, err := w.Svc.ChangeListingStatus(w.Ctx, w.admin, port.ListingRef{Owner: tenant, Name: "mine.dev/crm"}, "", port.ListingDeprecated, "old")
		return err
	})
	step("setListingStarter", "connector.listing.status_changed", func() error { _, err := w.Svc.SetListingStarter(w.Ctx, op, ref, true); return err })
	step("putCatalogPolicy", "connector.catalog.policy_changed", func() error {
		_, err := w.Svc.PutCatalogPolicy(w.Ctx, w.admin, port.SubjectWorkspace, "w1", port.PolicyVisible)
		return err
	})
	step("putOffer", "connector.catalog.offer_changed", func() error {
		_, err := w.Svc.PutOffer(w.Ctx, op, port.Offer{Tenant: tenant, TargetKind: port.TargetSource, Target: "fake", Withheld: true})
		return err
	})
	step("putBinding", "connector.binding.changed", func() error {
		_, err := w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: w.conn.ID, Agent: "x", AskRequester: true})
		return err
	})
	var g port.Grant
	step("createGrant", "connector.grant.created", func() error {
		r, err := w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: w.conn.ID, Selector: "fn:search", Mode: port.ModeAsk, DefinitionHash: "old"})
		g = r.Grant
		return err
	})
	step("confirmGrant", "connector.grant.confirmed", func() error { _, err := w.Svc.ConfirmGrant(w.Ctx, w.admin, g.ID); return err })
	step("deleteGrant", "connector.grant.revoked", func() error { return w.Svc.DeleteGrant(w.Ctx, w.admin, g.ID) })
	step("setEnabled", "connector.connector.enabled_changed", func() error {
		_, err := w.Svc.SetConnectorEnabled(w.Ctx, w.admin, w.conn.ID, false)
		return err
	})
	var cred port.Credential
	step("createCredential", "connector.credential.connected", func() error {
		var err error
		cred, err = w.Svc.CreateCredential(w.Ctx, w.alice, service.CredentialInput{Connector: w.conn.ID, OwnerKind: port.OwnerUser, OwnerID: "alice", Kind: port.CredentialAPIKey, Secret: []byte("k")})
		return err
	})
	step("revokeCredential", "connector.credential.revoked", func() error { return w.Svc.RevokeCredential(w.Ctx, w.alice, cred.ID) })
	step("deleteBinding", "connector.binding.changed", func() error { return w.Svc.DeleteBinding(w.Ctx, w.admin, w.conn.ID, "x") })
	step("removeConnector", "connector.connector.removed", func() error { return w.Svc.RemoveConnector(w.Ctx, w.admin, w.conn.ID) })
}

func TestARefusedMutationLeavesNoEvent(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	before := len(w.Events())
	_, err := w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeTenant, Listing: &ref, Selector: "*", Mode: port.ModeAllow})
	want(t, port.IsCode(err, port.CodeRowRefused), true, "a tenant row never says allow (G8)")
	_, err = w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeSession, ScopeID: "s1", Connector: w.conn.ID, Selector: "fn:search", Mode: port.ModeAllow})
	want(t, port.IsCode(err, port.CodeRowRefused), true, "a session row is written by an approval only (G5)")
	want(t, len(w.Events()), before, "no event")
}

func TestIdempotencyKeyRunsAMutationOnce(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	ctx := service.WithIdempotencyKey(w.Ctx, "k1")
	g := port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(workspace), Connector: w.conn.ID, Selector: "class:read", Mode: port.ModeDeny}
	before := len(w.Events())
	st.Ok(w.Svc.CreateGrant(ctx, w.admin, g))
	st.Ok(w.Svc.CreateGrant(ctx, w.admin, g))
	want(t, countOf(w.Types(before), "connector.grant.created"), 1, "one event for two identical requests")
	rows := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeWorkspace}))
	want(t, countOf(types(rows), "class:read"), 1, "one row")
}

func types(rows []port.Grant) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Selector)
	}
	return out
}

func TestStarterRulesAreWrittenAtCreationOnly(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(c *service.Config, _ *service.Deps) { c.TenantDefault = "starter" }}})
	st.Ok(w.Svc.SetListingStarter(w.Ctx, w.Operator(), ref, true))
	before := len(w.Events())
	st.Ok(w.Svc.CreateTenant(w.Ctx, w.Operator(), "t2"))
	types := w.Types(before)
	want(t, slices.Contains(types, "connector.tenant.created"), true, "event 37")
	want(t, countOf(types, "connector.catalog.rule_changed"), 1, "one starter rule")
	admin2 := w.TenantAdmin("t2")
	rules := st.Ok(w.Svc.ListCatalogRules(w.Ctx, admin2))
	want(t, len(rules), 1, "the tenant admin reads the rule")
	want(t, rules[0].CreatedBy, port.StarterCreator, "created by starter")
	want(t, rules[0].Ceiling, port.CeilingRead, "ceiling read")
	st.Ok(w.Svc.CreateTenant(w.Ctx, w.Operator(), "t2"))
	want(t, len(w.Events()), before+len(types), "a second call writes nothing")
	want(t, len(st.Ok(w.Svc.ListCatalogRules(w.Ctx, w.admin))), 2, "t1 got no starter rule: only its own two")
	vis := st.Ok(w.Svc.ListListings(w.Ctx, admin2, "w2", service.ListingFilter{}))
	want(t, len(vis), 1, "the starter listing is in the store of t2")
}

func TestOperatorQuarantineReachesTenantsThroughTheWorker(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	before := len(w.Events())
	res := st.Ok(w.Svc.ChangeListingStatus(w.Ctx, w.Operator(), ref, "", port.ListingQuarantined, "cve"))
	want(t, res.Operation != nil && !res.Operation.Done, true, "an operation with no tenant data")
	want(t, len(res.Blocked), 0, "the operator is not told which connectors it blocks")
	want(t, w.call(x, "s__search", `{"q":"z"}`).Reason, port.ReasonListingQuarantined, "calls are refused from the first commit")
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Status, service.OutcomePending, "the approval waits for the worker")

	w.sweep()
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Code, port.ReasonListingQuarantined, "cancelled by the fan-out")
	var tenantEvents int
	for _, e := range w.Events()[before:] {
		if e.Type == "connector.connector.blocked_changed" && e.Source == port.SourceOf(tenant) {
			tenantEvents++
		}
	}
	want(t, tenantEvents, 1, "event 36 in the stream of the tenant")
	op := st.Ok(w.Svc.GetOperation(w.Ctx, w.Operator(), res.Operation.ID))
	want(t, op.Done, true, "operation done")
	want(t, op.Counts["connectors_blocked"], 1, "counts only")

	st.Ok(w.Svc.ChangeListingStatus(w.Ctx, w.Operator(), ref, "", port.ListingActive, ""))
	want(t, w.call(x, "s__search", `{"q":"y"}`).Status, service.OutcomePending, "works again after restoreListing")
}

func TestCredentialResolutionPerUser(t *testing.T) {
	w := newWorld(t, opts{reviewed: true, auth: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	out := w.call(x, "s__search", `{}`)
	want(t, out.Code, port.CodeCredentialRequired, "no credential connected")
	want(t, out.ConnectAddress, true, "a connect address for per_user with a person")
	nobody := w.AgentClaims(port.Claims{Tenant: tenant, Workspace: workspace, Agent: "x", Session: "s9"})
	want(t, w.call(nobody, "s__search", `{}`).ConnectAddress, false, "a call that acts for nobody gets no connect address")

	cred := st.Ok(w.Svc.CreateCredential(w.Ctx, w.alice, service.CredentialInput{Connector: w.conn.ID, OwnerKind: port.OwnerUser, OwnerID: "alice", Kind: port.CredentialAPIKey, Secret: []byte("k")}))
	want(t, len(cred.Secret.Ciphertext), 0, "metadata only")
	want(t, w.call(x, "s__search", `{"q":"2"}`).Status, service.OutcomeResult, "forwarded with the credential")
	want(t, w.Runtime.Calls[len(w.Runtime.Calls)-1].Credential, cred.ID, "the runtime was told which credential")
	want(t, w.call(nobody, "s__search", `{"q":"3"}`).Code, port.CodeCredentialRequired, "never another owner's credential")
}

func TestTenantRemovalDestroysSealedCredentials(t *testing.T) {
	w, x := askWorld(t)
	cred := st.Ok(w.Svc.CreateCredential(w.Ctx, w.alice, service.CredentialInput{Connector: w.conn.ID, OwnerKind: port.OwnerUser, OwnerID: "alice", Kind: port.CredentialAPIKey, Secret: []byte("hunter2")}))
	var sealed port.Credential
	w.Read(tenant, func(tx port.Tx) { sealed = st.Ok(tx.GetCredential(cred.ID)) })
	held := w.call(x, "s__search", `{}`)
	before := len(w.Events())

	op := st.Ok(w.Svc.RemoveTenant(w.Ctx, w.Operator(), tenant))
	_, err := w.Svc.ListFunctions(w.Ctx, x)
	want(t, port.IsCode(err, port.CodeTenantRemoved), true, "every token is refused at once")
	_, err = w.Svc.ListConnectors(w.Ctx, w.admin, "")
	want(t, port.IsCode(err, port.CodeTenantRemoved), true, "management too")
	want(t, w.Secrets.Destroyed(tenant), false, "the key stays until the worker runs")
	_ = held

	w.sweep()
	want(t, w.Secrets.Destroyed(tenant), true, "crypto-shredding")
	_, err = w.Secrets.Open(w.Ctx, tenant, sealed.Secret)
	want(t, port.IsCode(err, port.CodeUnavailable), true, "the sealed credential can no longer be opened")
	done := st.Ok(w.Svc.GetOperation(w.Ctx, w.Operator(), op.ID))
	want(t, done.Done, true, "operation done")
	want(t, done.Counts["credentials"], 1, "counts")
	want(t, done.Counts["connectors"], 1, "counts")
	var tn port.Tenant
	w.Read("", func(tx port.Tx) { tn = st.Ok(tx.GetTenant(tenant)) })
	want(t, tn.State, port.TenantRemoved, "registry row stays")
	w.Read(tenant, func(tx port.Tx) {
		conns, _ := tx.ListConnectors(port.ConnectorFilter{})
		want(t, len(conns), 0, "rows deleted")
	})
	removed := 0
	for _, e := range w.Events()[before:] {
		if e.Type == "connector.tenant.removed" && e.Source == port.SourceOf(port.Installation) && e.Data["key"] == "wrapped_key_deleted" {
			removed++
		}
	}
	want(t, removed, 1, "event 38 on the installation stream")
}

func TestExplainAccessAndListAgentFunctionsAgree(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	d := st.Ok(w.Svc.ExplainAccess(w.Ctx, w.admin, port.Subject{Agent: "x", Session: "s1", OnBehalfOf: "alice"}, w.conn.ID, "delete_repo"))
	want(t, d.Mode, port.ModeAskOnce, "mode")
	want(t, slices.Contains(d.Notes, port.NoteCappedByDestructiveMax), true, "note")
	want(t, d.Layers[1], string(port.ModeAllow), "workspace layer")
}
