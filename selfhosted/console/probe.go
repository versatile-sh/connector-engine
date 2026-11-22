package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/port"
)

const probeTimeout = 20 * time.Second

// Access is what the owner chose for one function: off, on (allowed at the class of its own nature) or ask (each call waits for the owner).
type Access string

// The three choices of a function.
const (
	AccessOff Access = "off"
	AccessOn  Access = "on"
	AccessAsk Access = "ask"
)

// Valid reports whether a is one of the three choices.
func (a Access) Valid() bool {
	return a == AccessOff || a == AccessOn || a == AccessAsk
}

func (a Access) mode() port.Mode {
	switch a {
	case AccessOn:
		return port.ModeAllow
	case AccessAsk:
		return port.ModeAskOnce
	}
	return port.ModeDeny
}

func accessFor(m port.Mode) (Access, bool) {
	switch m {
	case port.ModeAllow:
		return AccessOn, true
	case port.ModeAsk, port.ModeAskOnce:
		return AccessAsk, true
	}
	return AccessOff, false
}

// ProbedTool is one tool an MCP server listed, with its hints and the choice the console proposes.
type ProbedTool struct {
	Name        string     `json:"name"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	ReadOnly    bool       `json:"read_only"`
	Destructive bool       `json:"destructive"`
	Kind        port.Class `json:"kind"`
	Estimated   bool       `json:"estimated,omitempty"`
	Proposed    Access     `json:"proposed"`
}

// Probed is what an MCP server said about itself and its tools.
type Probed struct {
	URL         string       `json:"url"`
	Protocol    string       `json:"protocol"`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Version     string       `json:"version,omitempty"`
	WebsiteURL  string       `json:"website_url,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Tools       []ProbedTool `json:"tools"`
	Items       []Item       `json:"items"`
	OAuth       *OAuthHint   `json:"oauth,omitempty"`
}

// OAuthHint is what a server that answered 401 said about its sign-in: the issuer to ask and the resource a token is for.
type OAuthHint struct {
	Issuer   string `json:"issuer"`
	Resource string `json:"resource"`
}

// Propose pre-classifies a tool from its hints, which come from an untrusted server, or from its name when it declares none: only a read is on.
func Propose(name string, a *mcp.ToolAnnotations) (kind port.Class, estimated bool, proposed Access) {
	kind, estimated = KindOf(name, a)
	if kind == port.ClassRead {
		return kind, estimated, AccessOn
	}
	return kind, estimated, AccessAsk
}

func probedTool(name, title, description string, hints *mcp.ToolAnnotations) ProbedTool {
	kind, estimated, proposed := Propose(name, hints)
	return ProbedTool{Name: name, Title: title, Description: description, ReadOnly: !estimated && kind == port.ClassRead, Destructive: !estimated && kind == port.ClassDestructive, Kind: kind, Estimated: estimated, Proposed: proposed}
}

func hintsOf(a *port.Annotations) *mcp.ToolAnnotations {
	if a == nil {
		return nil
	}
	return &mcp.ToolAnnotations{ReadOnlyHint: a.ReadOnlyHint != nil && *a.ReadOnlyHint, DestructiveHint: a.DestructiveHint}
}

// KindOf names what a tool does by its hints: read or destructive as declared, write when it declares it does not delete.
// A tool that declares neither gets Estimate of its name, with estimated true.
func KindOf(name string, a *mcp.ToolAnnotations) (kind port.Class, estimated bool) {
	switch {
	case a != nil && a.ReadOnlyHint:
		return port.ClassRead, false
	case a == nil || a.DestructiveHint == nil:
		return Estimate(name), true
	case *a.DestructiveHint:
		return port.ClassDestructive, false
	}
	return port.ClassWrite, false
}

var (
	destructiveVerbs = wordSet("delete", "remove", "trash", "destroy", "drop", "purge", "revoke", "cancel", "archive", "erase", "wipe", "unlink", "uninstall", "terminate")
	writeVerbs       = wordSet("create", "update", "send", "write", "post", "add", "set", "upload", "edit", "book", "submit", "publish", "copy", "move", "rename", "share", "invite", "assign", "upsert", "insert", "put", "patch", "modify", "change", "save", "apply", "merge", "close", "reopen", "approve", "reject", "complete", "start", "stop", "run", "execute", "trigger", "schedule", "reply", "comment", "react", "pin", "unpin", "star", "mark", "enable", "disable", "restore", "import", "attach", "link", "transfer", "pay", "charge", "refund", "deploy", "commit", "push", "duplicate", "convert", "generate", "make", "new", "manage", "batch", "bulk", "toggle", "lock", "unlock", "subscribe", "unsubscribe", "follow", "unfollow", "like", "tag", "label", "move", "resolve", "invoke", "call", "fill", "replace", "append", "clear", "reset", "clone", "sync", "install", "register", "upgrade", "retry", "rerun", "build", "claim", "rotate", "grant", "revert", "rollback")
	camelBoundary    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	nonWord          = regexp.MustCompile(`[^a-z0-9]+`)
)

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// Estimate guesses a tool's class from the verbs in its name, by the MCP catalog's rule: a destructive verb, else a write verb, else read.
func Estimate(name string) port.Class {
	words := nonWord.Split(strings.ToLower(camelBoundary.ReplaceAllString(name, "${1}_${2}")), -1)
	switch {
	case slices.ContainsFunc(words, func(w string) bool { return destructiveVerbs[w] }):
		return port.ClassDestructive
	case slices.ContainsFunc(words, func(w string) bool { return writeVerbs[w] }):
		return port.ClassWrite
	}
	return port.ClassRead
}

// Nature is what an installed function does: its reviewed class in the pinned manifest m when it has one, else what KindOf says.
func Nature(f port.Function, m port.Manifest) (kind port.Class, estimated bool) {
	if class, source := listing.Base(f, m); source == port.ClassSourceReviewed {
		return class, false
	}
	var d struct {
		Annotations *mcp.ToolAnnotations `json:"annotations"`
	}
	_ = json.Unmarshal(f.Definition, &d)
	return KindOf(f.Name, d.Annotations)
}

// ErrSignInRequired means the server answered 401: it lists its functions only to a signed-in client.
var ErrSignInRequired = errors.New("asks you to sign in before it lists its functions")

// SignInError is a 401 from an MCP server, with the WWW-Authenticate values it carried; it matches ErrSignInRequired.
type SignInError struct {
	Host       string
	Challenges []string
}

// Error words the refusal for a person.
func (e *SignInError) Error() string { return e.Host + ": " + ErrSignInRequired.Error() }

// Is reports whether target is ErrSignInRequired.
func (e *SignInError) Is(target error) bool { return target == ErrSignInRequired }

type signInGate struct {
	next       http.RoundTripper
	mu         sync.Mutex
	asked      bool
	challenges []string
}

func (g *signInGate) answered() (bool, []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.asked, g.challenges
}

func (g *signInGate) RoundTrip(r *http.Request) (*http.Response, error) {
	next := g.next
	if next == nil {
		next = http.DefaultTransport
	}
	res, err := next.RoundTrip(r)
	if err == nil && res.StatusCode == http.StatusUnauthorized {
		g.mu.Lock()
		g.asked, g.challenges = true, slices.Clone(res.Header.Values("WWW-Authenticate"))
		g.mu.Unlock()
	}
	return res, err
}

// Probe connects to an MCP server (server/discover for 2026-07-28, initialize before), reads its server info and lists its tools.
func Probe(ctx context.Context, rawURL string, reach Reach) (Probed, error) {
	u, err := reach.CheckURL(rawURL)
	if err != nil {
		return Probed{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "connector-console", Version: "0"}, nil)
	httpClient := reach.Client()
	httpClient.Timeout = 0
	gate := &signInGate{next: httpClient.Transport}
	httpClient.Transport = gate
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: u.String(), HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if asked, challenges := gate.answered(); err != nil && asked {
		return Probed{}, unreachable(&SignInError{Host: u.Host, Challenges: challenges})
	}
	if err != nil {
		return Probed{}, unreachable(fmt.Errorf("could not connect to %s: %w", u.Host, err))
	}
	defer session.Close()
	out := Probed{URL: u.String()}
	var instructions string
	if init := session.InitializeResult(); init != nil {
		out.Protocol, instructions = init.ProtocolVersion, init.Instructions
		if s := init.ServerInfo; s != nil {
			out.Name, out.Title, out.Description, out.Version, out.WebsiteURL = s.Name, s.Title, s.Description, s.Version, s.WebsiteURL
			out.Icon = pickIcon(s.Icons)
		}
	}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return out, unreachable(fmt.Errorf("tools/list on %s: %w", u.Host, err))
		}
		title := tool.Title
		if title == "" && tool.Annotations != nil {
			title = tool.Annotations.Title
		}
		out.Tools = append(out.Tools, probedTool(tool.Name, title, tool.Description, tool.Annotations))
	}
	if out.Items, err = probeItems(ctx, session, instructions); err != nil {
		return out, unreachable(fmt.Errorf("%w on %s", err, u.Host))
	}
	if out.Icon == "" {
		out.Icon = faviconOf(out.WebsiteURL, u.String())
	}
	return out, nil
}

func listed[T any](out *[]Item, method string, list iter.Seq2[T, error], name func(T) string) error {
	for v, err := range list {
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		raw, _ := json.Marshal(v)
		if it, ok := ItemOf(name(v), raw); ok {
			*out = append(*out, it)
		}
	}
	return nil
}

func probeItems(ctx context.Context, session *mcp.ClientSession, instructions string) ([]Item, error) {
	out := []Item{}
	if instructions != "" {
		raw, _ := json.Marshal(map[string]string{"instructions": instructions})
		it, _ := ItemOf(port.InstructionsItem, raw)
		out = append(out, it)
	}
	caps := session.InitializeResult().Capabilities
	if caps == nil {
		return out, nil
	}
	if caps.Prompts != nil {
		if err := listed(&out, "prompts/list", session.Prompts(ctx, nil), func(p *mcp.Prompt) string { return port.ItemPrompt + p.Name }); err != nil {
			return out, err
		}
	}
	if caps.Resources != nil {
		if err := listed(&out, "resources/list", session.Resources(ctx, nil), func(r *mcp.Resource) string { return port.ItemResource + r.URI }); err != nil {
			return out, err
		}
		if err := listed(&out, "resources/templates/list", session.ResourceTemplates(ctx, nil), func(t *mcp.ResourceTemplate) string { return port.ItemResource + t.URITemplate }); err != nil {
			return out, err
		}
	}
	return out, nil
}

func pickIcon(icons []mcp.Icon) string {
	for _, ic := range icons {
		src := strings.TrimSpace(ic.Source)
		if strings.Contains(strings.ToLower(ic.MIMEType), "svg") || strings.HasSuffix(strings.ToLower(src), ".svg") || strings.HasPrefix(src, "data:image/svg") {
			continue
		}
		if strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:image/") {
			return src
		}
	}
	return ""
}

func faviconOf(candidates ...string) string {
	for _, c := range candidates {
		u, err := url.Parse(c)
		if err == nil && u.Scheme == "https" && u.Host != "" {
			return "https://" + u.Host + "/favicon.ico"
		}
	}
	return ""
}
