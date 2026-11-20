package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/port"
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
		if res, served, err := g.contentRequest(ctx, req); served {
			return res, err
		}
		if refusedMethods[method] {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		}
		res, err := next(ctx, method, req)
		if (g.cfg.Skills || g.cfg.Instructions) && err == nil {
			g.instructed(ctx, req, res)
		}
		return res, err
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

func ownError(rpc int64, code port.Code, message string) *jsonrpc.Error {
	data, _ := json.Marshal(map[string]any{"_meta": mcp.Meta{port.ErrorMeta: code}})
	return &jsonrpc.Error{Code: rpc, Message: message, Data: data}
}

func unknownTool(name string) error {
	return ownError(jsonrpc.CodeInvalidParams, port.CodeUnknownTool, fmt.Sprintf("unknown tool %q", name))
}

func (g *Gateway) failure(err error) error {
	var pe *port.Error
	switch port.CodeOf(err) {
	case port.CodeInvalidArgument:
		errors.As(err, &pe)
		return ownError(jsonrpc.CodeInvalidParams, port.CodeInvalidArgument, pe.Detail)
	case port.CodePermissionDenied, port.CodeUnauthenticated, port.CodeAgentPrincipalRefused, port.CodeTenantRemoved:
		return ownError(jsonrpc.CodeInvalidRequest, port.CodePermissionDenied, "request refused")
	}
	g.cfg.Logger.Error("agent endpoint", "error", err)
	return ownError(jsonrpc.CodeInternalError, port.CodeInternal, "internal error")
}
