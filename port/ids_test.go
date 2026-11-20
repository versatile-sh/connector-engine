package port_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/port"
)

func TestCheckIDs(t *testing.T) {
	for _, c := range []struct {
		kind, id string
		ok       bool
	}{
		{"tenant", "acme-1", true},
		{"tenant", "installation", false},
		{"workspace", "w1/x", false},
		{"agent", "a b", false},
		{"person", "ana@example.com", true},
		{"subject", "auth0|123", true},
		{"on_behalf_of", "ana/x", false},
		{"person", "ana x", false},
	} {
		if err := port.CheckIDs(c.kind, c.id); (err == nil) != c.ok {
			t.Errorf("%s %q: err %v, want ok=%v", c.kind, c.id, err, c.ok)
		}
	}
}
