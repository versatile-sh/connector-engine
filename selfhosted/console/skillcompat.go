package console

import (
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/skillcompat"
)

// ClientCompat is a skill's derived status in one AI app, with the app's name.
type ClientCompat struct {
	Client  string   `json:"client"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
	Source  string   `json:"source"`
}

// CompatWarning says an AI app that uses this gateway cannot fully use a skill, with the access keys that app connects through.
type CompatWarning struct {
	ClientCompat
	Keys []string `json:"keys"`
}

var appNames = []struct{ match, client string }{
	{"claude code", "claude-code"}, {"claude", "claude-desktop"}, {"codex", "codex-cli"}, {"cursor", "cursor"}, {"gemini", "gemini-cli"},
	{"copilot", "vscode-copilot"}, {"vs code", "vscode-copilot"}, {"visual studio code", "vscode-copilot"}, {"vscode", "vscode-copilot"}, {"opencode", "opencode"}, {"oh-my-pi", "opencode"},
	{"chatgpt", "chat-mcp"}, {"openai", "chat-mcp"},
}

// ClientForName maps an OAuth client name to a catalog client id, and false for an app the table does not know.
func ClientForName(name string) (string, bool) {
	low := strings.ToLower(name)
	for _, a := range appNames {
		if strings.Contains(low, a.match) {
			return a.client, true
		}
	}
	return "", false
}

func (c *Console) clientOfKey(a Agent) (string, bool) {
	if _, ok := skillcompat.Find(a.App); ok {
		return a.App, true
	}
	if cl, ok := c.State.OAuthClient(a.ClientID); ok && a.ClientID != "" {
		return ClientForName(cl.Name)
	}
	return "", false
}

func compatViews(compat []port.SkillCompat) []ClientCompat {
	out := make([]ClientCompat, len(compat))
	for i, e := range compat {
		name := e.Client
		if cl, ok := skillcompat.Find(e.Client); ok {
			name = cl.Name
		}
		out[i] = ClientCompat{Client: e.Client, Name: name, Status: e.Status, Reasons: e.Reasons, Source: e.Source}
	}
	return out
}

// CompatWarnings lists, in table order, each app behind the keys whose status for the skill is not full, with its keys; keyApps maps a key name to its app id.
func CompatWarnings(compat []port.SkillCompat, keyApps map[string]string) []CompatWarning {
	out := []CompatWarning{}
	for _, v := range compatViews(compat) {
		if v.Status == skillcompat.StatusFull {
			continue
		}
		var keys []string
		for key, app := range keyApps {
			if app == v.Client {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		if len(keys) > 0 {
			out = append(out, CompatWarning{ClientCompat: v, Keys: keys})
		}
	}
	return out
}

func (c *Console) appsOf(agents []Agent) map[string]string {
	out := map[string]string{}
	for _, a := range agents {
		if id, ok := c.clientOfKey(a); ok {
			out[a.Name] = id
		}
	}
	return out
}

func installedNeeds(s *port.Skill) (port.SkillNeeds, []port.SkillCompat) {
	if s.Needs != nil && s.Compat != nil {
		return *s.Needs, s.Compat
	}
	paths, content := make([]string, len(s.Files)), map[string]string{}
	for i, f := range s.Files {
		paths[i] = f.Path
		if f.Content != nil {
			content[f.Path] = string(f.Content)
		}
	}
	needs := skillcompat.Analyze(content["SKILL.md"], paths, func(rel string) (string, bool) { t, ok := content[rel]; return t, ok })
	if s.Needs != nil {
		needs = *s.Needs
	}
	return needs, skillcompat.Compat(needs, s.HasScripts)
}
