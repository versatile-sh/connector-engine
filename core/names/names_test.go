package names_test

import (
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/port"
)

func TestNames(t *testing.T) {
	if names.ValidAlias("git_hub") == nil || names.ValidAlias("Git") == nil || names.ValidAlias("git-hub2") != nil {
		t.Fatal("§10.1: alias is [a-z0-9-]")
	}
	if _, err := names.Exposed("a", strings.Repeat("f", 62), names.Separator, names.MaxLength); !port.IsCode(err, port.CodeNameTooLong) {
		t.Fatalf("§10.1: at most 64 characters: %v", err)
	}
	tb, err := names.Build(names.Separator, names.MaxLength, map[port.ConnectorID]string{"c1": "gh", "c2": "gh-x"}, []names.Entry{
		{Connector: "c2", Function: "a__b"}, {Connector: "c1", Function: "x__a__b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := tb.Resolve("gh__x__a__b"); !ok || e.Connector != "c1" || e.Function != "x__a__b" {
		t.Fatalf("§10.1: resolved by lookup, never split: %+v", e)
	}
	if got := tb.Names(); len(got) != 2 || got[0] != "gh-x__a__b" {
		t.Fatalf("deterministic order: %v", got)
	}
}
