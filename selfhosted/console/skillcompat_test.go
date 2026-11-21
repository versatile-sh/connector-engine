package console_test

import (
	"reflect"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestClientForName(t *testing.T) {
	for name, want := range map[string]string{"Claude Code": "claude-code", "Claude": "claude-desktop", "Gemini CLI": "gemini-cli", "Codex": "codex-cli", "Visual Studio Code": "vscode-copilot", "ChatGPT": "chat-mcp"} {
		if got, ok := console.ClientForName(name); !ok || got != want {
			t.Errorf("%s: got %q %v, want %q", name, got, ok, want)
		}
	}
	if got, ok := console.ClientForName("my-script"); ok {
		t.Errorf("an unknown app mapped to %q", got)
	}
}

func TestCompatWarnings(t *testing.T) {
	scripts := []port.SkillCompat{
		{Client: "claude-code", Status: "full", Reasons: []string{"ok"}, Source: "derived"},
		{Client: "gemini-cli", Status: "full", Reasons: []string{"ok"}, Source: "derived"},
		{Client: "claude-desktop", Status: "no", Reasons: []string{"runs-bundled-code"}, Source: "derived"},
		{Client: "chat-mcp", Status: "instructions", Reasons: []string{"no-web"}, Source: "derived"},
	}
	apps := map[string]string{"work": "claude-desktop", "home": "claude-desktop", "cli": "claude-code", "chat": "chat-mcp"}
	t.Run("an app that cannot use the skill is named once with all its keys", func(t *testing.T) {
		got := console.CompatWarnings(scripts, apps)
		if len(got) != 2 || got[0].Client != "claude-desktop" || got[0].Name != "Claude Desktop" || got[0].Status != "no" || !reflect.DeepEqual(got[0].Keys, []string{"home", "work"}) {
			t.Fatalf("%+v", got)
		}
		if got[1].Client != "chat-mcp" || got[1].Status != "instructions" || got[1].Reasons[0] != "no-web" {
			t.Fatalf("%+v", got[1])
		}
	})
	t.Run("apps that run the skill, or are unknown, raise nothing", func(t *testing.T) {
		if got := console.CompatWarnings(scripts, map[string]string{"cli": "claude-code"}); len(got) != 0 {
			t.Fatalf("%+v", got)
		}
		if got := console.CompatWarnings(scripts, nil); got == nil || len(got) != 0 {
			t.Fatalf("%#v", got)
		}
	})
}
