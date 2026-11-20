package approval_test

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/port"
)

func TestMain(m *testing.M) {
	debug.SetMemoryLimit(5 << 30)
	os.Exit(m.Run())
}

func heavy(t *testing.T) {
	if raceDetector || testing.Short() {
		t.Skip("bounded exploration of millions of states: run without -race and -short, under a memory cap")
	}
}

func describe(t *testing.T, rep report) {
	t.Helper()
	if rep.truncated {
		t.Fatalf("%s: exploration stopped at the state limit", rep.spec)
	}
	t.Logf("%s: %d states, %d transitions, %d invariants violated", rep.spec, rep.states, rep.transitions, len(rep.violations))
	for _, name := range rep.names() {
		v := rep.violations[name]
		t.Logf("  VIOLATED %s in %d states. First: %s\n    shortest trace: %s", name, rep.counts[name], v.detail, strings.Join(v.trace, " -> "))
	}
}

func TestCorrectedTableIsClean(t *testing.T) {
	for _, askOnce := range []bool{false, true} {
		t.Run(fmt.Sprintf("03-engine §8.2 on core/approval, ask_once=%v", askOnce), func(t *testing.T) {
			if askOnce {
				heavy(t)
			}
			rep := explore(corrected(askOnce))
			describe(t, rep)
			if len(rep.violations) != 0 {
				t.Fatalf("violations: %v", rep.names())
			}
			for _, tag := range []string{tagDeliveredOnRepeat, tagRecheckSessionEnded, tagRecheckQuarantined} {
				if !rep.seen[tag] {
					t.Fatalf("not reached inside the bounds: %s", tag)
				}
			}
		})
	}
}

func TestWiderBoundsReachTheBrakeAndTheRecheck(t *testing.T) {
	heavy(t)
	for _, askOnce := range []bool{false, true} {
		t.Run(fmt.Sprintf("03-engine §8.2 and §8.3, 3 records, 2 pending, ask_once=%v", askOnce), func(t *testing.T) {
			rep := explore(widened(askOnce))
			describe(t, rep)
			if len(rep.violations) != 0 {
				t.Fatalf("violations: %v", rep.names())
			}
			for _, tag := range []string{tagTwoPending, tagBrakeCancelsPending, tagRecheckPaused, tagRecheckSessionEnded, tagRecheckQuarantined, tagDeliveredOnRepeat} {
				if !rep.seen[tag] {
					t.Fatalf("not reached inside the bounds: %s", tag)
				}
			}
		})
	}
}

func walker(t *testing.T, s spec) func(steps ...string) (world, string) {
	return func(steps ...string) (world, string) {
		t.Helper()
		w, last := start(), ""
	next:
		for _, prefix := range steps {
			for _, e := range s.next(w) {
				if strings.HasPrefix(e.label, prefix) {
					w, last = e.to, e.label
					continue next
				}
			}
			t.Fatalf("no transition %q", prefix)
		}
		return w, last
	}
}

func TestBrakeScenario(t *testing.T) {
	s := widened(false)
	walk := walker(t, s)
	w, _ := walk(
		"A1: record 0 opened for arguments 1",
		"A1: record 1 opened for arguments 2",
		"A5: approver 0 decides deny on record 0",
		"A14: the agent is given the answer of record 0",
		"A1: record 2 opened for arguments 1",
		"A5: approver 0 decides deny on record 1",
	)
	if !w.Paused || w.Records[2].State != cancelled || w.Records[2].ArgsKept {
		t.Fatalf("03-engine §8.3: the second consecutive denial writes the pause and cancels the other pending record: paused %v, record 2 %+v", w.Paused, w.Records[2])
	}
	w, _ = walk(
		"A1: record 0 opened for arguments 1",
		"A1: record 1 opened for arguments 2",
		"A3: approver 0 decides approve on record 1",
		"the process crashes",
		"A5: approver 0 decides deny on record 0",
		"A14: the agent is given the answer of record 0",
		"A1: record 2 opened for arguments 1",
		"A5: approver 0 decides deny on record 2",
	)
	if !w.Paused || w.Records[1].State != approved {
		t.Fatalf("a pause written while record 1 is approved: paused %v, %+v", w.Paused, w.Records[1])
	}
	var labels []string
	for _, e := range s.next(w) {
		if strings.Contains(e.label, "record 1") && e.worker {
			labels = append(labels, e.label)
		}
	}
	if got := strings.Join(labels, "; "); strings.Contains(got, "A8:") || !strings.Contains(got, "A9: the re-check of record 1 fails, reason paused") {
		t.Fatalf("03-engine §8.2 A8 and A9 under a pause: %q", got)
	}
}

func TestRepeatScenario(t *testing.T) {
	walk := walker(t, corrected(true))
	seq := func(base []string, more ...string) []string { return append(append([]string{}, base...), more...) }
	approvedSteps := []string{"A1: record 0 opened for arguments 1", "A4: approver 0 decides approve_for_session on record 0"}
	w, last := walk(seq(approvedSteps, "agent repeats arguments 1")...)
	if !w.SessionGrant || w.DirectRuns != 0 || w.Records[0].Delivered || !strings.Contains(last, "attaches to record 0") {
		t.Fatalf("03-engine §8.6 R1 while approved: %q, direct runs %d, delivered %v", last, w.DirectRuns, w.Records[0].Delivered)
	}
	executingSteps := seq(approvedSteps, "A8: record 0 moves to executing", "the engine forwards record 0 with arguments 1")
	w, last = walk(seq(executingSteps, "agent repeats arguments 1")...)
	if w.DirectRuns != 0 || w.Records[0].Executions != 1 || !strings.Contains(last, "attaches to record 0") {
		t.Fatalf("03-engine §8.6 R1 while executing: %q, direct runs %d", last, w.DirectRuns)
	}
	w, last = walk(seq(executingSteps, "A10/A11: the connector answers record 0 (ok=true)", "agent repeats arguments 1")...)
	if w.DirectRuns != 0 || !w.Records[0].Delivered || !strings.Contains(last, "never delivered, is returned") {
		t.Fatalf("03-engine §8.6 R2: %q, direct runs %d", last, w.DirectRuns)
	}
	w, last = walk(seq(executingSteps, "A10/A11: the connector answers record 0 (ok=true)", "agent repeats arguments 1", "agent calls with arguments 1")...)
	if w.DirectRuns != 1 || !strings.Contains(last, "forwarded on the session grant") {
		t.Fatalf("03-engine §8.6 R4: %q, direct runs %d", last, w.DirectRuns)
	}
	unknown := seq(executingSteps, "the call deadline of record 0 passes", "A14: the agent is given the answer of record 0")
	w, last = walk(seq(unknown, "A1: record 1 opened for arguments 1")...)
	if w.Records[0].State != outcomeUnknown || !w.SessionGrant || w.DirectRuns != 0 || w.Records[1].State != pending || !strings.HasPrefix(last, "A1: record 1 opened") {
		t.Fatalf("03-engine §8.6 R3: %q, direct runs %d", last, w.DirectRuns)
	}
	w, last = walk(seq(unknown, "agent calls with arguments 2")...)
	if w.DirectRuns != 1 || !strings.Contains(last, "forwarded on the session grant") {
		t.Fatalf("03-engine §8.6 R3 reads the call hash: %q", last)
	}
}

func TestInvariantsCatchMutants(t *testing.T) {
	heavy(t)
	for _, m := range []struct {
		name   string
		mutate func(*spec)
		want   string
	}{
		{"the deadline reports failed", func(s *spec) { s.deadlineToFailed = true }, "I5 the recorded state tells the truth about the upstream"},
		{"arguments are taken from the retried request", func(s *spec) { s.argsFromRequest = true }, "I2 execution only with the approved arguments"},
		{"the worker forwards again after a crash in executing", func(s *spec) { s.resendAfterCrash = true }, "I1 at most one execution per approval"},
		{"the result timer skips outcome_unknown", func(s *spec) { s.retentionSkipUnknown = true }, "I3 no state is left without a timer and an owner"},
		{"the re-check of A8 always passes", func(s *spec) { s.noRecheck = true }, "I9 nothing starts executing after the session ended, the function was quarantined or a pause was written"},
		{"a plain approve also writes the session grant", func(s *spec) { s.approveWritesGrant = true }, "I10 a session grant is written only by A4"},
		{"the session grant survives the end of the session", func(s *spec) { s.grantSurvivesSession = true }, "I11 no session grant outlives its session or a quarantine of its function"},
		{"a repeat after a terminal state is always a new call", func(s *spec) { s.repeatIsNewCall = true }, "I12 a call whose answer was never delivered is not started again"},
		{"a repeat under a session grant is forwarded beside its open record", func(s *spec) { s.grantBesideOpen = true }, "I13 nothing runs while a record of the same call is open"},
		{"attaching to an open record marks it delivered", func(s *spec) { s.attachMarksDelivered = true }, "I14 the delivered mark is set only when the answer was given to the agent"},
		{"a repeat after outcome_unknown is forwarded on the session grant", func(s *spec) { s.grantAfterUnknown = true }, "I15 a call whose last outcome is unknown is not forwarded on a session grant"},
		{"the brake writes the pause and leaves the other pending records", func(s *spec) { s.brakeKeepsPending = true; s.pendingCap, s.records, s.crashes = 2, 3, 0 }, "I8 a paused session holds no pending approval"},
	} {
		t.Run(m.name, func(t *testing.T) {
			s := corrected(true)
			m.mutate(&s)
			if _, ok := explore(s).violations[m.want]; !ok {
				t.Fatalf("mutant not caught by %q", m.want)
			}
		})
	}
}

func TestPendingRecordsPerSession(t *testing.T) {
	s := corrected(false)
	w, last := start(), ""
	for args := uint8(1); args <= 2; args++ {
		e, _ := s.call(w, args)
		w, last = e.to, e.label
	}
	n := 0
	for _, r := range w.Records {
		if r.State == pending {
			n++
		}
	}
	if n != 1 || !strings.Contains(last, "too_many_pending") {
		t.Fatalf("03-engine §8.2 A2: %d pending records, last answer %q", n, last)
	}
}

func TestApprovedRecordAfterACrash(t *testing.T) {
	s := corrected(false)
	w, _ := walker(t, s)("A1:", "A3:", "the process crashes")
	var got []string
	for _, e := range s.next(w) {
		if e.worker {
			got = append(got, e.label)
		}
	}
	if all := strings.Join(got, "; "); !strings.Contains(all, "A8:") || !strings.Contains(all, "execute-within") {
		t.Fatalf("03-engine §8.2 A8 and A9: worker transitions out of approved after a crash: %q", all)
	}
}

func TestTransitions(t *testing.T) {
	l := approval.Defaults
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	b := port.Binding{AskRequester: true, Approvers: []string{"p2", "u1"}}
	if got := approval.Approvers(b, "u1"); len(got) != 2 || got[0] != "u1" || got[1] != "p2" {
		t.Fatalf("§8.1 approvers: %v", got)
	}
	if got := approval.Approvers(port.Binding{AskRequester: true}, ""); len(got) != 0 {
		t.Fatalf("§8.1: a run for nobody adds no requester: %v", got)
	}
	a := port.Approval{ID: "a", Mode: port.ModeAsk, Approvers: []string{"u1"}, Arguments: &port.Sealed{}}
	if _, err := approval.Open(port.Approval{Mode: port.ModeAsk}, 0, l, now); !port.IsCode(err, port.CodeNoApprover) {
		t.Fatalf("§8.2 A2 no_approver: %v", err)
	}
	if _, err := approval.Open(a, l.MaxPending, l, now); !port.IsCode(err, port.CodeTooManyPending) {
		t.Fatalf("§8.2 A2 too_many_pending: %v", err)
	}
	a, err := approval.Open(a, 0, l, now)
	if err != nil || a.State != port.ApprovalPending || !a.ExpiresAt.Equal(now.Add(l.Timeout)) {
		t.Fatalf("§8.2 A1: %+v %v", a, err)
	}
	if _, _, err := approval.Decide(a, "u2", true, port.DecisionApprove, "", "", l, now); !port.IsCode(err, port.CodeNotAnApprover) {
		t.Fatalf("§8.2 A3 guard: %v", err)
	}
	if _, _, err := approval.Decide(a, "u1", false, port.DecisionApprove, "", "", l, now); !port.IsCode(err, port.CodeNotAnApprover) {
		t.Fatalf("§8.2 A3: the Authorizer allows the approver now: %v", err)
	}
	if _, _, err := approval.Decide(a, "u1", true, port.DecisionApproveForSession, "", "", l, now); err == nil {
		t.Fatal("§8.2 A4: approve for this session needs ask_once")
	}
	if _, err := approval.Expire(a, now); err == nil {
		t.Fatal("§8.2 A6 before expires_at")
	}
	denied, _, err := approval.Decide(a, "u1", true, port.DecisionDeny, strings.Repeat("n", 600), "", l, now)
	if err != nil || denied.State != port.ApprovalDenied || denied.Arguments != nil || len(denied.DecisionNote) != l.NoteMax {
		t.Fatalf("§8.2 A5: %+v %v", denied, err)
	}
	if _, _, err := approval.Decide(denied, "u1", true, port.DecisionApprove, "", "", l, now); !port.IsCode(err, port.CodeInvalidTransition) {
		t.Fatalf("§8.2: a second decision finds the state changed: %v", err)
	}
	ok, _, _ := approval.Decide(a, "u1", true, port.DecisionApprove, "", "", l, now)
	if s, _ := approval.Start(ok, "", time.Minute, l, now.Add(2*l.ExecuteWithin)); s.State != port.ApprovalCancelled || s.Reason != string(port.CodeStaleApproval) {
		t.Fatalf("§8.3 execute-within: %+v", s)
	}
	run, _ := approval.Start(ok, "", time.Minute, l, now)
	if _, err := approval.Unknown(run, l, now.Add(time.Minute)); err == nil {
		t.Fatal("§8.2 A12 inside the deadline plus grace")
	}
	if k, due := approval.Due(run, l, now.Add(time.Minute+l.ExecuteGrace)); !due || k != port.WorkExecuteDeadline {
		t.Fatalf("§8.3 due work: %s %v", k, due)
	}
	unknown, _ := approval.Unknown(run, l, now.Add(2*time.Minute))
	if v := approval.Lookup([]port.Approval{unknown}, nil); v.Rule != approval.Answer {
		t.Fatalf("§8.6 R2: %v", v.Rule)
	}
	unknown, _ = approval.Deliver(unknown, now)
	if v := approval.Lookup([]port.Approval{unknown}, nil); v.Rule != approval.HoldAfterUnknown {
		t.Fatalf("§8.6 R3 after A12: %v", v.Rule)
	}
	never := &port.Call{State: port.CallOutcomeUnknown, StartedAt: now}
	if v := approval.Lookup(nil, never); v.Rule != approval.HoldAfterUnknown {
		t.Fatalf("§8.6 R3 after C12, a call that was never held: %v", v.Rule)
	}
	if v := approval.Lookup(nil, &port.Call{State: port.CallFinished, StartedAt: now}); v.Rule != approval.NewCall {
		t.Fatalf("§8.6 R4: %v", v.Rule)
	}
	p, fired := approval.Brake(port.Pause{}, port.DecisionDeny, l, now, now)
	p, _ = approval.Brake(p, port.DecisionApprove, l, now, now)
	for range 2 {
		p, fired = approval.Brake(p, port.DecisionDeny, l, now, now)
	}
	if fired || p.Consecutive != 2 {
		t.Fatalf("§8.3: an approval resets the consecutive count: %+v", p)
	}
	if p, fired = approval.Brake(p, port.DecisionDeny, l, now, now); !fired || !p.Paused {
		t.Fatalf("§8.3: the third consecutive denial fires the brake: %+v", p)
	}
}

func TestAnExpiredRecordCannotBeDecidedBeforeTheWorkerExpiresIt(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	a, err := approval.Open(port.Approval{ID: "a", Mode: port.ModeAsk, Approvers: []string{"u1"}, Arguments: &port.Sealed{}}, 0, approval.Defaults, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := approval.Decide(a, "u1", true, port.DecisionApprove, "", "", approval.Defaults, a.ExpiresAt); !port.IsCode(err, port.CodeApprovalExpired) {
		t.Fatalf("at expires_at: %v", err)
	}
	if _, _, err := approval.Decide(a, "u1", true, port.DecisionApprove, "", "", approval.Defaults, a.ExpiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("just before expires_at: %v", err)
	}
}
