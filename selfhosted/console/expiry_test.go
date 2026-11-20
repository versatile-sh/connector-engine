package console

import (
	"os"
	"path/filepath"
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

func TestWriteAtomicLeavesOnlyTheOwnerReadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := writeAtomic(path, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	info, err := os.Stat(path)
	raw, _ := os.ReadFile(path)
	if err != nil || len(entries) != 1 || info.Mode().Perm() != 0o600 || string(raw) != `{"a":2}` {
		t.Fatalf("entries %v, mode %v, body %s, %v", entries, info.Mode(), raw, err)
	}
}

func TestASkillSourceLinkIsHTTPSOnly(t *testing.T) {
	for raw, want := range map[string]string{"https://github.com/a/b": "https://github.com/a/b", "javascript:alert(1)": "", "http://x.example": "", "https:relative": "", "": ""} {
		if got := httpsOnly(raw); got != want {
			t.Errorf("%q: got %q", raw, got)
		}
	}
}
