// Package lexical is the ToolIndex that ranks functions by the tokens of their name, title and description, one index per tenant.
package lexical

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	nameWeight   = 3.0
	titleWeight  = 2.0
	bodyWeight   = 1.0
	prefixFactor = 0.5
	minPrefix    = 3
)

type entry struct {
	doc    port.ToolDoc
	fields [3]map[string]bool
}

// Index keeps documents per tenant; a search never reads another tenant's.
type Index struct {
	mu      sync.RWMutex
	tenants map[port.TenantID]map[string]entry
}

// New returns an empty index.
func New() *Index { return &Index{tenants: map[port.TenantID]map[string]entry{}} }

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out[strings.ToLower(string(cur))] = true
			cur = cur[:0]
		}
	}
	var prev rune
	for _, r := range s {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && unicode.IsLower(prev):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
		prev = r
	}
	flush()
	return out
}

// Put adds or replaces the documents of a tenant by name.
func (x *Index) Put(_ context.Context, tenant port.TenantID, docs []port.ToolDoc) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	t := x.tenants[tenant]
	if t == nil {
		t = map[string]entry{}
		x.tenants[tenant] = t
	}
	for _, d := range docs {
		t[d.Name] = entry{doc: d, fields: [3]map[string]bool{tokens(d.Name), tokens(d.Title), tokens(d.Description)}}
	}
	return nil
}

func (e entry) score(q map[string]bool, idf map[string]float64) float64 {
	var total float64
	for i, weight := range [3]float64{nameWeight, titleWeight, bodyWeight} {
		for qt := range q {
			if e.fields[i][qt] {
				total += weight * idf[qt]
				continue
			}
			if len(qt) >= minPrefix {
				for dt := range e.fields[i] {
					if strings.HasPrefix(dt, qt) {
						total += weight * prefixFactor * idf[qt]
						break
					}
				}
			}
		}
	}
	return total
}

// Search returns at most limit documents of the tenant that match the query, best first, among the names in allowed only; none allowed returns none.
func (x *Index) Search(_ context.Context, tenant port.TenantID, query string, allowed []string, limit int) ([]port.ToolDoc, error) {
	q := tokens(query)
	if len(q) == 0 || len(allowed) == 0 {
		return nil, nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	var pool []entry
	for _, name := range allowed {
		if e, ok := x.tenants[tenant][name]; ok {
			pool = append(pool, e)
		}
	}
	idf := map[string]float64{}
	for qt := range q {
		df := 0
		for _, e := range pool {
			if slices.ContainsFunc(e.fields[:], func(f map[string]bool) bool { return f[qt] }) {
				df++
			}
		}
		idf[qt] = math.Log(1 + float64(len(pool)+1)/float64(df+1))
	}
	type hit struct {
		doc   port.ToolDoc
		score float64
	}
	var hits []hit
	for _, e := range pool {
		if s := e.score(q, idf); s > 0 {
			hits = append(hits, hit{e.doc, s})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int {
		if a.score != b.score {
			return int(math.Copysign(1, b.score-a.score))
		}
		return strings.Compare(a.doc.Name, b.doc.Name)
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]port.ToolDoc, len(hits))
	for i, h := range hits {
		out[i] = h.doc
	}
	return out, nil
}
