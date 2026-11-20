package console

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/port"
)

const (
	activityListMax = 500
	activityWindow  = 24 * time.Hour
	joinSkew        = time.Second
	engineRowPrefix = "e-"
)

// ActivityCall is one row of the activity page: a captured call joined with the engine's audit row, or an engine row alone.
type ActivityCall struct {
	ID            string      `json:"id"`
	At            time.Time   `json:"at"`
	Agent         string      `json:"agent"`
	Connector     string      `json:"connector,omitempty"`
	Alias         string      `json:"alias,omitempty"`
	Function      string      `json:"function"`
	Class         port.Class  `json:"class,omitempty"`
	Outcome       CallOutcome `json:"outcome"`
	Code          string      `json:"code,omitempty"`
	DurationMS    *int64      `json:"duration_ms,omitempty"`
	Approval      string      `json:"approval,omitempty"`
	ApprovalState string      `json:"approval_state,omitempty"`
	Captured      bool        `json:"captured"`
}

// ActivityDetail is one call in full: its row, what the console captured of it and the engine's record.
type ActivityDetail struct {
	ActivityCall
	Tool          string          `json:"tool,omitempty"`
	Session       string          `json:"session,omitempty"`
	MCPSession    string          `json:"mcp_session,omitempty"`
	RPCID         json.RawMessage `json:"rpc_id,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	KeptArguments bool            `json:"kept_arguments"`
	Arguments     string          `json:"arguments,omitempty"`
	ArgumentsCut  bool            `json:"arguments_cut,omitempty"`
	KeptResult    bool            `json:"kept_result"`
	Result        string          `json:"result,omitempty"`
	ResultCut     bool            `json:"result_cut,omitempty"`
	ResultBytes   int             `json:"result_bytes,omitempty"`
	EngineCall    string          `json:"engine_call,omitempty"`
	CallState     string          `json:"call_state,omitempty"`
	FinishedAt    time.Time       `json:"finished_at,omitzero"`
}

// HourCount is how many calls started in one UTC hour, by outcome.
type HourCount struct {
	Start   time.Time `json:"start"`
	OK      int       `json:"ok"`
	Error   int       `json:"error"`
	Denied  int       `json:"denied"`
	Pending int       `json:"pending"`
	Running int       `json:"running"`
}

// ConnectorCount is how many calls reached one connector and how many of them failed or were denied.
type ConnectorCount struct {
	Connector string `json:"connector,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`
}

// ActivityPage is what GET /api/activity answers: the newest calls that pass the filter, their count by hour, by connector and the apps seen.
type ActivityPage struct {
	From       time.Time        `json:"from"`
	To         time.Time        `json:"to"`
	Calls      []ActivityCall   `json:"calls"`
	Total      int              `json:"total"`
	Hours      []HourCount      `json:"hours"`
	Connectors []ConnectorCount `json:"connectors"`
	Agents     []string         `json:"agents"`
}

type activityFilter struct {
	from, to         time.Time
	connector, agent string
	outcome          CallOutcome
}

type itemStream[T any] struct {
	keep  func(T) bool
	items []T
}

func (s *itemStream[T]) readBody(r io.Reader) error {
	d := json.NewDecoder(r)
	if tok, err := d.Token(); err != nil || tok != json.Delim('{') {
		return errors.Join(errors.New("the engine answered no object"), err)
	}
	for d.More() {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		if tok != "items" {
			var skip json.RawMessage
			if err := d.Decode(&skip); err != nil {
				return err
			}
			continue
		}
		if tok, err = d.Token(); err != nil || tok == nil {
			return err
		}
		if tok != json.Delim('[') {
			return errors.New("the engine answered items that are not a list")
		}
		for d.More() {
			var v T
			if err := d.Decode(&v); err != nil {
				return err
			}
			if s.keep(v) {
				s.items = append(s.items, v)
			}
		}
		if _, err := d.Token(); err != nil {
			return err
		}
	}
	return nil
}

type engineFacts struct {
	calls     []port.Call
	approvals map[port.ApprovalID]port.Approval
	byAlias   map[string]string
	byID      map[string]string
}

func (c *Console) engineFacts(ctx context.Context, since time.Time, agent string) (engineFacts, error) {
	var conns items[connectorView]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/connectors"+q("workspace", c.Engine.Workspace), nil, &conns); err != nil {
		return engineFacts{}, err
	}
	f := engineFacts{approvals: map[port.ApprovalID]port.Approval{}, byAlias: map[string]string{}, byID: map[string]string{}}
	for _, v := range conns.Items {
		f.byID[string(v.Connector.ID)] = v.Connector.Alias
		if _, taken := f.byAlias[v.Connector.Alias]; !taken || v.Connector.RemovedAt.IsZero() {
			f.byAlias[v.Connector.Alias] = string(v.Connector.ID)
		}
	}
	calls := &itemStream[port.Call]{keep: func(x port.Call) bool { return !x.StartedAt.Before(since) }}
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/calls"+q("agent", agent), nil, calls); err != nil {
		return engineFacts{}, err
	}
	approvals := &itemStream[port.Approval]{keep: func(x port.Approval) bool { return !x.CreatedAt.Before(since.Add(-activityWindow)) }}
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/approvals"+q("agent", agent), nil, approvals); err != nil {
		return engineFacts{}, err
	}
	for _, a := range approvals.items {
		a.Arguments = nil
		f.approvals[a.ID] = a
	}
	f.calls = calls.items
	return f, nil
}

type joined struct {
	row     ActivityCall
	capture *Capture
	call    *port.Call
}

type callKey struct{ agent, session, connector, function string }

func closest(list []*port.Call, cp *Capture, used map[port.CallID]bool) *port.Call {
	var best *port.Call
	var gap time.Duration
	for _, call := range list {
		if used[call.ID] || call.StartedAt.Before(cp.At.Add(-joinSkew)) || call.StartedAt.After(cp.Done.Add(joinSkew)) {
			continue
		}
		d := call.StartedAt.Sub(cp.At).Abs()
		if best == nil || d < gap {
			best, gap = call, d
		}
	}
	return best
}

func join(captures []Capture, f engineFacts) []joined {
	open := map[callKey][]*port.Call{}
	byApproval := map[port.ApprovalID]*port.Call{}
	for i := range f.calls {
		call := &f.calls[i]
		if call.Approval != "" {
			byApproval[call.Approval] = call
			continue
		}
		k := callKey{string(call.Agent), string(call.Session), string(call.Connector), call.Function}
		open[k] = append(open[k], call)
	}
	used := map[port.CallID]bool{}
	out := make([]joined, 0, len(captures)+len(f.calls))
	for i := range captures {
		cp := &captures[i]
		alias, fn, _ := strings.Cut(cp.Tool, toolSeparator)
		conn := f.byAlias[alias]
		var call *port.Call
		switch {
		case cp.Approval != "":
			call = byApproval[port.ApprovalID(cp.Approval)]
		case cp.Outcome == OutcomeOK || cp.Outcome == OutcomeError:
			call = closest(open[callKey{cp.Agent, cp.Session, conn, fn}], cp, used)
		}
		if call != nil {
			used[call.ID] = true
		}
		row := ActivityCall{ID: cp.ID, At: cp.At, Agent: cp.Agent, Connector: conn, Alias: alias, Function: firstOf(fn, cp.Tool), Outcome: cp.Outcome, Code: cp.Code, Approval: cp.Approval, Captured: true}
		if cp.Outcome != OutcomePending && !cp.Done.IsZero() {
			ms := cp.Done.Sub(cp.At).Milliseconds()
			row.DurationMS = &ms
		}
		out = append(out, joined{row: f.settle(row, call), capture: cp, call: call})
	}
	for i := range f.calls {
		call := &f.calls[i]
		if !used[call.ID] {
			row := ActivityCall{ID: engineRowPrefix + string(call.ID), At: call.StartedAt, Agent: string(call.Agent), Approval: string(call.Approval)}
			out = append(out, joined{row: f.settle(row, call), call: call})
		}
	}
	return out
}

func (f engineFacts) settle(r ActivityCall, call *port.Call) ActivityCall {
	if call != nil {
		r.Connector, r.Alias, r.Function, r.Class = string(call.Connector), f.byID[string(call.Connector)], call.Function, call.Class
		r.DurationMS = nil
		switch {
		case call.State == port.CallStarted:
			r.Outcome, r.Code = OutcomeRunning, ""
		case call.State == port.CallFinished && !call.IsError:
			r.Outcome, r.Code = OutcomeOK, ""
		default:
			r.Outcome, r.Code = OutcomeError, firstOf(call.Reason, string(call.State))
		}
		if !call.FinishedAt.IsZero() {
			ms := call.FinishedAt.Sub(call.StartedAt).Milliseconds()
			r.DurationMS = &ms
		}
	}
	a, ok := f.approvals[port.ApprovalID(r.Approval)]
	if r.Approval == "" || !ok {
		return r
	}
	r.ApprovalState = string(a.State)
	if r.Class == "" {
		r.Class = a.Class
	}
	if call != nil {
		return r
	}
	switch a.State {
	case port.ApprovalPending:
		r.Outcome, r.Code = OutcomePending, ""
	case port.ApprovalDenied:
		r.Outcome, r.Code = OutcomeDenied, "approval_denied"
	case port.ApprovalExpired, port.ApprovalCancelled:
		r.Outcome, r.Code = OutcomeError, "approval_"+string(a.State)
	case port.ApprovalApproved, port.ApprovalExecuting:
		r.Outcome, r.Code = OutcomeRunning, ""
	case port.ApprovalFailed:
		r.Outcome, r.Code = OutcomeError, firstOf(r.Code, "approval_failed")
	}
	return r
}

func failed(o CallOutcome) bool { return o == OutcomeError || o == OutcomeDenied }

func (h *HourCount) add(o CallOutcome) {
	switch o {
	case OutcomeOK:
		h.OK++
	case OutcomeError:
		h.Error++
	case OutcomeDenied:
		h.Denied++
	case OutcomePending:
		h.Pending++
	case OutcomeRunning:
		h.Running++
	}
}

func pageOf(rows []joined, f activityFilter) ActivityPage {
	first := f.from.Truncate(time.Hour)
	p := ActivityPage{From: f.from, To: f.to, Calls: []ActivityCall{}, Connectors: []ConnectorCount{}, Agents: []string{}}
	for at := first; !at.After(f.to); at = at.Add(time.Hour) {
		p.Hours = append(p.Hours, HourCount{Start: at})
	}
	byConn := map[string]*ConnectorCount{}
	for _, j := range rows {
		r := j.row
		if r.At.Before(f.from) || r.At.After(f.to) {
			continue
		}
		agentOK := f.agent == "" || r.Agent == f.agent
		connOK := f.connector == "" || r.Connector == f.connector
		outcomeOK := f.outcome == "" || r.Outcome == f.outcome
		if connOK && outcomeOK && !slices.Contains(p.Agents, r.Agent) {
			p.Agents = append(p.Agents, r.Agent)
		}
		if agentOK && outcomeOK {
			k := firstOf(r.Connector, "alias:"+r.Alias)
			cc := byConn[k]
			if cc == nil {
				cc = &ConnectorCount{Connector: r.Connector, Alias: r.Alias}
				byConn[k] = cc
			}
			cc.Total++
			if failed(r.Outcome) {
				cc.Failed++
			}
		}
		if !agentOK || !connOK || !outcomeOK {
			continue
		}
		p.Total++
		if i := int(r.At.Sub(first) / time.Hour); i >= 0 && i < len(p.Hours) {
			p.Hours[i].add(r.Outcome)
		}
		p.Calls = append(p.Calls, r)
	}
	slices.SortStableFunc(p.Calls, func(x, y ActivityCall) int { return y.At.Compare(x.At) })
	if len(p.Calls) > activityListMax {
		p.Calls = p.Calls[:activityListMax]
	}
	for _, cc := range byConn {
		p.Connectors = append(p.Connectors, *cc)
	}
	slices.SortFunc(p.Connectors, func(x, y ConnectorCount) int {
		if x.Total != y.Total {
			return y.Total - x.Total
		}
		return strings.Compare(x.Alias, y.Alias)
	})
	slices.Sort(p.Agents)
	return p
}

func parseActivityFilter(r *http.Request, floor time.Time) (activityFilter, error) {
	qv := r.URL.Query()
	now := time.Now().UTC()
	f := activityFilter{from: now.Add(-activityWindow), to: now, connector: qv.Get("connector"), agent: qv.Get("agent"), outcome: CallOutcome(qv.Get("outcome"))}
	for name, into := range map[string]*time.Time{"from": &f.from, "to": &f.to} {
		if v := qv.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return f, errors.New(name + " is an RFC 3339 time")
			}
			*into = t.UTC()
		}
	}
	if f.to.Before(f.from) {
		return f, errors.New("from is before to")
	}
	if f.to.Sub(f.from) > activityRetention+time.Hour {
		return f, errors.New("the range is at most 30 days")
	}
	switch f.outcome {
	case "", OutcomeOK, OutcomeError, OutcomeDenied, OutcomePending, OutcomeRunning:
	default:
		return f, errors.New("outcome is ok, error, denied, pending or running")
	}
	f.from = latest(f.from, floor)
	f.to = latest(f.to, f.from)
	return f, nil
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (c *Console) activityOff(w http.ResponseWriter) bool {
	if c.Activity == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "this console keeps no activity"})
		return true
	}
	return false
}

func (c *Console) listActivity(w http.ResponseWriter, r *http.Request) {
	if c.activityOff(w) {
		return
	}
	f, err := parseActivityFilter(r, c.Activity.Floor())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.Activity.Flush()
	captures, err := c.Activity.Captures(f.from)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot read the activity log"})
		return
	}
	facts, err := c.engineFacts(r.Context(), f.from.Add(-joinSkew), f.agent)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pageOf(join(captures, facts), f))
}

func detailOf(j joined) ActivityDetail {
	d := ActivityDetail{ActivityCall: j.row}
	if cp := j.capture; cp != nil {
		d.Tool, d.Session, d.MCPSession, d.RPCID, d.Detail = cp.Tool, cp.Session, cp.MCPSession, cp.RPCID, cp.Detail
		d.KeptArguments, d.Arguments, d.ArgumentsCut = cp.KeptArguments, cp.Arguments, cp.ArgumentsCut
		d.KeptResult, d.Result, d.ResultCut, d.ResultBytes = cp.KeptResult, cp.Result, cp.ResultCut, cp.ResultBytes
	}
	if call := j.call; call != nil {
		d.EngineCall, d.CallState, d.FinishedAt, d.Session = string(call.ID), string(call.State), call.FinishedAt, string(call.Session)
		if call.ResultBytes > 0 {
			d.ResultBytes = call.ResultBytes
		}
		if call.Reason != "" && d.Detail == "" {
			d.Detail = call.Reason
		}
	}
	return d
}

func (c *Console) activityDetail(w http.ResponseWriter, r *http.Request) {
	if c.activityOff(w) {
		return
	}
	id := r.PathValue("id")
	floor := c.Activity.Floor()
	var captures []Capture
	var since time.Time
	var agent string
	if engineID, ok := strings.CutPrefix(id, engineRowPrefix); ok {
		var call port.Call
		if err := c.Engine.Call(r.Context(), http.MethodGet, "/v1/calls/"+esc(engineID), nil, &call); err != nil {
			c.fail(w, err)
			return
		}
		since, agent = call.StartedAt, string(call.Agent)
	} else {
		c.Activity.Flush()
		cp, ok, err := c.Activity.Capture(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot read the activity log"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such call in the activity"})
			return
		}
		captures, since, agent = []Capture{cp}, cp.At.Add(-joinSkew), cp.Agent
	}
	if since.Before(floor) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such call in the activity"})
		return
	}
	facts, err := c.engineFacts(r.Context(), since, agent)
	if err != nil {
		c.fail(w, err)
		return
	}
	for _, j := range join(captures, facts) {
		if j.row.ID == id {
			writeJSON(w, http.StatusOK, detailOf(j))
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such call in the activity"})
}

type activitySettingsView struct {
	RecordArguments bool      `json:"record_arguments"`
	RecordResults   bool      `json:"record_results"`
	RetentionDays   int       `json:"retention_days"`
	ClearedAt       time.Time `json:"cleared_at,omitzero"`
}

func (c *Console) activitySettings(w http.ResponseWriter, _ *http.Request) {
	if c.activityOff(w) {
		return
	}
	s := c.Activity.Settings()
	writeJSON(w, http.StatusOK, activitySettingsView{RecordArguments: s.RecordArguments, RecordResults: s.RecordResults, RetentionDays: int(activityRetention / (24 * time.Hour)), ClearedAt: s.ClearedAt})
}

func (c *Console) setActivitySettings(w http.ResponseWriter, r *http.Request) {
	if c.activityOff(w) {
		return
	}
	var in struct {
		RecordArguments *bool `json:"record_arguments"`
		RecordResults   *bool `json:"record_results"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.RecordArguments == nil || in.RecordResults == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "give record_arguments and record_results"})
		return
	}
	if err := c.Activity.SetRecording(*in.RecordArguments, *in.RecordResults); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot save the activity settings"})
		return
	}
	c.activitySettings(w, r)
}

func (c *Console) deleteActivity(w http.ResponseWriter, _ *http.Request) {
	if c.activityOff(w) {
		return
	}
	if err := c.Activity.DeleteAll(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot delete the activity"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
