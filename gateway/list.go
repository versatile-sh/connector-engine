package gateway

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodeTools(fns []service.ServedFunction) []*mcp.Tool {
	tools := make([]*mcp.Tool, 0, len(fns))
	for _, f := range fns {
		var t mcp.Tool
		if json.Unmarshal(f.Definition, &t) == nil {
			tools = append(tools, &t)
		}
	}
	return tools
}

func (g *Gateway) collapsed(tools []*mcp.Tool) bool {
	if g.index == nil {
		return false
	}
	raw, _ := json.Marshal(tools)
	return len(tools) > g.cfg.ThresholdFunctions || len(raw)/tokenBytesPerTok > g.cfg.ThresholdTokens
}

func (g *Gateway) list(ctx context.Context, req *mcp.ListToolsRequest) (mcp.Result, error) {
	p, err := principalOf(req)
	if err != nil {
		return nil, err
	}
	fl, err := g.core.ListFunctions(ctx, p)
	if err != nil {
		return nil, g.failure(err)
	}
	tools := decodeTools(fl.Functions)
	if g.collapsed(tools) {
		tools = slices.Clone(searchTools)
	}
	if fl.ApprovalTools {
		tools = append(tools, approvalTools...)
	}
	if g.cfg.Skills {
		tools = append(tools, skillTools...)
	}
	return &mcp.ListToolsResult{Tools: tools, Cacheable: mcp.Cacheable{TTLMs: int(fl.TTL.Milliseconds()), CacheScope: "private"}}, nil
}

func (g *Gateway) find(ctx context.Context, p port.Principal, raw json.RawMessage) (*mcp.CallToolResult, error) {
	var a findArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.Query == "" {
		return toolError(port.CodeInvalidArgument, "find takes a query"), nil
	}
	fl, err := g.core.ListFunctions(ctx, p)
	if err != nil {
		return nil, g.failure(err)
	}
	found := []json.RawMessage{}
	if g.index != nil {
		docs := make([]port.ToolDoc, 0, len(fl.Functions))
		names := make([]string, 0, len(fl.Functions))
		byName := make(map[string]json.RawMessage, len(fl.Functions))
		for _, f := range fl.Functions {
			var t mcp.Tool
			if json.Unmarshal(f.Definition, &t) != nil {
				continue
			}
			docs = append(docs, port.ToolDoc{Name: f.Name, Title: t.Title, Description: t.Description})
			names = append(names, f.Name)
			byName[f.Name] = f.Definition
		}
		limit := g.cfg.SearchResultMax
		if a.Limit > 0 && a.Limit < limit {
			limit = a.Limit
		}
		if err := g.index.Put(ctx, p.Claims.Tenant, docs); err != nil {
			return nil, g.failure(err)
		}
		hits, err := g.index.Search(ctx, p.Claims.Tenant, a.Query, names, limit)
		if err != nil {
			return nil, g.failure(err)
		}
		for _, h := range hits {
			found = append(found, byName[h.Name])
		}
	}
	return structured(map[string]any{"tools": found}), nil
}
