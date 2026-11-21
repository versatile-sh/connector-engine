package console_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func seed(r *activityRig, n int, tool string, ago time.Duration) []string {
	now := time.Now().UTC()
	ids := make([]string, n)
	for i := range n {
		at := now.Add(-ago - time.Duration(i)*time.Minute)
		ids[i] = fmt.Sprintf("%s-%s%04d", at.Format("20060102"), tool[:2], i)
		r.activity.Record(console.Capture{ID: ids[i], At: at, Done: at.Add(time.Millisecond), Agent: "claude-code", Tool: tool, Outcome: console.OutcomeOK})
	}
	r.activity.Flush()
	return ids
}

func TestTheActivityPagesWithACursorNewestFirst(t *testing.T) {
	r := newActivityRig(t)
	seed(r, 7, "notes__search", time.Minute)
	var seen []string
	query, pages := "?limit=3", 0
	for {
		p := r.page(query)
		pages++
		if p.Total != 7 {
			t.Fatalf("total on page %d: %d", pages, p.Total)
		}
		for _, c := range p.Calls {
			seen = append(seen, c.ID)
		}
		if p.NextCursor == "" {
			break
		}
		query = "?limit=3&cursor=" + p.NextCursor
	}
	all := r.page("?limit=500").Calls
	if pages != 3 || len(seen) != 7 || len(all) != 7 {
		t.Fatalf("pages %d, seen %d, all %d", pages, len(seen), len(all))
	}
	for i, c := range all {
		if seen[i] != c.ID {
			t.Fatalf("page order differs at %d: %s vs %s", i, seen[i], c.ID)
		}
		if i > 0 && c.At.After(all[i-1].At) {
			t.Fatalf("not newest first at %d", i)
		}
	}
}

func TestThePageSizeDefaultsToFiftyAndFiltersNarrowBeforePaging(t *testing.T) {
	r := newActivityRig(t)
	seed(r, 60, "notes__search", time.Minute)
	seed(r, 4, "mail__read", time.Second)
	if p := r.page(""); len(p.Calls) != 50 || p.NextCursor == "" || p.Total != 64 {
		t.Fatalf("default page: %d calls, cursor %q, total %d", len(p.Calls), p.NextCursor, p.Total)
	}
	p := r.page("?connector=c-mail&limit=3")
	if len(p.Calls) != 3 || p.Total != 4 || p.NextCursor == "" {
		t.Fatalf("a filtered first page: %d calls, total %d", len(p.Calls), p.Total)
	}
	rest := r.page("?connector=c-mail&limit=3&cursor=" + p.NextCursor)
	if len(rest.Calls) != 1 || rest.NextCursor != "" || rest.Calls[0].Connector != "c-mail" {
		t.Fatalf("the filtered rest: %+v", rest)
	}
	if got := r.page("?outcome=error"); got.Total != 0 || len(got.Calls) != 0 {
		t.Fatalf("an outcome filter: %+v", got)
	}
}

func TestABadLimitOrCursorIsRefused(t *testing.T) {
	r := newActivityRig(t)
	for _, q := range []string{"?limit=0", "?limit=501", "?limit=x", "?cursor=nope"} {
		if status, _ := r.b.send("GET", "/api/activity"+q, nil, nil); status != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, status)
		}
	}
}

func TestRetentionIsChosenKeptAndAppliedByThePrune(t *testing.T) {
	r := newActivityRig(t)
	old := filepath.Join(r.dir, time.Now().UTC().AddDate(0, 0, -20).Format("2006-01-02")+".jsonl")
	line, _ := json.Marshal(console.Capture{ID: "x-1", At: time.Now().UTC().AddDate(0, 0, -20), Agent: "a", Tool: "notes__search", Outcome: console.OutcomeOK})
	if err := os.WriteFile(old, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	put := func(keep string) (int, map[string]any) {
		status, raw := r.b.send("PUT", "/api/activity/settings", map[string]any{"record_arguments": true, "record_results": false, "retention": keep}, nil)
		var s map[string]any
		_ = json.Unmarshal(raw, &s)
		return status, s
	}
	if status, _ := put("fortnight"); status != http.StatusBadRequest {
		t.Fatalf("an unknown retention: %d", status)
	}
	if status, s := put("7d"); status != http.StatusOK || s["retention"] != "7d" || s["retention_days"] != float64(7) {
		t.Fatalf("7 days: %d %v", status, s)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("a day past the new retention was kept: %v", err)
	}
	if _, s := put("forever"); s["retention"] != "forever" {
		t.Fatalf("forever: %v", s)
	}
	if err := os.WriteFile(old, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	r.activity.Close()
	again, err := console.OpenActivity(r.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err := os.Stat(old); err != nil || again.Settings().Retention != console.KeepForever {
		t.Fatalf("forever prunes nothing and survives a restart: %v %+v", err, again.Settings())
	}
	if err := again.SetRetention(console.KeepOff); err != nil {
		t.Fatal(err)
	}
	again.Record(console.Capture{ID: "x-2", At: time.Now(), Agent: "a", Tool: "notes__search"})
	again.Flush()
	if list, _ := again.Captures(time.Time{}); len(list) != 0 {
		t.Fatalf("off records and keeps nothing: %+v", list)
	}
}

func TestDeletingAPeriodCountsFirstThenRemovesItsCallsAndHidesEngineRows(t *testing.T) {
	r := newActivityRig(t)
	seed(r, 5, "notes__search", 10*time.Minute)
	seed(r, 2, "mail__read", 3*time.Hour)
	r.engine.addCall(port.Call{ID: "k1", Agent: "cursor", Connector: "c-mail", Function: "list", State: port.CallFinished, StartedAt: time.Now().UTC().Add(-3 * time.Hour)})
	span := func(dry bool) map[string]any {
		var out map[string]any
		r.b.ok("POST", "/api/activity/prune", map[string]any{"from": time.Now().Add(-4 * time.Hour), "to": time.Now().Add(-2 * time.Hour), "dry_run": dry}, &out)
		return out
	}
	if out := span(true); out["count"] != float64(3) || out["deleted"] != false {
		t.Fatalf("the dry run: %v", out)
	}
	if p := r.page(""); p.Total != 8 {
		t.Fatalf("a dry run deleted: %d", p.Total)
	}
	if out := span(false); out["count"] != float64(3) || out["deleted"] != true {
		t.Fatalf("the delete: %v", out)
	}
	if p := r.page(""); p.Total != 5 {
		t.Fatalf("after deleting the period: %d", p.Total)
	}
	if status, _ := r.b.send("GET", "/api/activity/e-k1", nil, nil); status != http.StatusNotFound {
		t.Fatalf("a hidden engine row's detail: %d", status)
	}
	if audit := r.auditActions(); len(audit) == 0 || audit[0] != "activity.deleted" {
		t.Fatalf("a deletion is audited: %v", audit)
	}
	var out map[string]any
	r.b.ok("POST", "/api/activity/prune", map[string]any{"older_than_days": 0}, &out)
	if p := r.page(""); p.Total != 0 || out["count"] != float64(5) {
		t.Fatalf("older than 0 days: %d %v", p.Total, out)
	}
}

func TestAPruneNeedsOneClearPeriod(t *testing.T) {
	r := newActivityRig(t)
	for _, body := range []map[string]any{{}, {"older_than_days": -1}, {"older_than_days": 3, "from": time.Now()}, {"from": time.Now(), "to": time.Now().Add(-time.Hour)}} {
		if status, _ := r.b.send("POST", "/api/activity/prune", body, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d", body, status)
		}
	}
	if status, _ := r.b.send("POST", "/api/activity/prune", map[string]any{"older_than_days": 1}, map[string]string{"X-CSRF-Token": "nope"}); status != http.StatusForbidden {
		t.Fatalf("a prune without CSRF: %d", status)
	}
}

func (r *activityRig) auditActions() []string {
	var out []string
	for _, a := range r.state.Audit(10) {
		out = append(out, a.Action)
	}
	return out
}
