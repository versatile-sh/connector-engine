package console_test

import (
	"slices"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestManyFunctionsChangeInOneRequest(t *testing.T) {
	p := profiledStack(t)
	beta := p.mint("beta")
	functions := "/api/connectors/" + p.conn + "/functions"
	profile := "/api/connectors/" + p.conn + "/profiles/beta"

	t.Run("the default sets many functions and answers each", func(t *testing.T) {
		codes := p.batch("PUT", functions, map[string]any{"names": []string{"lookup", "file", "nothing"}, "access": "ask"})
		if codes["lookup"] != "" || codes["file"] != "" || codes["nothing"] != "not_found" {
			t.Fatalf("results %v", codes)
		}
		for _, fn := range p.summary().Functions {
			if fn.Access != console.AccessAsk {
				t.Fatalf("%s is %s", fn.Name, fn.Access)
			}
		}
	})

	t.Run("a key narrows many functions and is refused each one it would widen", func(t *testing.T) {
		p.batch("PUT", functions, map[string]any{"names": []string{"file"}, "access": "on"})
		if codes := p.batch("PUT", profile, map[string]any{"names": []string{"lookup", "file"}, "access": "on"}); codes["lookup"] != "refused" || codes["file"] != "" {
			t.Fatalf("on for beta: %v", codes)
		}
		if codes := p.batch("PUT", profile, map[string]any{"names": []string{"lookup", "file"}, "access": "off"}); codes["lookup"] != "" || codes["file"] != "" {
			t.Fatalf("off for beta: %v", codes)
		}
		if got := toolsOf(t, beta); len(got) != 0 {
			t.Fatalf("beta lists %v", got)
		}
	})

	t.Run("a key's overrides reset to the default in one request", func(t *testing.T) {
		p.batch("PUT", profile, map[string]any{"names": []string{"lookup", "file"}, "reset": true})
		if pr := p.profile("beta"); pr.Mode != console.ProfileFollow {
			t.Fatalf("beta after the reset: %+v", pr)
		}
		if got := toolsOf(t, beta); !slices.Equal(got, []string{"file", "lookup"}) {
			t.Fatalf("beta lists %v", got)
		}
	})

	t.Run("a profile is copied to several keys at once", func(t *testing.T) {
		p.mint("gamma")
		p.mint("delta")
		p.narrow("beta", "lookup", console.AccessOff)
		copyTo := "/api/connectors/" + p.conn + "/profiles:copy"
		if codes := p.batch("POST", copyTo, map[string]any{"from": "beta", "keys": []string{"gamma", "delta", "nobody"}}); codes["gamma"] != "" || codes["delta"] != "" || codes["nobody"] != "not_found" {
			t.Fatalf("copy: %v", codes)
		}
		for _, name := range []string{"gamma", "delta"} {
			if pr := p.profile(name); pr.Mode != console.ProfileCustom || pr.Overrides["lookup"] != console.AccessOff || len(pr.Overrides) != 1 {
				t.Fatalf("%s after the copy: %+v", name, pr)
			}
		}
		p.mode(console.ProfileOff, "beta")
		p.batch("POST", copyTo, map[string]any{"from": "beta", "keys": []string{"gamma"}})
		if pr := p.profile("gamma"); pr.Mode != console.ProfileOff {
			t.Fatalf("gamma after copying an off key: %+v", pr)
		}
	})
}

func TestABatchHasABoundedSize(t *testing.T) {
	p := profiledStack(t)
	names := make([]string, 1001)
	for i := range names {
		names[i] = "lookup"
	}
	for _, path := range []string{"/api/connectors/" + p.conn + "/functions", "/api/connectors/" + p.conn + "/profiles"} {
		if status, raw := p.b.send("PUT", path, map[string]any{"names": names, "keys": names, "access": "ask", "mode": "off"}, nil); status != 400 {
			t.Errorf("%s: %d %s", path, status, raw)
		}
	}
}
