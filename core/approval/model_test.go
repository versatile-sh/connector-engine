package approval_test

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/port"
)

var states = []port.ApprovalState{"", port.ApprovalPending, port.ApprovalApproved, port.ApprovalExecuting, port.ApprovalExecuted,
	port.ApprovalFailed, port.ApprovalOutcomeUnknown, port.ApprovalDenied, port.ApprovalExpired, port.ApprovalCancelled}

const (
	none uint8 = iota
	pending
	approved
	executing
	executed
	failed
	outcomeUnknown
	denied
	expired
	cancelled
)

func stateOf(s port.ApprovalState) uint8 { return uint8(slices.Index(states, s)) }

type spec struct {
	name                 string
	askOnce              bool
	pendingCap           int
	brakeAfter           int
	crashes              int
	calls                int
	records              int
	noRecheck            bool
	approveWritesGrant   bool
	grantSurvivesSession bool
	repeatIsNewCall      bool
	grantBesideOpen      bool
	attachMarksDelivered bool
	grantAfterUnknown    bool
	brakeKeepsPending    bool
	deadlineToFailed     bool
	resendAfterCrash     bool
	argsFromRequest      bool
	retentionSkipUnknown bool
}

func corrected(askOnce bool) spec {
	return spec{name: "core/approval, 2 records, 2 crashes", askOnce: askOnce, pendingCap: 1, brakeAfter: 2, crashes: 2, calls: 3, records: 2}
}

func widened(askOnce bool) spec {
	s := corrected(askOnce)
	s.name = "core/approval, 3 records, 2 pending, 1 crash"
	s.pendingCap, s.records, s.crashes = 2, 3, 1
	return s
}

func (s spec) limits() approval.Limits {
	l := approval.Defaults
	l.MaxPending, l.BrakeConsecutive, l.BrakePerSession = s.pendingCap, s.brakeAfter, 255
	return l
}

const (
	maxRecords = 3
	maxStates  = 12000000
	approvers  = 2
)

const (
	tagBrakeCancelsPending = "the brake cancels another pending record"
	tagRecheckPaused       = "A9 for an approved record after a pause"
	tagRecheckSessionEnded = "A9 for an approved record after the session ended"
	tagRecheckQuarantined  = "A9 for an approved record after a quarantine"
	tagDeliveredOnRepeat   = "an undelivered answer returned to a repeat"
	tagTwoPending          = "two records pending at once"
)

type flight uint8

const (
	idle flight = iota
	committed
	sent
)

type record struct {
	State        uint8
	Args         uint8
	ArgsKept     bool
	ResultKept   bool
	Executions   uint8
	ExecutedArgs uint8
	Decided      bool
	Handoff      bool
	Flight       flight
	GotError     bool
	Delivered    bool
	Given        bool
}

type world struct {
	Records      [maxRecords]record
	Role         [approvers]bool
	SessionEnded bool
	Quarantined  bool
	SessionGrant bool
	Paused       bool
	Denials      uint8
	Crashes      uint8
	Calls        uint8
	DirectRuns   uint8
	StoredReturn bool
	Cleared      bool
}

type edge struct {
	label  string
	worker bool
	to     world
	tag    string
	starts uint8
}

var (
	t0          = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	later       = t0.Add(24 * time.Hour)
	sealed      = &port.Sealed{}
	approverIDs = []string{"p0", "p1"}
)

func start() world { return world{Role: [approvers]bool{true, true}} }

func (s spec) mode() port.Mode {
	if s.askOnce {
		return port.ModeAskOnce
	}
	return port.ModeAsk
}

func (s spec) toApproval(r record, i int) port.Approval {
	a := port.Approval{State: states[r.State], Mode: s.mode(), Approvers: approverIDs, CreatedAt: t0.Add(time.Duration(i) * time.Second), ExpiresAt: t0.Add(time.Second), ExecutionStartedAt: t0, ResultExpiresAt: t0}
	if r.ArgsKept {
		a.Arguments = sealed
	}
	if r.ResultKept {
		a.ResultRef = sealed
	}
	if r.Decided {
		a.DecidedAt = t0
	}
	if r.Delivered {
		a.DeliveredAt = t0
	}
	return a
}

func from(r record, a port.Approval) record {
	r.State, r.ArgsKept, r.ResultKept = stateOf(a.State), a.Arguments != nil, a.ResultRef != nil
	r.Decided, r.Delivered = !a.DecidedAt.IsZero(), !a.DeliveredAt.IsZero()
	return r
}

func (r *record) open() bool     { return states[r.State].Open() }
func (r *record) terminal() bool { return states[r.State].Terminal() }

func (r *record) final() bool {
	return r.State == none || (r.terminal() && !r.ArgsKept && !r.ResultKept)
}

func (w world) quiet() bool {
	for i := range w.Records {
		if r := &w.Records[i]; r.Flight != idle || r.Handoff {
			return false
		}
	}
	return true
}

func (w world) final() bool {
	for i := range w.Records {
		if !w.Records[i].final() {
			return false
		}
	}
	return true
}

func (w world) notGiven(args uint8) int {
	for i := range w.Records {
		if r := &w.Records[i]; r.Args == args && r.terminal() && !r.Given {
			return i
		}
	}
	return -1
}

func (w world) openFor(args uint8) int {
	for i := range w.Records {
		if r := &w.Records[i]; r.Args == args && r.open() {
			return i
		}
	}
	return -1
}

func (w world) newest(args uint8) int {
	for i := len(w.Records) - 1; i >= 0; i-- {
		if r := &w.Records[i]; r.Args == args && r.State != none {
			return i
		}
	}
	return -1
}

func (w world) unknownLast(args uint8) bool {
	i := w.newest(args)
	return i >= 0 && w.Records[i].State == outcomeUnknown
}

func (w world) closedBy() (port.Code, string) {
	switch {
	case w.SessionEnded:
		return port.ReasonSessionEnded, tagRecheckSessionEnded
	case w.Quarantined:
		return port.ReasonQuarantined, tagRecheckQuarantined
	case w.Paused:
		return port.ReasonPaused, tagRecheckPaused
	}
	return "", ""
}

func (s spec) cancel(w *world, i int, reason port.Code) bool {
	a, err := approval.Cancel(s.toApproval(w.Records[i], i), reason)
	if err != nil {
		return false
	}
	w.Records[i] = from(w.Records[i], a)
	w.Records[i].Handoff = false
	return true
}

func (s spec) call(w world, args uint8) (edge, bool) {
	to, label, ok := s.callInner(w, args)
	if !ok {
		return edge{}, false
	}
	e := edge{label: label, to: to}
	starts := to.DirectRuns != w.DirectRuns
	pend := 0
	for i := range to.Records {
		starts = starts || (to.Records[i].State == pending && w.Records[i].State == none)
		if to.Records[i].State == pending {
			pend++
		}
	}
	switch {
	case to.StoredReturn && !w.StoredReturn:
		e.tag = tagDeliveredOnRepeat
	case starts && pend > 1:
		e.tag = tagTwoPending
	}
	if starts {
		e.starts = args
	}
	return e, true
}

func (s spec) callInner(w world, args uint8) (world, string, bool) {
	if int(w.Calls) >= s.calls || w.SessionEnded {
		return w, "", false
	}
	w.Calls++
	if w.Paused {
		return w, fmt.Sprintf("agent calls with arguments %d: refused, paused", args), true
	}
	if w.Quarantined {
		return w, fmt.Sprintf("agent calls with arguments %d: refused, quarantined", args), true
	}
	if s.grantBesideOpen && s.askOnce && w.SessionGrant && w.openFor(args) >= 0 {
		w.DirectRuns++
		return w, fmt.Sprintf("agent repeats arguments %d: forwarded on the session grant beside its open record", args), true
	}
	var same []port.Approval
	var index []int
	for i := range w.Records {
		if r := w.Records[i]; r.Args == args && r.State != none {
			same, index = append(same, s.toApproval(r, i)), append(index, i)
		}
	}
	v := approval.Lookup(same, nil)
	at := func() int {
		return index[slices.IndexFunc(same, func(a port.Approval) bool { return a.CreatedAt.Equal(v.Record.CreatedAt) })]
	}
	if s.repeatIsNewCall && v.Rule == approval.Answer {
		v.Rule = approval.NewCall
	}
	switch v.Rule {
	case approval.Attach:
		i := at()
		w.Records[i].Delivered = w.Records[i].Delivered || s.attachMarksDelivered
		return w, fmt.Sprintf("agent repeats arguments %d: attaches to record %d", args, i), true
	case approval.Answer:
		i := at()
		a, err := approval.Deliver(v.Record, t0)
		if err != nil {
			panic(err)
		}
		w.Records[i] = from(w.Records[i], a)
		w.Records[i].Given, w.StoredReturn = true, true
		return w, fmt.Sprintf("agent repeats arguments %d: the answer of record %d, never delivered, is returned and marked delivered", args, i), true
	}
	if s.askOnce && w.SessionGrant && (v.Rule == approval.NewCall || s.grantAfterUnknown) {
		w.DirectRuns++
		return w, fmt.Sprintf("agent calls with arguments %d: forwarded on the session grant", args), true
	}
	pend := 0
	for i := range w.Records {
		if w.Records[i].State == pending {
			pend++
		}
	}
	for i := range w.Records {
		if i < s.records && w.Records[i].State == none {
			a, err := approval.Open(port.Approval{Mode: s.mode(), Approvers: approverIDs}, pend, s.limits(), t0)
			if port.IsCode(err, port.CodeTooManyPending) {
				return w, fmt.Sprintf("agent calls with arguments %d: refused, too_many_pending", args), true
			}
			if err != nil {
				panic(err)
			}
			w.Records[i] = from(record{Args: args}, a)
			w.Records[i].ArgsKept = true
			return w, fmt.Sprintf("A1: record %d opened for arguments %d", i, args), true
		}
	}
	return w, "", false
}

var decisions = []port.Decision{port.DecisionApprove, port.DecisionApproveForSession, port.DecisionDeny}

func (s spec) decide(w world, i, a int, d port.Decision) (world, string, bool) {
	rec, grantRow, err := approval.Decide(s.toApproval(w.Records[i], i), approverIDs[a], w.Role[a], d, "", "test", s.limits(), t0)
	if err != nil {
		return w, "", false
	}
	tag := ""
	w.Records[i] = from(w.Records[i], rec)
	if grantRow || (s.approveWritesGrant && s.askOnce && d == port.DecisionApprove) {
		w.SessionGrant = true
	}
	if d != port.DecisionDeny {
		w.Records[i].Handoff = true
	}
	pause, fired := approval.Brake(port.Pause{Paused: w.Paused, Consecutive: int(w.Denials)}, d, s.limits(), t0, later)
	w.Denials = uint8(pause.Consecutive)
	if fired {
		w.Paused = true
		for k := range w.Records {
			if w.Records[k].State == pending && !s.brakeKeepsPending && s.cancel(&w, k, port.ReasonPaused) {
				tag = tagBrakeCancelsPending
			}
		}
	}
	return w, tag, true
}

func (s spec) next(w world) []edge {
	var out []edge
	add := func(label string, worker bool, to world) {
		out = append(out, edge{label: label, worker: worker, to: to})
	}
	for args := uint8(1); args <= 2; args++ {
		if e, ok := s.call(w, args); ok {
			out = append(out, e)
		}
	}
	l := s.limits()
	for i := range w.Records {
		r := w.Records[i]
		if r.State == none {
			continue
		}
		for a := 0; a < approvers; a++ {
			for k, d := range decisions {
				if to, tag, ok := s.decide(w, i, a, d); ok {
					out = append(out, edge{label: fmt.Sprintf("A%d: approver %d decides %s on record %d", k+3, a, d, i), to: to, tag: tag})
				}
			}
		}
		if r.State == pending {
			to := w
			if a, err := approval.Expire(s.toApproval(r, i), later); err == nil {
				to.Records[i] = from(r, a)
				add(fmt.Sprintf("A6: record %d expires", i), true, to)
			}
			to = w
			if s.cancel(&to, i, "approval_cancel") {
				add(fmt.Sprintf("A7: the agent cancels record %d", i), false, to)
			}
		}
		if r.State == approved {
			to := w
			reason, tag := w.closedBy()
			if s.noRecheck {
				reason = ""
			}
			a, err := approval.Start(s.toApproval(r, i), reason, time.Minute, l, t0)
			if err != nil {
				panic(err)
			}
			to.Records[i] = from(r, a)
			to.Records[i].Handoff = false
			if a.State == port.ApprovalExecuting {
				to.Records[i].Flight = committed
				add(fmt.Sprintf("A8: record %d moves to executing", i), true, to)
			} else {
				out = append(out, edge{label: fmt.Sprintf("A9: the re-check of record %d fails, reason %s", i, reason), worker: true, to: to, tag: tag})
			}
			to = w
			if a, err := approval.Start(s.toApproval(r, i), "", time.Minute, l, later); err == nil && a.Reason == string(port.CodeStaleApproval) {
				to.Records[i] = from(r, a)
				to.Records[i].Handoff = false
				add(fmt.Sprintf("A9: record %d is approved for longer than execute-within", i), true, to)
			}
		}
		if r.State == executing && r.Flight == idle && s.resendAfterCrash {
			to := w
			to.Records[i].Flight = committed
			add(fmt.Sprintf("the worker resumes record %d", i), true, to)
		}
		if r.Flight == committed {
			for other := uint8(1); other <= 2; other++ {
				if other != r.Args && !s.argsFromRequest {
					continue
				}
				to := w
				x := &to.Records[i]
				x.Flight = sent
				if x.Executions < 2 {
					x.Executions++
				}
				x.ExecutedArgs = other
				add(fmt.Sprintf("the engine forwards record %d with arguments %d", i, other), false, to)
			}
			to := w
			to.Records[i].Flight = idle
			if a, err := approval.Finish(s.toApproval(r, i), sealed, true, l, t0); err == nil {
				to.Records[i] = from(to.Records[i], a)
				to.Records[i].GotError = true
				add(fmt.Sprintf("A11: record %d cannot be sent", i), false, to)
			}
		}
		if r.Flight == sent {
			for _, ok := range []bool{true, false} {
				to := w
				to.Records[i].Flight = idle
				if a, err := approval.Finish(s.toApproval(r, i), sealed, !ok, l, t0); err == nil {
					to.Records[i] = from(to.Records[i], a)
					to.Records[i].GotError = !ok
				}
				add(fmt.Sprintf("A10/A11: the connector answers record %d (ok=%v)", i, ok), false, to)
			}
		}
		if r.State == executing {
			to := w
			a, err := approval.Unknown(s.toApproval(r, i), l, later)
			if s.deadlineToFailed {
				a, err = approval.Finish(s.toApproval(r, i), sealed, true, l, later)
			}
			if err != nil {
				panic(err)
			}
			to.Records[i] = from(r, a)
			add(fmt.Sprintf("the call deadline of record %d passes", i), true, to)
		}
		if r.terminal() && !r.Delivered {
			to := w
			if a, err := approval.Deliver(s.toApproval(r, i), t0); err == nil {
				to.Records[i] = from(r, a)
				to.Records[i].Given = true
				add(fmt.Sprintf("A14: the agent is given the answer of record %d (held response or approval_status)", i), false, to)
			}
		}
		if _, due := approval.Due(s.toApproval(r, i), l, later); due && r.terminal() && !(s.retentionSkipUnknown && r.State == outcomeUnknown) {
			to := w
			a, err := approval.Retain(s.toApproval(r, i), later)
			if err != nil {
				panic(err)
			}
			to.Records[i] = from(r, a)
			add(fmt.Sprintf("the result timer of record %d fires", i), true, to)
		}
	}
	for a := 0; a < approvers; a++ {
		if w.Role[a] {
			to := w
			to.Role[a] = false
			add(fmt.Sprintf("approver %d loses the role", a), false, to)
		}
	}
	if !w.SessionEnded {
		to := w
		to.SessionEnded = true
		to.SessionGrant = to.SessionGrant && s.grantSurvivesSession
		for i := range to.Records {
			s.cancel(&to, i, port.ReasonSessionEnded)
		}
		add("A7: the session ends", false, to)
	}
	if !w.Quarantined {
		to := w
		to.Quarantined, to.SessionGrant = true, false
		for i := range to.Records {
			s.cancel(&to, i, port.ReasonQuarantined)
		}
		add("A7: the function is quarantined", false, to)
	}
	if w.Paused && !w.Cleared {
		to := w
		p := approval.Clear(port.Pause{Paused: true, Consecutive: int(w.Denials)})
		to.Paused, to.Denials, to.Cleared = p.Paused, uint8(p.Consecutive), true
		add("clearPause", false, to)
	}
	if int(w.Crashes) < s.crashes {
		to := w
		to.Crashes++
		for i := range to.Records {
			to.Records[i].Handoff = false
			to.Records[i].Flight = idle
		}
		add("the process crashes and restarts", false, to)
	}
	return out
}

type violation struct {
	detail string
	trace  []string
}

type report struct {
	spec        string
	states      int
	transitions int
	violations  map[string]violation
	counts      map[string]int
	seen        map[string]bool
	truncated   bool
}

func (r report) names() []string {
	var out []string
	for k := range r.violations {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func check(w world) map[string]string {
	bad := map[string]string{}
	open := map[uint8]int{}
	for i := range w.Records {
		r := &w.Records[i]
		if r.State == none {
			continue
		}
		if r.open() {
			open[r.Args]++
		}
		if r.Executions > 1 {
			bad["I1 at most one execution per approval"] = fmt.Sprintf("record %d was forwarded %d times", i, r.Executions)
		}
		if r.Executions > 0 && r.ExecutedArgs != r.Args {
			bad["I2 execution only with the approved arguments"] = fmt.Sprintf("record %d approved arguments %d and ran %d", i, r.Args, r.ExecutedArgs)
		}
		if r.Executions > 0 && !r.Decided {
			bad["I2 execution only with the approved arguments"] = fmt.Sprintf("record %d ran with no decision", i)
		}
		if (r.State == denied || r.State == expired || r.State == cancelled) && r.Executions > 0 {
			bad["I5 the recorded state tells the truth about the upstream"] = fmt.Sprintf("record %d is %v and was forwarded", i, states[r.State])
		}
		if r.State == failed && !r.GotError {
			bad["I5 the recorded state tells the truth about the upstream"] = fmt.Sprintf("record %d is failed and no error was received", i)
		}
		if (r.State == denied || r.State == expired || r.State == cancelled) && r.ArgsKept {
			bad["I6 sealed arguments do not outlive a record that will not run"] = fmt.Sprintf("record %d is %v with its arguments", i, states[r.State])
		}
		if r.Delivered && !r.Given {
			bad["I14 the delivered mark is set only when the answer was given to the agent"] = fmt.Sprintf("record %d is marked delivered and its answer was never given", i)
		}
		if w.Paused && r.State == pending {
			bad["I8 a paused session holds no pending approval"] = fmt.Sprintf("record %d is pending under a pause", i)
		}
	}
	if w.SessionGrant && (w.SessionEnded || w.Quarantined) {
		bad["I11 no session grant outlives its session or a quarantine of its function"] = fmt.Sprintf("session ended %v, quarantined %v", w.SessionEnded, w.Quarantined)
	}
	for args, n := range open {
		if n > 1 {
			bad["I7 one open record per session and call hash"] = fmt.Sprintf("%d open records for arguments %d", n, args)
		}
	}
	return bad
}

func checkEdge(from world, e edge) map[string]string {
	bad := map[string]string{}
	started := false
	for i := range e.to.Records {
		started = started || (e.to.Records[i].State == executing && from.Records[i].State != executing)
	}
	if reason, _ := from.closedBy(); started && reason != "" {
		bad["I9 nothing starts executing after the session ended, the function was quarantined or a pause was written"] = fmt.Sprintf("%q with %s", e.label, reason)
	}
	if e.to.SessionGrant && !from.SessionGrant && !strings.Contains(e.label, "approve_for_session") {
		bad["I10 a session grant is written only by A4"] = fmt.Sprintf("%q wrote the session grant", e.label)
	}
	if i := from.notGiven(e.starts); e.starts != 0 && i >= 0 {
		bad["I12 a call whose answer was never delivered is not started again"] = fmt.Sprintf("%q while the answer of record %d was never given", e.label, i)
	}
	direct := e.to.DirectRuns != from.DirectRuns
	if i := from.openFor(e.starts); direct && i >= 0 {
		bad["I13 nothing runs while a record of the same call is open"] = fmt.Sprintf("%q while record %d is open", e.label, i)
	}
	if direct && from.unknownLast(e.starts) {
		bad["I15 a call whose last outcome is unknown is not forwarded on a session grant"] = fmt.Sprintf("%q after an outcome_unknown", e.label)
	}
	return bad
}

func explore(s spec) report {
	rep := report{spec: s.name, violations: map[string]violation{}, counts: map[string]int{}, seen: map[string]bool{}}
	type node struct {
		parent int
		label  string
	}
	index := map[world]int32{start(): 0}
	worlds := []world{start()}
	nodes := []node{{-1, ""}}
	edges := [][]int32{nil}
	workerEdges := [][]int32{nil}
	trace := func(i int) []string {
		var t []string
		for ; nodes[i].parent >= 0; i = nodes[i].parent {
			t = append([]string{nodes[i].label}, t...)
		}
		return t
	}
	note := func(name, detail string, at int) {
		rep.counts[name]++
		if _, ok := rep.violations[name]; !ok {
			rep.violations[name] = violation{detail, trace(at)}
		}
	}
	for head := 0; head < len(worlds); head++ {
		if len(worlds) > maxStates {
			rep.truncated = true
			break
		}
		w := worlds[head]
		for name, detail := range check(w) {
			note(name, detail, head)
		}
		next := s.next(w)
		rep.transitions += len(next)
		for _, e := range next {
			if e.tag != "" {
				rep.seen[e.tag] = true
			}
			for name, detail := range checkEdge(w, e) {
				note(name, detail+" (the trace ends one step before)", head)
			}
			id, ok := index[e.to]
			if !ok {
				id = int32(len(worlds))
				index[e.to] = id
				worlds = append(worlds, e.to)
				nodes = append(nodes, node{head, e.label})
				edges = append(edges, nil)
				workerEdges = append(workerEdges, nil)
			}
			edges[head] = append(edges[head], id)
			if e.worker {
				workerEdges[head] = append(workerEdges[head], id)
			}
		}
	}
	rep.states = len(worlds)
	index = nil
	settles := make([]bool, len(worlds))
	reverse := make([][]int32, len(worlds))
	var queue []int32
	for i, w := range worlds {
		for _, to := range workerEdges[i] {
			reverse[to] = append(reverse[to], int32(i))
		}
		if w.final() && w.quiet() {
			settles[i] = true
			queue = append(queue, int32(i))
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, f := range reverse[i] {
			if !settles[f] {
				settles[f] = true
				queue = append(queue, f)
			}
		}
	}
	for i, w := range worlds {
		if !settles[i] && w.quiet() && len(workerEdges[i]) == 0 {
			note("I3 no state is left without a timer and an owner", "no worker transition settles this state", i)
		}
		if !w.final() && len(edges[i]) == 0 {
			note("I4 every state has an exit", "no transition leaves this state", i)
		}
	}
	return rep
}
