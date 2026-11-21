package console

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	summaryRange  = 7 * day
	summaryRecent = 8
	summaryTop    = 10
)

// WindowCount is how many calls a stretch of time held and how many of them failed or were denied.
type WindowCount struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
}

// KeyCount is how many calls one access key made and how many of them failed or were denied.
type KeyCount struct {
	Agent  string `json:"agent"`
	Total  int    `json:"total"`
	Failed int    `json:"failed"`
}

// ActivitySummary is what GET /api/activity/summary answers: the last 24 hours against the 24 before them, and over the last seven days the calls by hour, by connector and by access key plus the newest ones.
type ActivitySummary struct {
	At          time.Time        `json:"at"`
	Total       int              `json:"total"`
	Day         WindowCount      `json:"day"`
	PreviousDay WindowCount      `json:"previous_day"`
	Hours       []HourCount      `json:"hours"`
	Connectors  []ConnectorCount `json:"connectors"`
	Keys        []KeyCount       `json:"keys"`
	Recent      []ActivityCall   `json:"recent"`
}

func (w *WindowCount) add(o CallOutcome) {
	w.Total++
	if failed(o) {
		w.Failed++
	}
}

func summaryOf(rows []joined, from, now time.Time) ActivitySummary {
	first := from.Truncate(time.Hour)
	s := ActivitySummary{At: now, Hours: []HourCount{}, Connectors: []ConnectorCount{}, Keys: []KeyCount{}, Recent: []ActivityCall{}}
	for at := first; !at.After(now); at = at.Add(time.Hour) {
		s.Hours = append(s.Hours, HourCount{Start: at})
	}
	byConn := map[string]*ConnectorCount{}
	byKey := map[string]*KeyCount{}
	for _, j := range rows {
		r := j.row
		if r.At.Before(from) || r.At.After(now) {
			continue
		}
		s.Total++
		if i := int(r.At.Sub(first) / time.Hour); i >= 0 && i < len(s.Hours) {
			s.Hours[i].add(r.Outcome)
		}
		switch {
		case !r.At.Before(now.Add(-day)):
			s.Day.add(r.Outcome)
		case !r.At.Before(now.Add(-2 * day)):
			s.PreviousDay.add(r.Outcome)
		}
		k := firstOf(r.Connector, "alias:"+r.Alias)
		cc := byConn[k]
		if cc == nil {
			cc = &ConnectorCount{Connector: r.Connector, Alias: r.Alias, Skill: r.Skill != ""}
			byConn[k] = cc
		}
		kc := byKey[r.Agent]
		if kc == nil {
			kc = &KeyCount{Agent: r.Agent}
			byKey[r.Agent] = kc
		}
		cc.Total++
		kc.Total++
		if failed(r.Outcome) {
			cc.Failed++
			kc.Failed++
		}
		s.Recent = append(s.Recent, r)
	}
	slices.SortFunc(s.Recent, func(x, y ActivityCall) int { return cmp.Or(y.At.Compare(x.At), strings.Compare(y.ID, x.ID)) })
	s.Recent = s.Recent[:min(len(s.Recent), summaryRecent)]
	for _, cc := range byConn {
		s.Connectors = append(s.Connectors, *cc)
	}
	slices.SortFunc(s.Connectors, func(x, y ConnectorCount) int { return cmp.Or(y.Total-x.Total, strings.Compare(x.Alias, y.Alias)) })
	s.Connectors = s.Connectors[:min(len(s.Connectors), summaryTop)]
	for _, kc := range byKey {
		s.Keys = append(s.Keys, *kc)
	}
	slices.SortFunc(s.Keys, func(x, y KeyCount) int { return cmp.Or(y.Total-x.Total, strings.Compare(x.Agent, y.Agent)) })
	s.Keys = s.Keys[:min(len(s.Keys), summaryTop)]
	return s
}

func (c *Console) activitySummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from := latest(now.Add(-summaryRange), c.Activity.Floor())
	c.Activity.Flush()
	captures, err := c.Activity.Captures(from)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read the activity log")
		return
	}
	facts, err := c.engineFacts(r.Context(), from.Add(-joinSkew), "")
	if err != nil {
		c.fail(w, err)
		return
	}
	rows, err := c.scopedRows(r, captures, facts, "")
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryOf(rows, from, now))
}
