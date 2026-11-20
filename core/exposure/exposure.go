// Package exposure computes the trifecta check: private data, untrusted content and a way out, over the functions an agent would be listed.
package exposure

import (
	"slices"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// Entry is one function an agent would be listed, under a key that names its connector.
type Entry struct {
	Key      string
	Function port.Function
}

// Key names a function of a connector; the evaluator reads the legs back through ParseKey.
func Key(c port.ConnectorID, function string) string { return string(c) + "/" + function }

// ParseKey splits a key made by Key.
func ParseKey(key string) (port.ConnectorID, string) {
	c, fn, _ := strings.Cut(key, "/")
	return port.ConnectorID(c), fn
}

// Check fills the three legs; an unreviewed flag counts for its leg, the cautious reading.
func Check(entries []Entry) port.Exposure {
	var e port.Exposure
	for _, en := range entries {
		f := en.Function
		if f.ReadsPrivate == nil || *f.ReadsPrivate {
			e.LegA = append(e.LegA, en.Key)
		}
		if f.ReturnsThirdParty == nil || *f.ReturnsThirdParty {
			e.LegB = append(e.LegB, en.Key)
		}
		if f.Class.Effective() != port.ClassRead || f.OpenWorld {
			e.LegC = append(e.LegC, en.Key)
		}
	}
	for _, leg := range [][]string{e.LegA, e.LegB, e.LegC} {
		slices.Sort(leg)
	}
	e.Exposed = len(e.LegA) > 0 && len(e.LegB) > 0 && len(e.LegC) > 0
	if !e.Exposed {
		e.LegC = nil
	}
	return e
}
