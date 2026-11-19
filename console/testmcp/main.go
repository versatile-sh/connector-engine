// Command testmcp is a tiny MCP server for the packaging check: one read, one write, one destructive and one unhinted tool.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:7999", "address to listen on")
	flag.Parse()
	srv := mcp.NewServer(&mcp.Implementation{Name: "check-notes", Title: "Check notes", Version: "1.0.0", Description: "A notebook for the packaging check"}, nil)
	no, yes := false, true
	for _, t := range []struct {
		name string
		ann  *mcp.ToolAnnotations
	}{{"search", &mcp.ToolAnnotations{ReadOnlyHint: true}}, {"create", &mcp.ToolAnnotations{DestructiveHint: &no}}, {"purge", &mcp.ToolAnnotations{DestructiveHint: &yes}}, {"mystery", nil}} {
		srv.AddTool(&mcp.Tool{Name: t.name, Description: "does " + t.name, Annotations: t.ann, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ran " + req.Params.Name}}}, nil
			})
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	log.Fatal((&http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}).ListenAndServe())
}
