// Package mcpapi serves the management MCP surface (§13.2): the reading operations of the table, and a mutation only when named.
package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/server"
)

// Options configure the surface; Mutations are the operation ids offered besides the reading ones, and the never-offered ones are refused at New.
type Options struct {
	Env       *server.Env
	Authn     server.Authenticator
	Mutations []string
	Version   string
}

// Offered lists the operations the surface exposes for the mutations named: every reading one, then the named ones (§13.2 P3, P4).
func Offered(mutations []string) ([]server.Operation, error) {
	var out []server.Operation
	for _, o := range server.Operations() {
		if o.Live || o.Never {
			continue
		}
		if o.Reads() || slices.Contains(mutations, o.ID) {
			out = append(out, o)
		}
	}
	for _, name := range mutations {
		if !slices.ContainsFunc(out, func(o server.Operation) bool { return o.ID == name }) {
			return nil, fmt.Errorf("manage.mcp.operations: %q is not an operation the management surface may offer", name)
		}
	}
	return out, nil
}

// New builds the handler of /mcp/manage: Streamable HTTP, a person token of the manage audience on every request.
func New(o Options) (http.Handler, error) {
	if o.Env == nil || o.Env.Svc == nil || o.Authn == nil {
		return nil, errors.New("mcpapi: an environment and an authenticator are required")
	}
	ops, err := Offered(o.Mutations)
	if err != nil {
		return nil, err
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "connector-engine-manage", Version: o.Version}, nil)
	for _, op := range ops {
		srv.AddTool(&mcp.Tool{Name: op.ID, Description: op.Method + " " + op.Path, InputSchema: server.InlineSchema(reflect.TypeOf(op.In()).Elem())}, tool{o, op}.call)
	}
	next := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return gate{o, next}, nil
}

type gate struct {
	Options
	next http.Handler
}

func (g gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := server.Authenticate(r.Context(), server.Operation{}, g.Authn, server.Bearer(r.Header.Get(server.HeaderAuthorization))); err != nil {
		p := server.ProblemOf(err)
		w.Header().Set("Content-Type", server.ProblemMediaType)
		w.WriteHeader(p.Status)
		_ = json.NewEncoder(w).Encode(p)
		return
	}
	g.next.ServeHTTP(w, r)
}

type tool struct {
	Options
	op server.Operation
}

func (t tool) call(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	header := http.Header{}
	if req.Extra != nil {
		header = req.Extra.Header
	}
	p, err := server.Authenticate(ctx, t.op, t.Authn, server.Bearer(header.Get(server.HeaderAuthorization)))
	if err != nil {
		return failure(err), nil
	}
	in := t.op.In()
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, in); err != nil {
			return failure(port.Errorf(port.CodeInvalidArgument, "arguments: %v", err)), nil
		}
	}
	out, err := t.op.Invoke(ctx, t.Env, p, header.Get(server.HeaderIdempotencyKey), in)
	if err != nil {
		return failure(err), nil
	}
	text, err := json.Marshal(out)
	if err != nil {
		return failure(err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}, StructuredContent: out}, nil
}

func failure(err error) *mcp.CallToolResult {
	text, _ := json.Marshal(server.ProblemOf(err))
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}, IsError: true}
}
