package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const customerCPF = "529.982.247-25"

type toolSpec struct {
	name, description string
	annotations       *mcp.ToolAnnotations
	outputSchema      map[string]any
	structured        map[string]any
	text              string
}

type mcpUpstream struct {
	*httptest.Server
	srv   *mcp.Server
	mu    sync.Mutex
	calls map[string]int
}

func ptr[T any](v T) *T { return &v }

func newMCPUpstream(t *testing.T, tools ...toolSpec) *mcpUpstream {
	t.Helper()
	u := &mcpUpstream{srv: mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "1"}, nil), calls: map[string]int{}}
	for _, spec := range tools {
		u.setTool(spec)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return u.srv }, nil)
	u.Server = httptest.NewServer(handler)
	t.Cleanup(u.Close)
	return u
}

func (u *mcpUpstream) setTool(spec toolSpec) {
	tool := &mcp.Tool{Name: spec.name, Description: spec.description, Annotations: spec.annotations, InputSchema: map[string]any{"type": "object"}}
	if spec.outputSchema != nil {
		tool.OutputSchema = spec.outputSchema
	}
	u.srv.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u.mu.Lock()
		u.calls[req.Params.Name]++
		u.mu.Unlock()
		text := spec.text
		if text == "" {
			text = "ran " + req.Params.Name
		}
		res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		if spec.structured != nil {
			res.StructuredContent = spec.structured
		}
		return res, nil
	})
}

func (u *mcpUpstream) count(name string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls[name]
}

func (u *mcpUpstream) url() string { return u.Server.URL + "/mcp" }

func issueTools() []toolSpec {
	return []toolSpec{
		{name: "search", description: "Searches the tracker", annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
		{name: "create_issue", description: "Creates an issue", annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: ptr(false)}},
		{name: "delete_repo", description: "Deletes a repository", annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)}},
		{name: "mystery", description: "Does something nobody classified"},
		{
			name: "customer", description: "Reads one customer", annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			outputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string"}, "cpf": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}}},
			structured: map[string]any{"name": "Ana", "cpf": customerCPF, "note": "vip"},
			text:       "customer Ana, cpf " + customerCPF,
		},
	}
}

type restUpstream struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newRESTUpstream(t *testing.T) *restUpstream {
	t.Helper()
	u := &restUpstream{hits: map[string]int{}}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.hits[r.Method+" "+r.URL.Path]++
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/pets":
			_ = json.NewEncoder(w).Encode(map[string]any{"pets": []map[string]any{{"id": "p1", "name": "Rex"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/pets":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "p2", "name": "Tom"})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/pets/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *restUpstream) count(method, path string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits[method+" "+path]
}

func petstoreSpec(base string) string {
	return fmt.Sprintf(`openapi: 3.0.3
info:
  title: Pet Store
  version: 1.0.0
  contact: {name: Pet Store Inc}
servers:
  - url: %s
paths:
  /pets:
    get:
      operationId: listPets
      summary: List pets
      responses:
        "200": {description: ok}
    post:
      operationId: createPet
      summary: Create a pet
      requestBody:
        content:
          application/json:
            schema: {type: object, properties: {name: {type: string}}}
      responses:
        "201": {description: created}
  /pets/{petId}:
    delete:
      operationId: deletePet
      parameters:
        - {name: petId, in: path, required: true, schema: {type: string}}
      responses:
        "204": {description: gone}
`, base)
}
