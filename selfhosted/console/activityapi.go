package console

import (
	"cmp"
	"context"
	"encoding/base64"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/core/names"
	"github.com/versatile-sh/connector-engine/port"
)

const (
	activityListMax  = 500
	activityPageDef  = 50
	activityRangeMax = 366 * day
	activityWindow   = 24 * time.Hour
	joinSkew         = time.Second
	engineRowPrefix  = "e-"
)

// ActivityCall is one row of the activity page, with its owner: a captured call joined with the engine's audit row, or an engine row alone.
type ActivityCall struct {
	ID            string      `json:"id"`
	At            time.Time   `json:"at"`
	Agent         string      `json:"agent"`
	Owner         OwnerRef    `json:"owner"`
	Connector     string      `json:"connector,omitempty"`
	Alias         string      `json:"alias,omitempty"`
	Function      string      `json:"function"`
	Skill         string      `json:"skill,omitempty"`
	Path          string      `json:"path,omitempty"`
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
	Tool string `json:"tool,omitempty"`
	CallPayload
	EngineCall string    `json:"engine_call,omitempty"`
	CallState  string    `json:"call_state,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
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
	Skill     bool   `json:"skill,omitempty"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`
}

// ActivityPage is what GET /api/activity answers: one page of the newest calls that pass the filter with the cursor of the next, and the counts by hour, by connector and the apps seen over all of them.
type ActivityPage struct {
	From       time.Time        `json:"from"`
	To         time.Time        `json:"to"`
	Calls      []ActivityCall   `json:"calls"`
	Total      int              `json:"total"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Hours      []HourCount      `json:"hours"`
	Connectors []ConnectorCount `json:"connectors"`
	Agents     []string         `json:"agents"`
}

type activityFilter struct {
	from, to                 time.Time
	connector, agent, search string
	outcome                  CallOutcome
	limit                    int
	after                    *rowCursor
	titles                   map[string]string
}

type rowCursor struct {
	at time.Time
	id string
}

func (c rowCursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.at.UTC().Format(time.RFC3339Nano) + "|" + c.id))
}

func parseRowCursor(s string) (*rowCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	at, id, ok := strings.Cut(string(raw), "|")
	t, terr := time.Parse(time.RFC3339Nano, at)
	if err != nil || !ok || terr != nil || id == "" {
		return nil, invalid("cursor is one the activity gave")
	}
	return &rowCursor{at: t, id: id}, nil
}

func (c rowCursor) before(r ActivityCall) bool {
	return r.At.Before(c.at) || r.At.Equal(c.at) && r.ID < c.id
}

func (f activityFilter) found(r ActivityCall) bool {
	return f.search == "" || slices.ContainsFunc([]string{r.Function, r.Skill, r.Alias, f.titles[r.Alias]}, func(s string) bool { return strings.Contains(strings.ToLower(s), f.search) })
}

func (c *Console) connectorTitles(ctx context.Context) (map[string]string, error) {
	conns, err := c.Engine.connectors(ctx)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	for _, v := range conns {
		if m, err := c.Engine.manifest(ctx, v.Connector); err == nil {
			titles[v.Connector.Alias] = firstOf(m.Title, m.Name)
		}
	}
	return titles, nil
}

type engineFacts struct {
	calls     []port.Call
	approvals map[port.ApprovalID]port.Approval
	byAlias   map[string]string
	byID      map[string]string
	skills    map[string]bool
}

func (c *Console) engineFacts(ctx context.Context, since time.Time, agent string) (engineFacts, error) {
	conns, err := c.Engine.connectors(ctx)
	if err != nil {
		return engineFacts{}, err
	}
	f := engineFacts{approvals: map[port.ApprovalID]port.Approval{}, byAlias: map[string]string{}, byID: map[string]string{}, skills: map[string]bool{}}
	for _, v := range conns {
		f.byID[string(v.Connector.ID)] = v.Connector.Alias
		if _, taken := f.byAlias[v.Connector.Alias]; !taken || v.Connector.RemovedAt.IsZero() {
			f.byAlias[v.Connector.Alias] = string(v.Connector.ID)
		}
	}
	calls, err := list[port.Call](ctx, c.Engine, "/v1/calls"+q("agent", agent, "since", since.UTC().Format(time.RFC3339Nano)))
	if err != nil {
		return engineFacts{}, err
	}
	approvals, err := list[port.Approval](ctx, c.Engine, "/v1/approvals"+q("agent", agent, "since", since.Add(-activityWindow).UTC().Format(time.RFC3339Nano)))
	if err != nil {
		return engineFacts{}, err
	}
	for _, a := range approvals {
		a.Arguments = nil
		f.approvals[a.ID] = a
	}
	f.calls = calls
	for _, call := range f.calls {
		if call.Function == port.SkillFunction {
			f.skills[string(call.Connector)] = false
		}
	}
	for _, v := range conns {
		if _, maybe := f.skills[string(v.Connector.ID)]; maybe {
			skill, err := c.isSkill(ctx, v.Connector)
			f.skills[string(v.Connector.ID)] = skill && err == nil
		}
	}
	return f, nil
}

func firstAgent(agent string, captures []Capture) string {
	if agent == "" && len(captures) > 0 {
		return captures[0].Agent
	}
	return agent
}

type joined struct {
	row     ActivityCall
	capture *Capture
	call    *port.Call
}

type callKey struct{ agent, session, connector, function string }

func earliest(list []*port.Call, cp *Capture, used map[port.CallID]bool) *port.Call {
	var best *port.Call
	for _, call := range list {
		if used[call.ID] || call.StartedAt.Before(cp.At) || call.StartedAt.After(cp.Done.Add(joinSkew)) {
			continue
		}
		if best == nil || call.StartedAt.Before(best.StartedAt) {
			best = call
		}
	}
	return best
}

func join(captures []Capture, f engineFacts, sep string) []joined {
	open := map[callKey][]*port.Call{}
	byApproval := map[port.ApprovalID]*port.Call{}
	for i := range f.calls {
		call := &f.calls[i]
		if call.Approval != "" {
			byApproval[call.Approval] = call
		}
		k := callKey{string(call.Agent), string(call.Session), string(call.Connector), call.Function}
		open[k] = append(open[k], call)
	}
	used := map[port.CallID]bool{}
	out := make([]joined, 0, len(captures)+len(f.calls))
	for i := range captures {
		cp := &captures[i]
		alias, fn, found := strings.Cut(cp.Tool, sep)
		skill := cp.Skill
		switch {
		case cp.Item == port.ItemPrompt && !found && names.ValidAlias(cp.Tool) == nil:
			skill = cp.Tool
			alias, fn = skill, port.SkillFunction
		case skill != "":
			alias, fn = skill, port.SkillFunction
		case cp.Item == port.ItemResource:
			var uri string
			if alias, uri, found = names.SplitResourceURI(cp.Tool); found {
				fn = port.ItemResource + uri
			} else {
				alias, fn = "", cp.Tool
			}
		case !found:
			alias, fn = "", cp.Tool
		case cp.Item == port.ItemPrompt:
			fn = port.ItemPrompt + fn
		}
		conn := f.byAlias[alias]
		var call *port.Call
		switch {
		case cp.Approval != "":
			call = byApproval[port.ApprovalID(cp.Approval)]
		case cp.Outcome == OutcomeOK || cp.Outcome == OutcomeError:
			call = earliest(open[callKey{cp.Agent, cp.Session, conn, fn}], cp, used)
		}
		if call != nil {
			used[call.ID] = true
		}
		row := ActivityCall{ID: cp.ID, At: cp.At, Agent: cp.Agent, Owner: OwnerRef{Person: cp.Person}, Connector: conn, Alias: alias, Function: fn, Skill: skill, Path: cp.Path, Outcome: cp.Outcome, Code: cp.Code, Approval: cp.Approval, Captured: true}
		if cp.Outcome != OutcomePending && !cp.Done.IsZero() {
			ms := cp.Done.Sub(cp.At).Milliseconds()
			row.DurationMS = &ms
		}
		out = append(out, joined{row: f.settle(row, call), capture: cp, call: call})
	}
	for i := range f.calls {
		call := &f.calls[i]
		if !used[call.ID] {
			row := ActivityCall{ID: engineRowPrefix + string(call.ID), At: call.StartedAt, Agent: string(call.Agent), Owner: OwnerRef{Person: string(call.OnBehalfOf)}, Approval: string(call.Approval)}
			out = append(out, joined{row: f.settle(row, call), call: call})
		}
	}
	return out
}

func (f engineFacts) settle(r ActivityCall, call *port.Call) ActivityCall {
	if call != nil {
		r.Connector, r.Alias, r.Function, r.Class = string(call.Connector), f.byID[string(call.Connector)], call.Function, call.Class
		if f.skills[r.Connector] {
			r.Skill = r.Alias
		}
		if r.Approval == "" {
			r.Approval = string(call.Approval)
		}
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
		if r.At.Before(f.from) || r.At.After(f.to) || !f.found(r) {
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
				cc = &ConnectorCount{Connector: r.Connector, Alias: r.Alias, Skill: r.Skill != ""}
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
	slices.SortFunc(p.Calls, func(x, y ActivityCall) int { return cmp.Or(y.At.Compare(x.At), strings.Compare(y.ID, x.ID)) })
	if f.after != nil {
		p.Calls = slices.DeleteFunc(p.Calls, func(r ActivityCall) bool { return !f.after.before(r) })
	}
	if len(p.Calls) > f.limit {
		last := p.Calls[f.limit-1]
		p.Calls = p.Calls[:f.limit]
		p.NextCursor = rowCursor{at: last.At, id: last.ID}.String()
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

var activityParams = []string{"from", "to", "connector", "agent", "search", "outcome", "scope", "person", "limit", "cursor"}

func parseActivityFilter(r *http.Request, floor time.Time) (activityFilter, error) {
	qv := r.URL.Query()
	for name := range qv {
		if !slices.Contains(activityParams, name) {
			return activityFilter{}, invalid("unknown parameter %q: the activity takes %s", name, strings.Join(activityParams, ", "))
		}
	}
	now := time.Now().UTC()
	f := activityFilter{from: now.Add(-activityWindow), to: now, connector: qv.Get("connector"), agent: qv.Get("agent"), search: strings.ToLower(strings.TrimSpace(qv.Get("search"))), outcome: CallOutcome(qv.Get("outcome"))}
	f.limit = activityPageDef
	if v := qv.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > activityListMax {
			return f, invalid("limit is a number from 1 to %d", activityListMax)
		}
		f.limit = n
	}
	if v := qv.Get("cursor"); v != "" {
		cur, err := parseRowCursor(v)
		if err != nil {
			return f, err
		}
		f.after = cur
	}
	for name, into := range map[string]*time.Time{"from": &f.from, "to": &f.to} {
		if v := qv.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return f, invalid("%s is an RFC 3339 time", name)
			}
			*into = t.UTC()
		}
	}
	if f.to.Before(f.from) {
		return f, invalid("from is before to")
	}
	if f.to.Sub(f.from) > activityRangeMax {
		return f, invalid("the range is at most %d days", activityRangeMax/day)
	}
	switch f.outcome {
	case "", OutcomeOK, OutcomeError, OutcomeDenied, OutcomePending, OutcomeRunning:
	default:
		return f, invalid("outcome is ok, error, denied, pending or running")
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

func (c *Console) withActivity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.Activity == nil {
			writeError(w, http.StatusServiceUnavailable, "this console keeps no activity")
			return
		}
		next(w, r)
	}
}

func (c *Console) listActivity(w http.ResponseWriter, r *http.Request) {
	f, err := parseActivityFilter(r, c.Activity.Floor())
	if err != nil {
		c.fail(w, err)
		return
	}
	c.Activity.Flush()
	captures, err := c.Activity.Captures(f.from)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read the activity log")
		return
	}
	facts, err := c.engineFacts(r.Context(), f.from.Add(-joinSkew), f.agent)
	if err == nil && f.search != "" {
		f.titles, err = c.connectorTitles(r.Context())
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	all, err := everyone(r, CapActivityAny)
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	person := r.URL.Query().Get("person")
	rows := c.owned(join(captures, facts, c.separator(r.Context(), firstAgent(f.agent, captures))))
	rows = slices.DeleteFunc(rows, func(j joined) bool {
		p := j.row.Owner.Person
		return !all && p != u.Person || all && person != "" && p != person || c.Activity.Hidden(j.row.At)
	})
	writeJSON(w, http.StatusOK, pageOf(rows, f))
}

func (c *Console) owned(rows []joined) []joined {
	for i := range rows {
		rows[i].row.Owner = c.ownerRef(rows[i].row.Owner.Person)
	}
	return rows
}

func detailOf(j joined) ActivityDetail {
	d := ActivityDetail{ActivityCall: j.row}
	if cp := j.capture; cp != nil {
		d.Tool, d.CallPayload = cp.Tool, cp.CallPayload
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
	id := r.PathValue("id")
	floor := c.Activity.Floor()
	var captures []Capture
	var since time.Time
	var agent string
	if engineID, ok := strings.CutPrefix(id, engineRowPrefix); ok {
		var call port.Call
		if err := c.Engine.Read(r.Context(), http.MethodGet, "/v1/calls/"+esc(engineID), nil, &call); err != nil {
			c.fail(w, err)
			return
		}
		since, agent = call.StartedAt, string(call.Agent)
	} else {
		c.Activity.Flush()
		cp, ok, err := c.Activity.Capture(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cannot read the activity log")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "no such call in the activity")
			return
		}
		captures, since, agent = []Capture{cp}, cp.At.Add(-joinSkew), cp.Agent
	}
	if since.Before(floor) {
		writeError(w, http.StatusNotFound, "no such call in the activity")
		return
	}
	facts, err := c.engineFacts(r.Context(), since, agent)
	if err != nil {
		c.fail(w, err)
		return
	}
	u, _ := signedInUser(r.Context())
	for _, j := range c.owned(join(captures, facts, c.separator(r.Context(), agent))) {
		if j.row.ID == id && !c.Activity.Hidden(j.row.At) && may(u, CapActivityOwn, CapActivityAny, j.row.Owner.Person) {
			writeJSON(w, http.StatusOK, detailOf(j))
			return
		}
	}
	writeError(w, http.StatusNotFound, "no such call in the activity")
}

type activitySettingsView struct {
	RecordArguments bool      `json:"record_arguments"`
	RecordResults   bool      `json:"record_results"`
	Retention       Keep      `json:"retention"`
	RetentionDays   int       `json:"retention_days"`
	ClearedAt       time.Time `json:"cleared_at,omitzero"`
}

func (c *Console) activitySettings(w http.ResponseWriter, _ *http.Request) {
	s := c.Activity.Settings()
	writeJSON(w, http.StatusOK, activitySettingsView{RecordArguments: s.RecordArguments, RecordResults: s.RecordResults, Retention: s.Retention.or(), RetentionDays: s.Retention.days(), ClearedAt: s.ClearedAt})
}

func (c *Console) setActivitySettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RecordArguments *bool `json:"record_arguments"`
		RecordResults   *bool `json:"record_results"`
		Retention       *Keep `json:"retention"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.RecordArguments == nil || in.RecordResults == nil {
		writeError(w, http.StatusBadRequest, "give record_arguments and record_results")
		return
	}
	if in.Retention != nil && !slices.Contains(Keeps, *in.Retention) {
		writeError(w, http.StatusBadRequest, "retention is off, 7d, 30d, 90d or forever")
		return
	}
	err := c.Activity.SetRecording(*in.RecordArguments, *in.RecordResults)
	if err == nil && in.Retention != nil {
		err = c.Activity.SetRetention(*in.Retention)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot save the activity settings")
		return
	}
	c.activitySettings(w, r)
}

func (c *Console) deleteActivity(w http.ResponseWriter, _ *http.Request) {
	if err := c.Activity.DeleteAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot delete the activity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PruneResult is what POST /api/activity/prune answers: how many calls the period holds and whether they were deleted or only counted.
type PruneResult struct {
	Count   int  `json:"count"`
	Deleted bool `json:"deleted"`
}

type pruneRequest struct {
	From      *time.Time `json:"from"`
	To        *time.Time `json:"to"`
	OlderThan *int       `json:"older_than_days"`
	DryRun    bool       `json:"dry_run"`
}

func (in pruneRequest) span(now time.Time) (from, to time.Time, err error) {
	switch {
	case in.OlderThan != nil && (in.From != nil || in.To != nil):
		return from, to, invalid("give older_than_days or from and to, not both")
	case in.OlderThan != nil && *in.OlderThan >= 0:
		return from, now.Add(-time.Duration(*in.OlderThan) * day), nil
	case in.From != nil && in.To != nil && !in.To.Before(*in.From):
		return in.From.UTC(), in.To.UTC(), nil
	}
	return from, to, invalid("give older_than_days, or from and to with from not after to")
}

func (c *Console) pruneActivity(w http.ResponseWriter, r *http.Request) {
	var in pruneRequest
	if !readJSON(w, r, &in) {
		return
	}
	now := time.Now().UTC()
	from, to, err := in.span(now)
	if err != nil {
		c.fail(w, err)
		return
	}
	c.Activity.Flush()
	f := activityFilter{from: latest(from, c.Activity.Floor()), to: to, limit: 1}
	captures, err := c.Activity.Captures(f.from)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read the activity log")
		return
	}
	facts, err := c.engineFacts(r.Context(), f.from.Add(-joinSkew), "")
	if err != nil {
		c.fail(w, err)
		return
	}
	rows := c.owned(join(captures, facts, c.separator(r.Context(), firstAgent("", captures))))
	rows = slices.DeleteFunc(rows, func(j joined) bool {
		return j.row.At.After(to) || j.row.At.Before(f.from) || c.Activity.Hidden(j.row.At)
	})
	count := len(rows)
	if !in.DryRun {
		if _, err := c.Activity.DeleteRange(from, to); err != nil {
			writeError(w, http.StatusInternalServerError, "cannot delete the activity")
			return
		}
	}
	writeJSON(w, http.StatusOK, PruneResult{Count: count, Deleted: !in.DryRun})
}
