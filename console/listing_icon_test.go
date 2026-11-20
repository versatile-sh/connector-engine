package console_test

import (
	"testing"

	"github.com/agents-control/connector-engine/console"
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
