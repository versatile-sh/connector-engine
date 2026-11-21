package service_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

func askWorld(t *testing.T) (*world, port.Principal) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "*", port.ModeAsk)
	return w, w.agent("x", "s1")
}

func TestApprovalDeniedThenBrake(t *testing.T) {
	w, x := askWorld(t)
	for i, args := range []string{`{"q":"1"}`, `{"q":"2"}`, `{"q":"3"}`} {
		held := w.call(x, "s__search", args)
		want(t, held.Status, service.OutcomePending, "held")
		before := len(w.Events())
		w.Svc.DecideApproval(w.Ctx, w.alice, held.Approval.ID, port.DecisionDeny, "no", "test")
		ans := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
		want(t, ans.Code, service.CodeApprovalDenied, "denied answer")
		if i == 2 {
			want(t, slices.Contains(w.Types(before), "connector.approval.brake"), true, "the third denial fires the brake")
		}
	}
	want(t, w.call(x, "s__search", `{"q":"4"}`).Reason, port.ReasonPaused, "paused")
	want(t, len(w.names(x)), 0, "every function of the connector is hidden")
	pauses := st.Ok(w.Svc.ListPauses(w.Ctx, w.admin))
	if err := w.Svc.ClearPause(w.Ctx, w.admin, pauses[0].ID); err != nil {
		t.Fatal(err)
	}
	want(t, w.call(x, "s__search", `{"q":"5"}`).Status, service.OutcomePending, "clearPause lifts it")
}

func TestAnAgentTokenCannotDecide(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	want(t, held.Status, service.OutcomePending, "held")
	for _, d := range []port.Decision{port.DecisionApprove, port.DecisionDeny} {
		_, err := w.Svc.DecideApproval(w.Ctx, x, held.Approval.ID, d, "", "test")
		want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "the agent that asked decides nothing: "+string(d))
	}
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Status, service.OutcomePending, "still pending")
	want(t, w.Runtime.CallsTo("search"), 0, "nothing was forwarded")
}

func TestApprovalExpires(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	w.Clock.Advance(599 * time.Second)
	w.sweep()
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Status, service.OutcomePending, "still pending before the timeout")
	w.Clock.Advance(2 * time.Second)
	before := len(w.Events())
	w.sweep()
	ans := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
	want(t, ans.Code, port.CodeApprovalExpired, "expired answer")
	want(t, w.Types(before), []string{"connector.approval.expired"}, "event 16")
	_, err := w.Svc.DecideApproval(w.Ctx, w.alice, held.Approval.ID, port.DecisionApprove, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	want(t, w.Runtime.CallsTo("search"), 0, "an expired record is never forwarded")
}

func TestStaleApprovalIsCancelledAtTheRecheck(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	w.decide(held.Approval.ID, port.DecisionApprove)
	w.Clock.Advance(61 * time.Second)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 0, "stale decision not run")
	got := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
	want(t, got.Code, port.CodeStaleApproval, "reason")
}

func TestApprovedCallRefusedWhenFunctionChanged(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	w.decide(held.Approval.ID, port.DecisionApprove)
	w.Runtime.SetDefinitions(listing, w.redefine("search", "Searches, differently")...)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 0, "a definition that changed before A8 is not run")
	got := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
	want(t, got.Code, port.ReasonQuarantined, "the re-check sees the quarantine")
}

func TestOutcomeUnknownAndHoldAfterUnknown(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) { return port.Result{}, context.DeadlineExceeded })
	before := len(w.Events())
	out := w.call(x, "s__search", `{"q":"u"}`)
	want(t, out.Code, service.CodeOutcomeUnknown, "a silent deadline is not a failure")
	want(t, w.Types(before), []string{"connector.call.started"}, "the row was written before forwarding")

	w.sweep()
	want(t, len(w.Types(before)), 1, "inside deadline plus grace nothing changes")
	w.Clock.Advance(71 * time.Second)
	w.sweep()
	want(t, w.Types(before), []string{"connector.call.started", "connector.call.outcome_unknown"}, "C12")
	w.sweep()
	want(t, len(w.Types(before)), 2, "the sweep acts once")

	w.Runtime.Handle("search", nil)
	again := w.call(x, "s__search", `{"q":"u"}`)
	want(t, again.Status, service.OutcomeResult, "plain allow rows are not held after an unknown outcome (R3)")
}

func TestHeldCallOutcomeUnknownIsNeverRetried(t *testing.T) {
	w, x := askWorld(t)
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) { return port.Result{}, context.DeadlineExceeded })
	held := w.call(x, "s__search", `{"q":"1"}`)
	w.decide(held.Approval.ID, port.DecisionApprove)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "forwarded once")
	w.Clock.Advance(71 * time.Second)
	w.sweep()
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "never forwarded again")
	got := st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID))
	want(t, got.Code, service.CodeOutcomeUnknown, "answer")
	detail := st.Ok(w.Svc.GetApproval(w.Ctx, w.alice, held.Approval.ID))
	want(t, detail.Approval.State, port.ApprovalOutcomeUnknown, "state")
}

func TestUpdateQuarantinesChangedAndNewFunctions(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	held := w.call(x, "s__delete_repo", `{}`)
	w.decide(held.Approval.ID, port.DecisionApproveForSession)
	w.bind("z")
	w.row(port.ScopeAgent, "z", "*", port.ModeAllow)
	pending := w.call(w.agent("z", "s1"), "s__delete_repo", `{"q":"z"}`)
	want(t, pending.Status, service.OutcomePending, "a second agent is held")
	m2 := st.Manifest(listing, "2.0.0")
	st.Ok(w.Svc.CreateListing(w.Ctx, w.Operator(), service.CreateListingInput{Manifest: m2}))
	st.Ok(w.Svc.ReviewListing(w.Ctx, w.Operator(), port.VersionRef{Listing: ref, Version: "2.0.0"}, service.ReviewInput{Functions: []service.ReviewFunction{
		{Name: "create_issue", Hash: w.hashes["create_issue"], Class: port.ClassWrite}}}))
	list := w.redefine("delete_repo", "Deletes a repository v2")
	extra, _ := st.Definition("comment", "Comments", "")
	w.Runtime.SetDefinitions(listing, append(list, extra)...)
	before := len(w.Events())

	res := st.Ok(w.Svc.UpdateConnector(w.Ctx, w.admin, w.conn.ID, "2.0.0"))
	want(t, res.Quarantined, []string{"comment", "delete_repo"}, "changed and new are quarantined")
	sessions := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeSession}))
	want(t, len(sessions), 0, "session rows on a changed function are dropped")
	want(t, res.Connector.Listing.Version, "2.0.0", "pinned version")
	types := w.Types(before)
	want(t, slices.Contains(types, "connector.connector.updated"), true, "event 4")
	want(t, countOf(types, "connector.function.quarantined"), 2, "event 6 per function")
	want(t, countOf(types, "connector.approval.cancelled"), 1, "pending approval of a quarantined function is cancelled")
	w.sweep()
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Code, port.ReasonQuarantined, "the approved record is cancelled by the re-check")
	want(t, w.Runtime.CallsTo("delete_repo"), 0, "nothing ran")
	want(t, w.call(x, "s__comment", `{}`).Reason, port.ReasonQuarantined, "new function refused")
	want(t, w.call(x, "s__create_issue", `{}`).Status, service.OutcomeResult, "an unchanged function keeps working")
}

func countOf(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

func TestOverlayChangesServedDescription(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	desc := func() string {
		l := st.Ok(w.Svc.ListFunctions(w.Ctx, x))
		for _, f := range l.Functions {
			if f.Name == "s__search" {
				var d struct{ Description string }
				json.Unmarshal(f.Definition, &d)
				return d.Description
			}
		}
		return ""
	}
	target := service.OverlayTarget{Connector: w.conn.ID}
	h := w.hashes["search"]
	want(t, desc(), "Searches", "upstream text")
	o1 := st.Ok(w.Svc.ProposeOverlay(w.Ctx, w.admin, target, "search", "", "Find things", h))
	want(t, desc(), "Searches", "a proposal is not served")
	st.Ok(w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o1.Version, h))
	want(t, desc(), "Find things", "accepted text is served")
	o2 := st.Ok(w.Svc.ProposeOverlay(w.Ctx, w.admin, target, "search", "", "Look things up", h))
	st.Ok(w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o2.Version, h))
	want(t, desc(), "Look things up", "second version")
	st.Ok(w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o1.Version, h))
	want(t, desc(), "Find things", "restore")
	_, err := w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o2.Version, "stale-hash")
	want(t, port.IsCode(err, port.CodeDefinitionChanged), true, "acceptance names the hash seen")
	if err := w.Svc.ClearOverlay(w.Ctx, w.admin, target, "search"); err != nil {
		t.Fatal(err)
	}
	want(t, desc(), "Searches", "clear")
	out := w.call(x, "s__search", `{}`)
	want(t, out.Status, service.OutcomeResult, "call")
	want(t, countOf(w.Types(0), "connector.overlay.changed"), 4, "events 39")
}

func TestProjectionThenFilter(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	out := `"outputSchema":{"type":"object","properties":{"title":{"type":"string"},"note":{"type":"string"}}}`
	def, h := st.Definition("search", "Searches", out)
	list := w.redefine("search", "")
	for i := range list {
		if list[i].Name == "search" {
			list[i] = def
		}
	}
	w.Runtime.SetDefinitions(listing, list...)
	st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", h, false))
	st.Ok(w.Svc.ClassifyFunction(w.Ctx, w.admin, w.conn.ID, "search", port.ClassRead))
	st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "search", true))
	w.allowAll("x")
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
		return port.Result{Structured: json.RawMessage(`{"title":"a secret title","note":"another secret"}`)}, nil
	})
	x := w.agent("x", "s1")

	call := func(fields ...string) service.Outcome {
		return st.Ok(w.Svc.Call(w.Ctx, x, service.CallRequest{Name: "s__search", Arguments: json.RawMessage(`{"q":"1"}`), Fields: fields}))
	}
	plain := call()
	want(t, string(plain.Result.Structured), `{"title":"a secret title","note":"another secret"}`, "no filter adapter: result unchanged")

	bad := call("/missing")
	want(t, bad.Code, port.CodeInvalidPath, "a path that does not resolve is a tool error before C8")
	want(t, countOf(w.Types(0), "connector.call.started"), 1, "no audit row for the refused path")
}

func TestProjectionAppliesBeforeTheFilter(t *testing.T) {
	var cfgTune = func(_ *service.Config, d *service.Deps) { d.Filter = st.Filter{} }
	w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){cfgTune}})
	out := `"outputSchema":{"type":"object","properties":{"title":{"type":"string"},"note":{"type":"string"}}}`
	def, h := st.Definition("search", "Searches", out)
	list := w.redefine("search", "")
	for i := range list {
		if list[i].Name == "search" {
			list[i] = def
		}
	}
	w.Runtime.SetDefinitions(listing, list...)
	st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", h, false))
	st.Ok(w.Svc.ClassifyFunction(w.Ctx, w.admin, w.conn.ID, "search", port.ClassRead))
	st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "search", true))
	w.allowAll("x")
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
		return port.Result{Content: json.RawMessage(`[{"type":"text","text":"{\"title\":\"a secret title\",\"note\":\"another secret\"}"}]`),
			Structured: json.RawMessage(`{"title":"a secret title","note":"another secret"}`)}, nil
	})
	x := w.agent("x", "s1")
	got := st.Ok(w.Svc.Call(w.Ctx, x, service.CallRequest{Name: "s__search", Arguments: json.RawMessage(`{"q":"1"}`), Fields: []string{"/title"}}))
	want(t, string(got.Result.Structured), `{"title":"a *** title"}`, "projection first, then the filter on what is left")
	want(t, got.Findings["secret"], 2, "the filter saw the kept field in the content and the structured block only")
	whole := st.Ok(w.Svc.Call(w.Ctx, x, service.CallRequest{Name: "s__search", Arguments: json.RawMessage(`{"q":"2"}`)}))
	want(t, whole.Findings["secret"], 4, "without projection the filter sees both fields in both blocks")
}

func TestRelistFollowsTheUpstreamTTLWithinTheBounds(t *testing.T) {
	cases := []struct {
		name     string
		ttl      time.Duration
		notYet   time.Duration
		dueAfter time.Duration
	}{
		{"ttl inside the bounds", 100 * time.Second, 99 * time.Second, 101 * time.Second},
		{"ttl below relistMin is raised", time.Second, 29 * time.Second, 31 * time.Second},
		{"ttl above relistMax is cut", time.Hour, 299 * time.Second, 301 * time.Second},
		{"no ttl means relistMax", 0, 299 * time.Second, 301 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t, opts{reviewed: true})
			w.Runtime.SetMeta(listing, port.ListMeta{TTL: c.ttl})
			st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
			w.Clock.Advance(c.notYet)
			due := st.Ok(w.Svc.RelistDue(w.Ctx, tenant))
			want(t, len(due), 0, "not due before the bounded ttl")
			w.Clock.Advance(c.dueAfter - c.notYet)
			due = st.Ok(w.Svc.RelistDue(w.Ctx, tenant))
			want(t, len(due), 1, "due after it")
		})
	}
}

func TestRelistBeforeCallDecidesWhetherAnAllowedCallSeesUpstreamDrift(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		status service.OutcomeStatus
	}{{"held", service.OutcomeResult}, {"all", service.OutcomeUnknownTool}} {
		t.Run(tc.mode, func(t *testing.T) {
			w := newWorld(t, opts{reviewed: true, tune: []func(*service.Config, *service.Deps){func(c *service.Config, _ *service.Deps) { c.RelistBeforeCall = tc.mode }}})
			w.customWorkspace()
			w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
			w.bind("x")
			w.row(port.ScopeAgent, "x", "fn:search", port.ModeAllow)
			x := w.agent("x", "s1")
			w.Runtime.SetDefinitions(listing, w.redefine("search", "Searches, and now phones home")...)
			want(t, w.call(x, "s__search", `{"q":"a"}`).Status, tc.status, "the call inside the relistMax window")
		})
	}
}

func TestAnUndeliveredAnswerIsRepeatedNotForwardedAgain(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAskOnce)
	x := w.agent("x", "s1")
	held := w.call(x, "s__create_issue", `{"q":"1"}`)
	w.decide(held.Approval.ID, port.DecisionApproveForSession)
	w.sweep()
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, held.Approval.ID)).Status, service.OutcomeResult, "the status answers the result")
	again := w.call(x, "s__create_issue", `{"q":"1"}`)
	want(t, again.Status, service.OutcomeResult, "the repeat gets the stored answer")
	want(t, w.Runtime.CallsTo("create_issue"), 1, "an answer the agent may not have received is never forwarded again")
	if err := w.Svc.MarkDelivered(w.Ctx, x, held.Approval.ID); err != nil {
		t.Fatal(err)
	}
	w.call(x, "s__create_issue", `{"q":"1"}`)
	want(t, w.Runtime.CallsTo("create_issue"), 2, "after delivery the session row forwards the repeat")
}

func TestAFailedRelistLeavesTheApprovedRecordForTheNextSweep(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	w.decide(held.Approval.ID, port.DecisionApprove)
	w.Runtime.OnList = func() error { return port.Errorf(port.CodeUnavailable, "upstream down") }
	if _, err := w.Worker.Sweep(w.Ctx); err == nil {
		t.Fatal("the failed re-list is reported")
	}
	want(t, w.Runtime.CallsTo("search"), 0, "nothing is forwarded on a definition that could not be re-listed")
	want(t, st.Ok(w.Svc.GetApproval(w.Ctx, w.alice, held.Approval.ID)).Approval.State, port.ApprovalApproved, "the record stays approved")
	w.Runtime.OnList = nil
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 0, "a failed item backs off")
	w.Clock.Advance(6 * time.Second)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "the sweep after the backoff runs it")

	stale := w.call(x, "s__search", `{"q":"2"}`)
	w.decide(stale.Approval.ID, port.DecisionApprove)
	w.Runtime.OnList = func() error { return port.Errorf(port.CodeUnavailable, "upstream down") }
	w.Clock.Advance(61 * time.Second)
	_, _ = w.Worker.Sweep(w.Ctx)
	want(t, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, stale.Approval.ID)).Code, port.CodeStaleApproval, "a record whose re-list keeps failing goes stale (A9)")
}

func TestACancelledOrPostSendFailureIsOutcomeUnknown(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	for _, err := range []error{context.Canceled, port.Errorf(port.CodeOutcomeUnknown, "connection reset after the send")} {
		w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) { return port.Result{}, err })
		want(t, w.call(x, "s__search", `{}`).Code, service.CodeOutcomeUnknown, err.Error())
	}
}

func TestTheSessionRowEndsWithTheAgentToken(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	w.row(port.ScopeAgent, "x", "fn:create_issue", port.ModeAskOnce)
	exp := st.Epoch.Add(10 * time.Minute)
	x := w.AgentClaims(port.Claims{Tenant: tenant, Workspace: workspace, Agent: "x", Session: "s1", OnBehalfOf: "alice", ExpiresAt: exp})
	held := w.call(x, "s__create_issue", `{"q":"1"}`)
	if until := st.Ok(w.Svc.GetApproval(w.Ctx, w.alice, held.Approval.ID)).SessionUntil; !until.Equal(exp) {
		t.Fatalf("the pending call says a session approval ends at %v, not with the token at %v", until, exp)
	}
	w.decide(held.Approval.ID, port.DecisionApproveForSession)
	rows := st.Ok(w.Svc.ListGrants(w.Ctx, w.admin, port.GrantFilter{ScopeKind: port.ScopeSession, ScopeID: "s1"}))
	if len(rows) != 1 || !rows[0].ExpiresAt.Equal(exp) {
		t.Fatalf("the session row ends with the token: %+v", rows)
	}
	if until := st.Ok(w.Svc.GetApproval(w.Ctx, w.alice, held.Approval.ID)).SessionUntil; !until.IsZero() {
		t.Fatalf("a decided call still offers a session approval until %v", until)
	}
}

func TestAnUpstreamChangeKeepsOrSupersedesTheOverlayAtAcceptance(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.allowAll("x")
	x := w.agent("x", "s1")
	desc := func() string {
		for _, f := range st.Ok(w.Svc.ListFunctions(w.Ctx, x)).Functions {
			if f.Name == "s__search" {
				var d struct{ Description string }
				json.Unmarshal(f.Definition, &d)
				return d.Description
			}
		}
		return ""
	}
	target := service.OverlayTarget{Connector: w.conn.ID}
	o1 := st.Ok(w.Svc.ProposeOverlay(w.Ctx, w.admin, target, "search", "", "Find things", w.hashes["search"]))
	st.Ok(w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o1.Version, w.hashes["search"]))
	redefine := func(text string) port.Hash {
		w.Runtime.SetDefinitions(listing, w.redefine("search", text)...)
		res := st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
		return res.Changes[0].NewHash
	}
	h2 := redefine("Searches v2")
	st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", h2, true))
	want(t, desc(), "Find things", "a kept overlay is carried to the new upstream hash")
	h3 := redefine("Searches v3")
	st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", h3, false))
	want(t, desc(), "Searches v3", "an overlay not kept is superseded")
	st.Ok(w.Svc.ActivateOverlay(w.Ctx, w.admin, target, "search", o1.Version, h3))
	want(t, desc(), "Find things", "an older version is restored against the current upstream hash")
}

func TestASpentUseLimitedRowRunsOneApprovedRecord(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.customWorkspace()
	w.row(port.ScopeWorkspace, string(workspace), "*", port.ModeAllow)
	w.bind("x")
	once := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "x", Connector: w.conn.ID, Selector: "fn:search", Mode: port.ModeAskOnce, DefinitionHash: w.hashes["search"], MaxUses: 1}
	st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, once))
	x := w.agent("x", "s1")
	first, second := w.call(x, "s__search", `{"q":"1"}`), w.call(x, "s__search", `{"q":"2"}`)
	w.decide(first.Approval.ID, port.DecisionApprove)
	w.decide(second.Approval.ID, port.DecisionApprove)
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 1, "a use-limited row runs once")
	codes := []port.Code{st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, first.Approval.ID)).Code, st.Ok(w.Svc.ApprovalStatus(w.Ctx, x, second.Approval.ID)).Code}
	spent := slices.Contains(codes, port.ReasonNoAgentGrant) || slices.Contains(codes, port.CodeGrantExhausted)
	want(t, spent, true, "the other record is cancelled: the spent row no longer matches, or its reservation fails")
}

func TestADroppedSessionIsRestartedAfterALostAnswer(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeResult, "first call")
	starts := w.Runtime.Starts
	w.Runtime.Handle("create_issue", func(json.RawMessage) (port.Result, error) {
		return port.Result{}, port.Errorf(port.CodeOutcomeUnknown, "the connection closed before the answer")
	})
	want(t, w.call(x, "s__create_issue", `{}`).Code, service.CodeOutcomeUnknown, "a write that may have run")
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeResult, "the next call")
	want(t, w.Runtime.Starts, starts+1, "the session that lost the answer is not reused")
}

func TestADecisionAfterTheTimeoutIsRefusedBeforeTheSweep(t *testing.T) {
	w, x := askWorld(t)
	held := w.call(x, "s__search", `{}`)
	w.Clock.Advance(601 * time.Second)
	if _, err := w.Svc.DecideApproval(w.Ctx, w.alice, held.Approval.ID, port.DecisionApprove, "", "test"); !port.IsCode(err, port.CodeApprovalExpired) {
		t.Fatalf("decide after expiry: %v", err)
	}
	w.sweep()
	want(t, w.Runtime.CallsTo("search"), 0, "never forwarded")
}

func TestACapturedNameOutsideTheCharsetIsRefused(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	bad, _ := st.Definition("click [here](https://evil.test)", "Looks like a link", "")
	_, _, list := definitions("Searches", "Creates an issue")
	w.Runtime.SetDefinitions(listing, append(list, bad)...)
	if _, err := w.Svc.UpdateConnector(w.Ctx, w.admin, w.conn.ID, "1.0.0"); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("a markup name was captured: %v", err)
	}
}
