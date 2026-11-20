package console

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const probeTimeout = 20 * time.Second

// Access is what the owner chose for one function: off, read (allowed, classed read), write (allowed, classed write) or ask (each call waits for the owner).
type Access string

// The four choices of a function.
const (
	AccessOff   Access = "off"
	AccessRead  Access = "read"
	AccessWrite Access = "write"
	AccessAsk   Access = "ask"
)

// Valid reports whether a is one of the four choices.
func (a Access) Valid() bool {
	return a == AccessOff || a == AccessRead || a == AccessWrite || a == AccessAsk
}

// ProbedTool is one tool an MCP server listed, with its hints and the choice the console proposes.
type ProbedTool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	Destructive bool   `json:"destructive"`
	Proposed    Access `json:"proposed"`
}

// Probed is what an MCP server said about itself and its tools.
type Probed struct {
	URL          string       `json:"url"`
	Protocol     string       `json:"protocol"`
	Name         string       `json:"name"`
	Title        string       `json:"title,omitempty"`
	Description  string       `json:"description,omitempty"`
	Version      string       `json:"version,omitempty"`
	WebsiteURL   string       `json:"website_url,omitempty"`
	Icon         string       `json:"icon,omitempty"`
	Instructions string       `json:"instructions,omitempty"`
	Tools        []ProbedTool `json:"tools"`
}

// Propose pre-classifies a tool from its hints, which come from an untrusted server: only readOnlyHint true is read; anything else waits for the owner.
func Propose(a *mcp.ToolAnnotations) (readOnly, destructive bool, proposed Access) {
	if a != nil && a.ReadOnlyHint {
		return true, false, AccessRead
	}
	destructive = a == nil || a.DestructiveHint == nil || *a.DestructiveHint
	return false, destructive, AccessAsk
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
		return Probed{}, fmt.Errorf("could not connect to %s: %w", u.Host, err)
	}
	defer session.Close()
	out := Probed{URL: u.String()}
	if init := session.InitializeResult(); init != nil {
		out.Protocol, out.Instructions = init.ProtocolVersion, init.Instructions
		if s := init.ServerInfo; s != nil {
			out.Name, out.Title, out.Description, out.Version, out.WebsiteURL = s.Name, s.Title, s.Description, s.Version, s.WebsiteURL
			out.Icon = pickIcon(s.Icons)
		}
	}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return out, fmt.Errorf("tools/list on %s: %w", u.Host, err)
		}
		ro, de, prop := Propose(tool.Annotations)
		title := tool.Title
		if title == "" && tool.Annotations != nil {
			title = tool.Annotations.Title
		}
		out.Tools = append(out.Tools, ProbedTool{Name: tool.Name, Title: title, Description: tool.Description, ReadOnly: ro, Destructive: de, Proposed: prop})
	}
	if out.Icon == "" {
		out.Icon = faviconOf(out.WebsiteURL, u.String())
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
