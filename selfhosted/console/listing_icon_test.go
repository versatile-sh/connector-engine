package console_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestResolveIcon(t *testing.T) {
	base := "https://catalog.example/connectors/listings/notes.yaml"
	cases := map[string]string{
		"icons/notes.png":               "https://catalog.example/connectors/listings/icons/notes.png",
		"../icons/notes.png":            "https://catalog.example/connectors/icons/notes.png",
		"/static/notes.png":             "https://catalog.example/static/notes.png",
		"https://cdn.example/notes.png": "https://cdn.example/notes.png",
		"data:image/png;base64,AAAA":    "data:image/png;base64,AAAA",
		"":                              "",
	}
	for in, want := range cases {
		if got := console.ResolveIcon(base, in); got != want {
			t.Errorf("ResolveIcon(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAGenericListingNameProposesTheNamespaceAsAlias(t *testing.T) {
	for name, want := range map[string]string{"com.blockscout/mcp-server": "blockscout", "io.github.acme/mcp": "acme", "io.example/server": "example",
		"io.example/mcp-notes": "notes", "io.github.org/github-mcp-server": "github-mcp-server", "notes": "notes", "mcp-server": "server"} {
		if got := console.AliasBase(name); got != want {
			t.Errorf("AliasBase(%q) = %q, want %q", name, got, want)
		}
	}
}
