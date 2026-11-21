package console_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func (r *activityRig) record(id, agent, tool string, ago time.Duration, outcome console.CallOutcome) {
	at := time.Now().UTC().Add(-ago)
	r.activity.Record(console.Capture{ID: id, At: at, Done: at.Add(time.Millisecond), Agent: agent, Tool: tool, Outcome: outcome})
}

func (r *activityRig) summary() console.ActivitySummary {
	r.t.Helper()
	var s console.ActivitySummary
	r.b.ok("GET", "/api/activity/summary", nil, &s)
	return s
}

func TestTheSummaryHoldsTheLastDayAgainstTheDayBeforeAndTheWeekByConnectorAndKey(t *testing.T) {
	r := newActivityRig(t)
	r.record("20261120-n1", "laptop", "notes__search", 2*time.Hour, console.OutcomeOK)
	r.record("20261120-n2", "laptop", "notes__search", 3*time.Hour, console.OutcomeOK)
	r.record("20261120-n3", "phone", "notes__search", 5*time.Hour, console.OutcomeOK)
	r.record("20261120-m1", "laptop", "mail__read", time.Hour, console.OutcomeError)
	r.record("20261119-n4", "phone", "notes__search", 30*time.Hour, console.OutcomeOK)
	r.record("20261118-m2", "phone", "mail__read", 40*time.Hour, console.OutcomeDenied)
	r.record("20261114-m3", "phone", "mail__read", 6*24*time.Hour, console.OutcomeOK)
	r.activity.Flush()
	s := r.summary()

	t.Run("counts the last 24 hours and how many failed", func(t *testing.T) {
		if s.Day.Total != 4 || s.Day.Failed != 1 {
			t.Errorf("the last day: %+v", s.Day)
		}
	})
	t.Run("counts the 24 hours before them and how many were denied or failed", func(t *testing.T) {
		if s.PreviousDay.Total != 2 || s.PreviousDay.Failed != 1 {
			t.Errorf("the day before: %+v", s.PreviousDay)
		}
	})
	t.Run("counts the week and its hours", func(t *testing.T) {
		sum := 0
		for _, h := range s.Hours {
			sum += h.OK + h.Error + h.Denied + h.Pending + h.Running
		}
		if s.Total != 7 || sum != 7 {
			t.Errorf("the week %d, its hours add up to %d", s.Total, sum)
		}
	})
	t.Run("ranks connectors and keys by calls, busiest first", func(t *testing.T) {
		wantConnectors := []console.ConnectorCount{{Connector: "c-notes", Alias: "notes", Total: 4}, {Connector: "c-mail", Alias: "mail", Total: 3, Failed: 2}}
		if !slices.Equal(s.Connectors, wantConnectors) {
			t.Errorf("connectors %+v", s.Connectors)
		}
		wantKeys := []console.KeyCount{{Agent: "laptop", Total: 3, Failed: 1}, {Agent: "phone", Total: 4, Failed: 1}}
		slices.SortFunc(wantKeys, func(x, y console.KeyCount) int { return y.Total - x.Total })
		if !slices.Equal(s.Keys, wantKeys) {
			t.Errorf("keys %+v", s.Keys)
		}
	})
	t.Run("lists the newest calls first", func(t *testing.T) {
		if len(s.Recent) != 7 || s.Recent[0].ID != "20261120-m1" || s.Recent[6].ID != "20261114-m3" {
			t.Errorf("recent %d calls, first %q", len(s.Recent), s.Recent[0].ID)
		}
	})
	t.Run("agrees with the activity page on the last 24 hours", func(t *testing.T) {
		if page := r.page(""); page.Total != s.Day.Total {
			t.Errorf("the activity page counts %d, the summary %d", page.Total, s.Day.Total)
		}
	})
}

func TestTheSummaryKeepsOnlyTheNewestEightCallsAndTheTopTenOfEachRanking(t *testing.T) {
	r := newActivityRig(t)
	for i := range 12 {
		r.record(fmt.Sprintf("20261120-k%02d", i), fmt.Sprintf("key-%02d", i), "notes__search", time.Duration(i+1)*time.Minute, console.OutcomeOK)
	}
	r.activity.Flush()
	s := r.summary()
	if len(s.Recent) != 8 || s.Recent[0].ID != "20261120-k00" || s.Recent[7].ID != "20261120-k07" {
		t.Errorf("recent: %d calls, first %q, last %q", len(s.Recent), s.Recent[0].ID, s.Recent[len(s.Recent)-1].ID)
	}
	if len(s.Keys) != 10 || s.Total != 12 {
		t.Errorf("%d keys of %d calls", len(s.Keys), s.Total)
	}
}

func TestTheSummaryOfAQuietInstallIsEmptyListsNotNulls(t *testing.T) {
	r := newActivityRig(t)
	var raw map[string]any
	r.b.ok("GET", "/api/activity/summary", nil, &raw)
	for _, name := range []string{"connectors", "keys", "recent"} {
		if list, ok := raw[name].([]any); !ok || len(list) != 0 {
			t.Errorf("%s is %v", name, raw[name])
		}
	}
	if raw["total"] != float64(0) {
		t.Errorf("total %v", raw["total"])
	}
}
