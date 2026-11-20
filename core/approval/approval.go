// Package approval is the approval state machine: transitions A1 to A14, timers, the denial brake and the lookup of a repeated call.
package approval

import (
	"slices"
	"time"

	"github.com/agents-control/connector-engine/port"
)

// Limits are the approval keys.
type Limits struct {
	Timeout          time.Duration
	ExecuteWithin    time.Duration
	ExecuteGrace     time.Duration
	ResultRetention  time.Duration
	MaxPending       int
	BrakeConsecutive int
	BrakePerSession  int
	NoteMax          int
}

// Defaults are the estimates.
var Defaults = Limits{
	Timeout: 600 * time.Second, ExecuteWithin: 60 * time.Second, ExecuteGrace: 10 * time.Second, ResultRetention: time.Hour,
	MaxPending: 5, BrakeConsecutive: 3, BrakePerSession: 10, NoteMax: 500,
}

func transition(a port.Approval, from ...port.ApprovalState) error {
	if !slices.Contains(from, a.State) {
		return port.Errorf(port.CodeInvalidTransition, "approval %s is %s", a.ID, a.State)
	}
	return nil
}

func enter(a port.Approval, to port.ApprovalState) port.Approval {
	a.State = to
	if to == port.ApprovalDenied || to == port.ApprovalExpired || to == port.ApprovalCancelled {
		a.Arguments = nil
	}
	return a
}

// Open is A1: a pending record with its expiry; A2 refuses with no_approver or too_many_pending.
func Open(a port.Approval, pending int, l Limits, now time.Time) (port.Approval, error) {
	switch {
	case len(a.Approvers) == 0:
		return a, port.Errorf(port.CodeNoApprover, "no approver resolves for this call")
	case pending >= l.MaxPending:
		return a, port.Errorf(port.CodeTooManyPending, "%d approvals are pending for this session, agent and connector", pending)
	case a.Mode != port.ModeAsk && a.Mode != port.ModeAskOnce:
		return a, port.Errorf(port.CodeInvalidArgument, "an approval holds mode ask or ask_once, not %s", a.Mode)
	}
	timeout := l.Timeout
	if a.State == "" && a.ExpiresAt.After(now) {
		timeout = a.ExpiresAt.Sub(now)
	}
	a.State, a.CreatedAt, a.ExpiresAt = port.ApprovalPending, now, now.Add(timeout)
	return a, nil
}

// Approvers resolves the approvers of a held call from stored facts: the person acted for when ask_requester, then the binding's.
func Approvers(b port.Binding, onBehalfOf port.PersonID) []string {
	var out []string
	if b.AskRequester && onBehalfOf != "" {
		out = append(out, string(onBehalfOf))
	}
	for _, p := range b.Approvers {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Decide is A3, A4 or A5 on a pending, unexpired record by an allowed approver; it reports whether the caller writes a session row (A4).
func Decide(a port.Approval, by string, allowed bool, d port.Decision, note, channel string, l Limits, now time.Time) (port.Approval, bool, error) {
	if err := transition(a, port.ApprovalPending); err != nil {
		return a, false, err
	}
	if !now.Before(a.ExpiresAt) {
		return a, false, port.Errorf(port.CodeApprovalExpired, "approval %s expired at %s", a.ID, a.ExpiresAt)
	}
	if !allowed || !slices.Contains(a.Approvers, by) {
		return a, false, port.Errorf(port.CodeNotAnApprover, "%s is not an approver of %s", by, a.ID)
	}
	if l.NoteMax > 0 && len([]rune(note)) > l.NoteMax {
		note = string([]rune(note)[:l.NoteMax])
	}
	a.DecidedBy, a.DecidedAt, a.Channel, a.DecisionNote = by, now, channel, note
	switch d {
	case port.DecisionApprove:
		return enter(a, port.ApprovalApproved), false, nil
	case port.DecisionApproveForSession:
		if a.Mode != port.ModeAskOnce {
			return a, false, port.Errorf(port.CodeInvalidArgument, "approve for this session needs mode ask_once")
		}
		a.SessionGrant = true
		return enter(a, port.ApprovalApproved), true, nil
	case port.DecisionDeny:
		return enter(a, port.ApprovalDenied), false, nil
	}
	return a, false, port.Errorf(port.CodeInvalidArgument, "unknown decision %q", d)
}

// Brake counts a decision on the pause row of (session, agent, connector); it reports whether a denial fired the brake.
func Brake(p port.Pause, d port.Decision, l Limits, now, expires time.Time) (port.Pause, bool) {
	if d != port.DecisionDeny {
		p.Consecutive = 0
		return p, false
	}
	p.Consecutive++
	p.Total++
	if p.Paused || (p.Consecutive < l.BrakeConsecutive && p.Total < l.BrakePerSession) {
		return p, false
	}
	p.Paused, p.PausedAt, p.ExpiresAt = true, now, expires
	return p, true
}

// Clear is clearPause: the pause ends and the consecutive count resets.
func Clear(p port.Pause) port.Pause {
	p.Paused, p.Consecutive = false, 0
	return p
}

// Expire is A6, by the worker once expires_at passed.
func Expire(a port.Approval, now time.Time) (port.Approval, error) {
	if err := transition(a, port.ApprovalPending); err != nil {
		return a, err
	}
	if now.Before(a.ExpiresAt) {
		return a, port.Errorf(port.CodeInvalidTransition, "approval %s expires at %s", a.ID, a.ExpiresAt)
	}
	return enter(a, port.ApprovalExpired), nil
}

// Cancel is A7 for a pending record, with the reason of its cause.
func Cancel(a port.Approval, reason port.Code) (port.Approval, error) {
	if err := transition(a, port.ApprovalPending); err != nil {
		return a, err
	}
	a.Reason = string(reason)
	return enter(a, port.ApprovalCancelled), nil
}

// Start is A8 or A9 for an approved record: refused names the step that denied at the re-check, or a guard of A8
// (definition_changed, credential_required, grant_exhausted); a decision older than execute-within is stale_approval.
func Start(a port.Approval, refused port.Code, deadline time.Duration, l Limits, now time.Time) (port.Approval, error) {
	if err := transition(a, port.ApprovalApproved); err != nil {
		return a, err
	}
	if now.Sub(a.DecidedAt) > l.ExecuteWithin {
		refused = port.CodeStaleApproval
	}
	if refused != "" {
		a.Reason = string(refused)
		return enter(a, port.ApprovalCancelled), nil
	}
	a.State, a.ExecutionStartedAt, a.CallDeadline = port.ApprovalExecuting, now, deadline
	return a, nil
}

// Finish is A10 (a result, a tool error included) or A11 (not sent, or a protocol error): the result is kept for retention.
func Finish(a port.Approval, result *port.Sealed, failed bool, l Limits, now time.Time) (port.Approval, error) {
	if err := transition(a, port.ApprovalExecuting); err != nil {
		return a, err
	}
	a.State = port.ApprovalExecuted
	if failed {
		a.State = port.ApprovalFailed
	}
	a.ResultRef, a.ResultExpiresAt = result, now.Add(l.ResultRetention)
	return a, nil
}

// Unknown is A12: the call deadline plus grace passed with no answer; never retried, never forwarded again.
func Unknown(a port.Approval, l Limits, now time.Time) (port.Approval, error) {
	if err := transition(a, port.ApprovalExecuting); err != nil {
		return a, err
	}
	if now.Before(a.ExecutionStartedAt.Add(a.CallDeadline + l.ExecuteGrace)) {
		return a, port.Errorf(port.CodeInvalidTransition, "approval %s is inside its call deadline", a.ID)
	}
	a.State, a.ResultExpiresAt = port.ApprovalOutcomeUnknown, now.Add(l.ResultRetention)
	return a, nil
}

// Retain is A13: the stored result and the sealed arguments are destroyed once retention ends.
func Retain(a port.Approval, now time.Time) (port.Approval, error) {
	if err := transition(a, port.ApprovalExecuted, port.ApprovalFailed, port.ApprovalOutcomeUnknown); err != nil {
		return a, err
	}
	if now.Before(a.ResultExpiresAt) {
		return a, port.Errorf(port.CodeInvalidTransition, "approval %s keeps its result until %s", a.ID, a.ResultExpiresAt)
	}
	a.ResultRef, a.Arguments = nil, nil
	return a, nil
}

// Deliver is A14: set after the answer of a terminal record was written into a response to its agent.
func Deliver(a port.Approval, now time.Time) (port.Approval, error) {
	if !a.State.Terminal() || !a.DeliveredAt.IsZero() {
		return a, port.Errorf(port.CodeInvalidTransition, "approval %s is %s and delivered at %s", a.ID, a.State, a.DeliveredAt)
	}
	a.DeliveredAt = now
	return a, nil
}

// Due names the worker transition a record waits for at now, if any.
func Due(a port.Approval, l Limits, now time.Time) (port.WorkKind, bool) {
	switch {
	case a.State == port.ApprovalPending && !now.Before(a.ExpiresAt):
		return port.WorkApprovalExpiry, true
	case a.State == port.ApprovalApproved:
		return port.WorkApprovedRecord, true
	case a.State == port.ApprovalExecuting && !now.Before(a.ExecutionStartedAt.Add(a.CallDeadline+l.ExecuteGrace)):
		return port.WorkExecuteDeadline, true
	case (a.State == port.ApprovalExecuted || a.State == port.ApprovalFailed || a.State == port.ApprovalOutcomeUnknown) &&
		(a.ResultRef != nil || a.Arguments != nil) && !now.Before(a.ResultExpiresAt):
		return port.WorkResultRetention, true
	}
	return "", false
}

// Rule is the rule that answered a repeated call.
type Rule int

// Rules R1 to R4.
const (
	Attach Rule = iota + 1
	Answer
	HoldAfterUnknown
	NewCall
)

// Verdict is the answer of the lookup for one (session, call_hash).
type Verdict struct {
	Rule   Rule
	Record port.Approval
}

// Lookup applies R1 to R4 to the records and the newest audit row of one (session, call_hash); the first rule that matches ends it.
// HoldAfterUnknown means the session row of step 14 is not applied to this call.
func Lookup(records []port.Approval, latest *port.Call) Verdict {
	for _, a := range records {
		if a.State.Open() {
			return Verdict{Rule: Attach, Record: a}
		}
	}
	var undelivered []port.Approval
	for _, a := range records {
		if a.State.Terminal() && a.DeliveredAt.IsZero() {
			undelivered = append(undelivered, a)
		}
	}
	if len(undelivered) > 0 {
		return Verdict{Rule: Answer, Record: slices.MinFunc(undelivered, func(x, y port.Approval) int { return x.CreatedAt.Compare(y.CreatedAt) })}
	}
	unknown := false
	var at time.Time
	for _, a := range records {
		if a.CreatedAt.After(at) || at.IsZero() {
			at, unknown = a.CreatedAt, a.State == port.ApprovalOutcomeUnknown
		}
	}
	if latest != nil && latest.Approval == "" && latest.StartedAt.After(at) {
		unknown = latest.State == port.CallOutcomeUnknown
	}
	if unknown {
		return Verdict{Rule: HoldAfterUnknown}
	}
	return Verdict{Rule: NewCall}
}
