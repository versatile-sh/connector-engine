package console

import (
	"testing"
	"time"
)

func TestExpireDropsOnlyThePastAndTheWindowCountsOneMinute(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	m := map[string]time.Time{"old": now.Add(-time.Second), "live": now.Add(time.Second), "edge": now}
	expire(m, now, func(v time.Time) time.Time { return v })
	if _, ok := m["old"]; ok || len(m) != 2 {
		t.Fatalf("got %v", m)
	}
	var w minuteWindow
	w.hit(now.Add(-61 * time.Second))
	w.hit(now.Add(-30 * time.Second))
	if w.full(now, 2) || !w.full(now, 1) {
		t.Fatalf("one hit is left in the minute: %v", w.hits)
	}
}
