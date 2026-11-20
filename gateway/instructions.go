package gateway

import (
	"context"
	"strings"
	"unicode/utf8"

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

// RelayedInstructions is the engine's instructions followed by one quoted section per connector, each cut to 2,048 bytes and all to 8,192, with every cut marked.
func RelayedInstructions(base string, relayed []service.ServedInstructions) string {
	if len(relayed) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(base + relayedHead)
	for _, r := range relayed {
		section := "\n\nFrom connector " + r.Alias + " (its tools are named " + r.Alias + "__*):\n" + quoted(clip(r.Text, maxConnectorInstructions))
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
		*instructions = RelayedInstructions(SkillIndex(*instructions, fl.Skills), fl.Instructions)
	}
}
