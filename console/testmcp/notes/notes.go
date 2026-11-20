// Package notes is the MCP server testmcp serves: four tools, server instructions, two prompts, a static resource and a resource template.
package notes

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Default texts of a server built with zero Options.
const (
	Instructions = "Search before you create a note. Notes are plain text."
	Readme       = "The notes server keeps plain text notes."
)

// Options vary the server between runs; an empty field takes its default, and OnCall, when set, sees every call, get and read.
type Options struct {
	Instructions string
	PromptNote   string
	Readme       string
	OnCall       func(method, name string)
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func text(role mcp.Role, s string) *mcp.PromptMessage {
	return &mcp.PromptMessage{Role: role, Content: &mcp.TextContent{Text: s}}
}

// New builds the notes server.
func New(o Options) *mcp.Server {
	seen := func(method, name string) {
		if o.OnCall != nil {
			o.OnCall(method, name)
		}
	}
	complete := func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
		var values []string
		for _, id := range []string{"1", "2", "12"} {
			if strings.HasPrefix(id, req.Params.Argument.Value) {
				values = append(values, id)
			}
		}
		return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: values}}, nil
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "check-notes", Title: "Check notes", Version: "1.0.0", Description: "A notebook for the packaging check"},
		&mcp.ServerOptions{Instructions: or(o.Instructions, Instructions), CompletionHandler: complete})
	no, yes := false, true
	for _, t := range []struct {
		name string
		ann  *mcp.ToolAnnotations
	}{{"search", &mcp.ToolAnnotations{ReadOnlyHint: true}}, {"create", &mcp.ToolAnnotations{DestructiveHint: &no}}, {"purge", &mcp.ToolAnnotations{DestructiveHint: &yes}}, {"mystery", nil}} {
		srv.AddTool(&mcp.Tool{Name: t.name, Description: "does " + t.name, Annotations: t.ann, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				seen("tools/call", req.Params.Name)
				content := []mcp.Content{&mcp.TextContent{Text: "ran " + req.Params.Name}}
				if req.Params.Name == "search" {
					content = append(content, &mcp.ResourceLink{URI: "notes://1", Name: "note 1"})
				}
				return &mcp.CallToolResult{Content: content}, nil
			})
	}
	srv.AddPrompt(&mcp.Prompt{Name: "summarize", Description: strings.TrimSpace("Summarize the notes on a topic. " + o.PromptNote),
		Arguments: []*mcp.PromptArgument{{Name: "topic", Required: true}}},
		func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			seen("prompts/get", req.Params.Name)
			return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{text("user", "Summarize my notes about "+req.Params.Arguments["topic"]+".")}}, nil
		})
	srv.AddPrompt(&mcp.Prompt{Name: "daily", Description: "Review today's notes."},
		func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			seen("prompts/get", req.Params.Name)
			return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{text("user", "List today's notes."), text("assistant", "Reading them now.")}}, nil
		})
	srv.AddResource(&mcp.Resource{URI: "file:///README.md", Name: "readme", MIMEType: "text/markdown"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			seen("resources/read", req.Params.URI)
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: or(o.Readme, Readme)}}}, nil
		})
	srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "notes://{id}", Name: "note", MIMEType: "text/plain"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			seen("resources/read", req.Params.URI)
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/plain", Text: "note " + strings.TrimPrefix(req.Params.URI, "notes://")}}}, nil
		})
	return srv
}
