package exposure_test

import (
	"testing"

	"github.com/agents-control/connector-engine/core/exposure"
	"github.com/agents-control/connector-engine/port"
)

func flag(b bool) *bool { return &b }

func entry(key string, class port.Class, private, third *bool, open bool) exposure.Entry {
	return exposure.Entry{Key: key, Function: port.Function{Class: class, ReadsPrivate: private, ReturnsThirdParty: third, OpenWorld: open}}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name    string
		entries []exposure.Entry
		exposed bool
		legC    []string
	}{
		{"nothing listed", nil, false, nil},
		{"closed read-only agent", []exposure.Entry{entry("c/get", port.ClassRead, flag(true), flag(false), false)}, false, nil},
		{"all three legs on three functions", []exposure.Entry{
			entry("c/mail", port.ClassRead, flag(true), flag(false), false),
			entry("c/web", port.ClassRead, flag(false), flag(true), false),
			entry("c/send", port.ClassWrite, flag(false), flag(false), false),
		}, true, []string{"c/send"}},
		{"unknown flags count for both legs", []exposure.Entry{entry("c/post", port.ClassWrite, nil, nil, false)}, true, []string{"c/post"}},
		{"an open-world read is a way out", []exposure.Entry{
			entry("c/mail", port.ClassRead, flag(true), flag(true), true),
		}, true, []string{"c/mail"}},
		{"an unclassified function is destructive", []exposure.Entry{entry("c/x", port.ClassNone, flag(true), flag(true), false)}, true, []string{"c/x"}},
		{"private data alone is no trifecta", []exposure.Entry{
			entry("c/mail", port.ClassRead, flag(true), flag(false), false),
			entry("c/send", port.ClassWrite, flag(false), flag(false), false),
		}, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := exposure.Check(c.entries)
			if got.Exposed != c.exposed || len(got.LegC) != len(c.legC) {
				t.Fatalf("got %+v, want exposed=%v legC=%v", got, c.exposed, c.legC)
			}
			for i, k := range c.legC {
				if got.LegC[i] != k {
					t.Fatalf("legC %v, want %v", got.LegC, c.legC)
				}
			}
		})
	}
}

func TestParseKey(t *testing.T) {
	c, fn := exposure.ParseKey(exposure.Key("conn1", "a/b"))
	if c != "conn1" || fn != "a/b" {
		t.Fatalf("got %q %q", c, fn)
	}
}
