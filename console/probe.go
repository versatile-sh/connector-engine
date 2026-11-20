package console

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/port"
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
	return ""
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
}

// Propose pre-classifies a tool from its hints, which come from an untrusted server: only readOnlyHint true is on; anything else waits for the owner.
func Propose(a *mcp.ToolAnnotations) (readOnly, destructive bool, proposed Access) {
	if a != nil && a.ReadOnlyHint {
		return true, false, AccessOn
	}
	destructive = a == nil || a.DestructiveHint == nil || *a.DestructiveHint
	return false, destructive, AccessAsk
}

// KindOf names what a tool does by its hints: read, destructive (the MCP default when not read-only) or write.
func KindOf(readOnly, destructive bool) port.Class {
	switch {
	case readOnly:
		return port.ClassRead
	case destructive:
		return port.ClassDestructive
	}
	return port.ClassWrite
}

// Nature is what an installed function does: its reviewed class when it has one, else what its MCP hints say.
func Nature(f port.Function) port.Class {
	if f.ClassSource == port.ClassSourceReviewed && f.Class != port.ClassNone {
		return f.Class
	}
	var d struct {
		Annotations *mcp.ToolAnnotations `json:"annotations"`
	}
	_ = json.Unmarshal(f.Definition, &d)
	readOnly, destructive, _ := Propose(d.Annotations)
	return KindOf(readOnly, destructive)
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
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: u.String(), HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
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
		ro, de, prop := Propose(tool.Annotations)
		title := tool.Title
		if title == "" && tool.Annotations != nil {
			title = tool.Annotations.Title
		}
		out.Tools = append(out.Tools, ProbedTool{Name: tool.Name, Title: title, Description: tool.Description, ReadOnly: ro, Destructive: de, Kind: KindOf(ro, de), Proposed: prop})
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
