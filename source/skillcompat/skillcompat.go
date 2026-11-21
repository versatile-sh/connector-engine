// Package skillcompat derives what a skill needs and which AI apps can use it, by the rules of the skills catalog (scripts/compat.py); every value is derived, never tested.
package skillcompat

import (
	"regexp"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
)

// Source marks every compat entry as derived from the skill's files and the client table.
const Source = "derived"

// Status values of a skill in one client.
const (
	StatusFull         = "full"
	StatusInstructions = "instructions"
	StatusNo           = "no"
)

// Reason codes of a status.
const (
	ReasonOK           = "ok"
	ReasonRunsBundled  = "runs-bundled-code"
	ReasonNoShell      = "no-shell"
	ReasonNoFiles      = "no-files"
	ReasonNoWeb        = "no-web"
	maxScriptBytes     = 200_000
	chatClient         = "chat-mcp"
	frontmatterPattern = `(?s)^\x{FEFF}?---[ \t]*\r?\n(.*?)\r?\n---[ \t]*(\r?\n|$)`
)

// Client is one AI app of the rule table with what it can do on its own.
type Client struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	NativeSkills bool   `json:"native_skills"`
	Shell        bool   `json:"shell"`
	Files        bool   `json:"files"`
	Web          bool   `json:"web"`
}

// Clients is the fixed client table, a reviewed snapshot of each client's documentation.
var Clients = []Client{
	{"claude-code", "Claude Code", true, true, true, true},
	{"codex-cli", "Codex CLI", true, true, true, true},
	{"cursor", "Cursor", true, true, true, true},
	{"gemini-cli", "Gemini CLI", true, true, true, true},
	{"vscode-copilot", "VS Code Copilot", true, true, true, true},
	{"opencode", "opencode / oh-my-pi", true, true, true, true},
	{"claude-desktop", "Claude Desktop", true, false, false, false},
	{chatClient, "ChatGPT and other chat-only MCP clients", false, false, false, false},
}

var (
	shellTools  = set("Bash", "BashOutput", "KillShell")
	fileTools   = set("Read", "Write", "Edit", "MultiEdit", "NotebookEdit", "Glob", "Grep", "LS")
	webTools    = set("WebFetch", "WebSearch")
	scriptExt   = map[string]string{"py": "python", "js": "node", "mjs": "node", "cjs": "node", "ts": "node", "sh": "bash", "bash": "bash"}
	commandRuns = map[string]string{
		"python": "python", "python3": "python", "pip": "python", "pip3": "python", "uv": "uv", "uvx": "uv",
		"node": "node", "npm": "node", "npx": "node", "pnpm": "node", "yarn": "node", "bun": "bun", "deno": "deno",
		"docker": "docker", "git": "git", "gh": "gh", "go": "go", "cargo": "rust", "rustc": "rust", "ruby": "ruby",
		"bundle": "ruby", "java": "java", "mvn": "java", "gradle": "java", "dotnet": "dotnet", "php": "php",
		"composer": "php", "ffmpeg": "ffmpeg", "pandoc": "pandoc", "libreoffice": "libreoffice", "soffice": "libreoffice",
	}
	shellFences     = set("", "bash", "sh", "shell", "zsh", "console", "shell-session", "shellscript")
	networkCommands = set("curl", "wget")
	networkSubs     = map[string][]string{"pip": {"install"}, "pip3": {"install"}, "npm": {"install", "i"}, "pnpm": {"install", "add"}, "yarn": {"add"},
		"git": {"clone", "fetch", "pull", "push"}, "docker": {"pull"}, "uvx": nil, "npx": nil, "gh": nil}
	networkCode = regexp.MustCompile(`\b(requests\.(get|post|put|delete|request)|urllib\.request|httpx\.|aiohttp|fetch\(|axios|XMLHttpRequest)|\b(curl|wget)\s`)
	mcpTool     = regexp.MustCompile(`\bmcp__([A-Za-z0-9][A-Za-z0-9_-]*?)__[A-Za-z0-9_*-]+`)
	mcpToolHead = regexp.MustCompile(`^\bmcp__([A-Za-z0-9][A-Za-z0-9_-]*?)__[A-Za-z0-9_*-]+`)
	mcpBare     = regexp.MustCompile(`^mcp__([A-Za-z0-9][A-Za-z0-9_-]*)$`)
	frontmatter = regexp.MustCompile(frontmatterPattern)
	allowedKey  = regexp.MustCompile(`^allowed[-_]tools:[ \t]*(.*)$`)
	fenceLine   = regexp.MustCompile("^\\s*(```+|~~~+)\\s*([\\w-]*)")
	inlineCode  = regexp.MustCompile("`([^`\\n]+)`")
	cmdSplit    = regexp.MustCompile(`&&|\|\||;|\||\$\(|\(`)
	promptHead  = regexp.MustCompile(`^\s*[$>]\s*`)
	envAssign   = regexp.MustCompile(`^[A-Za-z_]\w*=`)
	toolName    = regexp.MustCompile(`^[A-Z][A-Za-z]*$`)
	filterTail  = regexp.MustCompile(`\(.*$`)
	listDash    = regexp.MustCompile(`^\s*-\s*`)
)

func set(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, i := range items {
		out[i] = true
	}
	return out
}

func frontmatterBlock(text string) string {
	if m := frontmatter.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

func splitTools(raw string) []string {
	var parts []string
	depth, cur := 0, strings.Builder{}
	for _, ch := range raw {
		if ch == '(' {
			depth++
		}
		if ch == ')' && depth > 0 {
			depth--
		}
		if depth == 0 && strings.ContainsRune(", \t\r\n", ch) {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(ch)
	}
	parts = append(parts, cur.String())
	var out []string
	for _, p := range parts {
		if p = strings.Trim(p, "\"'[]"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func beforeComment(s string) string { return strings.TrimSpace(strings.SplitN(s, " #", 2)[0]) }

// AllowedTools returns the tool names of the frontmatter allowed-tools key as written, from an inline string, a JSON list or a block list.
func AllowedTools(text string) []string {
	lines := strings.Split(strings.ReplaceAll(frontmatterBlock(text), "\r", ""), "\n")
	for i, line := range lines {
		m := allowedKey.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := beforeComment(m[1])
		if rest != "" && !strings.HasPrefix(rest, "|") && !strings.HasPrefix(rest, ">") {
			return splitTools(rest)
		}
		var block []string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) != "" && !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") && !strings.HasPrefix(next, "-") {
				break
			}
			if item := strings.TrimSpace(listDash.ReplaceAllString(strings.SplitN(next, " #", 2)[0], "")); item != "" {
				block = append(block, item)
			}
		}
		return splitTools(strings.Join(block, " "))
	}
	return nil
}

func codeCommands(body string) [][]string {
	var segments []string
	fence, open := "", false
	for _, line := range strings.Split(body, "\n") {
		if f := fenceLine.FindStringSubmatch(line); f != nil {
			fence, open = strings.ToLower(f[2]), !open
			continue
		}
		switch {
		case !open:
			for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
				segments = append(segments, m[1])
			}
		case shellFences[fence]:
			segments = append(segments, line)
		}
	}
	var out [][]string
	for _, seg := range segments {
		if strings.HasPrefix(strings.TrimLeft(seg, " \t"), "#") {
			continue
		}
		for _, part := range cmdSplit.Split(seg, -1) {
			tokens := strings.Fields(promptHead.ReplaceAllString(part, ""))
			for len(tokens) > 0 && (tokens[0] == "sudo" || envAssign.MatchString(tokens[0])) {
				tokens = tokens[1:]
			}
			if len(tokens) > 0 {
				out = append(out, tokens)
			}
		}
	}
	return out
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func ext(rel string) string {
	base := rel[strings.LastIndex(rel, "/")+1:]
	if i := strings.LastIndex(base, "."); i >= 0 {
		return strings.ToLower(base[i+1:])
	}
	return ""
}

// Analyze reads a skill's SKILL.md text and file paths into its requirements; readFile returns a bundled file's text and false when it has none.
func Analyze(skillMD string, paths []string, readFile func(rel string) (string, bool)) port.SkillNeeds {
	fm := frontmatterBlock(skillMD)
	body := skillMD
	if fm != "" {
		body = string([]rune(skillMD)[len([]rune(fm)):])
	}
	tools, mcp := map[string]bool{}, map[string]bool{}
	for _, item := range AllowedTools(skillMD) {
		name := strings.TrimSpace(filterTail.ReplaceAllString(item, ""))
		if m := mcpToolHead.FindStringSubmatch(name); m != nil {
			mcp[m[1]] = true
		} else if m := mcpBare.FindStringSubmatch(name); m != nil {
			mcp[m[1]] = true
		} else if toolName.MatchString(name) {
			tools[name] = true
		}
	}
	for _, m := range mcpTool.FindAllStringSubmatch(body, -1) {
		mcp[m[1]] = true
	}
	runtimes, network := map[string]bool{}, false
	for t := range tools {
		network = network || webTools[t]
	}
	for _, rel := range paths {
		if !strings.HasPrefix(rel, "scripts/") {
			continue
		}
		rt, ok := scriptExt[ext(rel)]
		if !ok {
			continue
		}
		runtimes[rt] = true
		if text, has := readFile(rel); has {
			network = network || networkCode.MatchString(text[:min(len(text), maxScriptBytes)])
		}
	}
	for _, tokens := range codeCommands(body) {
		cmd := tokens[0]
		if rt, ok := commandRuns[cmd]; ok {
			runtimes[rt] = true
		}
		if networkCommands[cmd] {
			network = true
		} else if subs, ok := networkSubs[cmd]; ok {
			network = network || subs == nil || len(tokens) > 1 && slices.Contains(subs, tokens[1])
		}
	}
	delete(mcp, "versatile")
	return port.SkillNeeds{Tools: sorted(tools), Runtimes: sorted(runtimes), Network: network, MCP: sorted(mcp)}
}

// Capabilities is what a skill needs from its app: a shell, files and the web.
type Capabilities struct{ Shell, Files, Web bool }

// Needs returns the capabilities the requirements ask of a client.
func Needs(n port.SkillNeeds, hasScripts bool) Capabilities {
	c := Capabilities{Web: n.Network}
	for _, t := range n.Tools {
		c.Shell, c.Files = c.Shell || shellTools[t], c.Files || fileTools[t]
	}
	c.Shell = c.Shell || hasScripts || len(n.Runtimes) > 0
	return c
}

// StatusOf returns the status and reasons of one client: no when it must run bundled code without a shell, instructions when a capability is missing, full otherwise.
func StatusOf(c Client, n port.SkillNeeds, hasScripts bool) (string, []string) {
	want := Needs(n, hasScripts)
	var missing []string
	for _, cap := range []struct {
		name         string
		wanted, have bool
	}{{ReasonNoShell, want.Shell, c.Shell}, {ReasonNoFiles, want.Files, c.Files}, {ReasonNoWeb, want.Web, c.Web}} {
		if cap.wanted && !cap.have {
			missing = append(missing, cap.name)
		}
	}
	switch {
	case hasScripts && !c.Shell:
		return StatusNo, []string{ReasonRunsBundled}
	case len(missing) > 0:
		return StatusInstructions, missing
	}
	return StatusFull, []string{ReasonOK}
}

// Compat returns one derived record per client of the table.
func Compat(n port.SkillNeeds, hasScripts bool) []port.SkillCompat {
	out := make([]port.SkillCompat, len(Clients))
	for i, c := range Clients {
		s, r := StatusOf(c, n, hasScripts)
		out[i] = port.SkillCompat{Client: c.ID, Status: s, Reasons: r, Source: Source}
	}
	return out
}

// ChatStatus is the status in the chat-only client.
func ChatStatus(n port.SkillNeeds, hasScripts bool) string {
	s, _ := StatusOf(Clients[len(Clients)-1], n, hasScripts)
	return s
}

// Find returns the table client with id.
func Find(id string) (Client, bool) {
	i := slices.IndexFunc(Clients, func(c Client) bool { return c.ID == id })
	if i < 0 {
		return Client{}, false
	}
	return Clients[i], true
}
