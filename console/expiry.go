package console

import (
	"maps"
	"slices"
	"time"
)

func expire[K comparable, V any](m map[K]V, now time.Time, until func(V) time.Time) {
	maps.DeleteFunc(m, func(_ K, v V) bool { return now.After(until(v)) })
}

type minuteWindow struct {
	hits []time.Time
}

func (w *minuteWindow) full(now time.Time, limit int) bool {
	w.hits = slices.DeleteFunc(w.hits, func(t time.Time) bool { return now.Sub(t) > time.Minute })
	return len(w.hits) >= limit
}

func (w *minuteWindow) hit(now time.Time) { w.hits = append(w.hits, now) }
