package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxConnectorInstructions = 2048
	maxRelayedInstructions   = 8192
	relayedHead              = "\n\nInstructions the connected servers give, each accepted by a person; the quoted text is the server's, not the engine's."
	relayedMore              = "\n\nMore connectors have instructions; they did not fit."
	truncatedMark            = " [truncated]"
)

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - len(truncatedMark)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncatedMark
}

func quoted(text string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r", ""), "\n")
	return "> " + strings.Join(lines, "\n> ")
}

// RelayedInstructions is the engine's instructions followed by one quoted section per instructions item, each cut to 2,048 bytes and all to 8,192, with every cut marked.
func RelayedInstructions(base string, items []service.ServedItem) string {
	var relayed []service.ServedItem
	for _, it := range items {
		if port.ItemKind(it.Item) == port.ItemInstructions {
			relayed = append(relayed, it)
		}
	}
	if len(relayed) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(base + relayedHead)
	for _, r := range relayed {
		var d struct {
			Instructions string `json:"instructions"`
		}
		_ = json.Unmarshal(r.Definition, &d)
		section := "\n\nFrom connector " + r.Alias + " (its tools are named " + r.Alias + "__*):\n" + quoted(clip(d.Instructions, maxConnectorInstructions))
		if b.Len()-len(base)+len(section)+len(relayedMore) > maxRelayedInstructions {
			b.WriteString(relayedMore)
			break
		}
		b.WriteString(section)
	}
	return b.String()
}

func (g *Gateway) instructed(ctx context.Context, req mcp.Request, res mcp.Result) {
	var instructions *string
	switch r := res.(type) {
	case *mcp.InitializeResult:
		instructions = &r.Instructions
	case *mcp.DiscoverResult:
		instructions, r.CacheScope = &r.Instructions, "private"
	default:
		return
	}
	p, err := principalOf(req)
	if err != nil {
		return
	}
	if fl, err := g.core.ListFunctions(ctx, p); err == nil {
		*instructions = RelayedInstructions(SkillIndex(*instructions, fl.Skills), fl.Items)
	}
}
