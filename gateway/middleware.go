package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/agents-control/connector-engine/port"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var refusedMethods = map[string]bool{
	"resources/list": true, "resources/templates/list": true, "resources/read": true, "resources/subscribe": true, "resources/unsubscribe": true,
	"prompts/list": true, "prompts/get": true, "completion/complete": true,
}

func (g *Gateway) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		switch r := req.(type) {
		case *mcp.ListToolsRequest:
			return g.list(ctx, r)
		case *mcp.CallToolRequest:
			return g.call(ctx, r)
		}
		if refusedMethods[method] {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		}
		return next(ctx, method, req)
	}
}

func principalOf(req mcp.Request) (port.Principal, error) {
	if extra := req.GetExtra(); extra != nil && extra.TokenInfo != nil {
		if p, ok := extra.TokenInfo.Extra[principalExtra].(port.Principal); ok {
			return p, nil
		}
	}
	return port.Principal{}, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: "no principal"}
}

func unknownTool(name string) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf("unknown tool %q", name)}
}

func (g *Gateway) failure(err error) error {
	var pe *port.Error
	switch port.CodeOf(err) {
	case port.CodeInvalidArgument:
		errors.As(err, &pe)
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: pe.Detail}
	case port.CodePermissionDenied, port.CodeUnauthenticated, port.CodeAgentPrincipalRefused, port.CodeTenantRemoved:
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: "request refused"}
	}
	g.cfg.Logger.Error("agent endpoint", "error", err)
	return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "internal error"}
}
