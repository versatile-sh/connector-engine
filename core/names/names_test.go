package names_test

import (
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/core/names"
	"github.com/versatile-sh/connector-engine/port"
)

func TestNames(t *testing.T) {
	if names.ValidAlias("git_hub") == nil || names.ValidAlias("Git") == nil || names.ValidAlias("git-hub2") != nil {
		t.Fatal("alias is [a-z0-9-]")
	}
	if _, err := names.Exposed("a", strings.Repeat("f", 62), names.Separator, names.MaxLength); !port.IsCode(err, port.CodeNameTooLong) {
		t.Fatalf("at most 64 characters: %v", err)
	}
	tb, err := names.Build(names.Separator, names.MaxLength, map[port.ConnectorID]string{"c1": "gh", "c2": "gh-x"}, []names.Entry{
		{Connector: "c2", Function: "a__b"}, {Connector: "c1", Function: "x__a__b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := tb.Resolve("gh__x__a__b"); !ok || e.Connector != "c1" || e.Function != "x__a__b" {
		t.Fatalf("resolved by lookup, never split: %+v", e)
	}
	if got := tb.Names(); len(got) != 2 || got[0] != "gh-x__a__b" {
		t.Fatalf("deterministic order: %v", got)
	}
}

func TestUpstreamNamesStayInsideTheirCharset(t *testing.T) {
	for _, ok := range []string{"search", "repo.list-issues_v2", "prompt:summarize", "res:file:///notes/a.md", "res:https://x.test/items/{id}?q=1", port.InstructionsItem} {
		if err := names.ValidUpstream(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a b", "click [here](https://evil.test)", "<b>x</b>", "tool\nline", "prompt:**urgent**", strings.Repeat("a", 129), "res:https://x.test/[a](b)", "res:a b"} {
		if err := names.ValidUpstream(bad); !port.IsCode(err, port.CodeInvalidManifest) {
			t.Errorf("%q was accepted: %v", bad, err)
		}
	}
}
