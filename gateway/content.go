package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/core/names"
	"github.com/versatile-sh/connector-engine/internal/tokenscan"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

const (
	codePending    = -32050
	skillPromptEnd = "\n\nThis is the skill %s. Load its other files with skill_load, skill %q and the file's path."
)

func (g *Gateway) contentRequest(ctx context.Context, req mcp.Request) (mcp.Result, bool, error) {
	switch r := req.(type) {
	case *mcp.ListPromptsRequest:
		if g.cfg.prompts() {
			res, err := g.listPrompts(ctx, r)
			return res, true, err
		}
	case *mcp.GetPromptRequest:
		if g.cfg.prompts() {
			res, err := g.getPrompt(ctx, r)
			return res, true, err
		}
	case *mcp.ListResourcesRequest, *mcp.ListResourceTemplatesRequest:
		if g.cfg.Content {
			res, err := g.listResources(ctx, req)
			return res, true, err
		}
	case *mcp.ReadResourceRequest:
		if g.cfg.Content {
			res, err := g.readResource(ctx, r)
			return res, true, err
		}
	case *mcp.CompleteRequest:
		if g.cfg.Content {
			res, err := g.complete(ctx, r)
			return res, true, err
		}
	}
	return nil, false, nil
}

func (g *Gateway) items(ctx context.Context, req mcp.Request, kind string) (service.FunctionList, []service.ServedItem, error) {
	p, err := principalOf(req)
	if err != nil {
		return service.FunctionList{}, nil, err
	}
	fl, err := g.core.ListFunctions(ctx, p)
	if err != nil {
		return fl, nil, g.failure(err)
	}
	var out []service.ServedItem
	for _, it := range fl.Items {
		if port.ItemKind(it.Item) == kind {
			out = append(out, it)
		}
	}
	return fl, out, nil
}

func cacheable(fl service.FunctionList) mcp.Cacheable {
	return mcp.Cacheable{TTLMs: int(fl.TTL.Milliseconds()), CacheScope: "private"}
}

func decoded[T any](items []service.ServedItem, keep func(*T) bool) []*T {
	out := []*T{}
	for _, it := range items {
		v := new(T)
		if json.Unmarshal(it.Definition, v) == nil && keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func (g *Gateway) listPrompts(ctx context.Context, req *mcp.ListPromptsRequest) (mcp.Result, error) {
	fl, items, err := g.items(ctx, req, port.ItemPrompt)
	if err != nil {
		return nil, err
	}
	prompts := []*mcp.Prompt{}
	if g.cfg.Content {
		prompts = decoded(items, func(*mcp.Prompt) bool { return true })
	}
	if g.cfg.SkillPrompts {
		for _, s := range fl.Skills {
			prompts = append(prompts, &mcp.Prompt{Name: s.Name, Title: s.Title, Description: s.Description})
		}
	}
	return &mcp.ListPromptsResult{Prompts: prompts, Cacheable: cacheable(fl)}, nil
}

func unknownItem(what, name string) error {
	return ownError(jsonrpc.CodeInvalidParams, port.CodeUnknownTool, fmt.Sprintf("unknown %s %q", what, name))
}

func (g *Gateway) itemResult(ctx context.Context, req mcp.Request, what string, call service.CallRequest, into any) error {
	p, err := principalOf(req)
	if err != nil {
		return err
	}
	out, err := g.run(ctx, p, deliveriesOf(req), call)
	if err != nil {
		return err
	}
	switch out.Status {
	case service.OutcomeUnknownTool:
		return unknownItem(what, call.Name)
	case service.OutcomePending:
		data, _ := json.Marshal(map[string]any{"approval_id": out.Approval.ID, "state": "pending", "poll_after_seconds": max(int(out.PollAfter/time.Second), 1)})
		return &jsonrpc.Error{Code: codePending, Message: "waiting for a person's approval; send the same request again later", Data: data}
	case service.OutcomeError:
		return ownError(jsonrpc.CodeInternalError, out.Code, string(out.Code)+": "+strings.ReplaceAll(out.Detail, p.Token, redacted))
	}
	if err := json.Unmarshal(tokenscan.Scrub(out.Result.Content, p.Token, redacted), into); err != nil {
		g.cfg.Logger.Error("agent endpoint", "error", err)
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "internal error"}
	}
	return nil
}

func (g *Gateway) getPrompt(ctx context.Context, req *mcp.GetPromptRequest) (mcp.Result, error) {
	name := req.Params.Name
	if g.cfg.SkillPrompts && names.ValidAlias(name) == nil {
		var blocks []*mcp.TextContent
		args, _ := json.Marshal(map[string]string{"path": ""})
		if err := g.itemResult(ctx, req, "prompt", service.CallRequest{Name: name, Arguments: args, Skill: true}, &blocks); err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, t := range blocks {
			b.WriteString(t.Text)
		}
		b.WriteString(fmt.Sprintf(skillPromptEnd, name, name))
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: b.String()}}}}, nil
	}
	if !g.cfg.Content {
		return nil, unknownItem("prompt", name)
	}
	args, _ := json.Marshal(map[string]any{"arguments": req.Params.Arguments})
	var messages []*mcp.PromptMessage
	if err := g.itemResult(ctx, req, "prompt", service.CallRequest{Name: name, Arguments: args, Item: port.ItemPrompt}, &messages); err != nil {
		return nil, err
	}
	return &mcp.GetPromptResult{Messages: messages}, nil
}

func (g *Gateway) listResources(ctx context.Context, req mcp.Request) (mcp.Result, error) {
	fl, items, err := g.items(ctx, req, port.ItemResource)
	if err != nil {
		return nil, err
	}
	if _, ok := req.(*mcp.ListResourceTemplatesRequest); ok {
		return &mcp.ListResourceTemplatesResult{ResourceTemplates: decoded(items, func(t *mcp.ResourceTemplate) bool { return t.URITemplate != "" }), Cacheable: cacheable(fl)}, nil
	}
	return &mcp.ListResourcesResult{Resources: decoded(items, func(r *mcp.Resource) bool { return r.URI != "" }), Cacheable: cacheable(fl)}, nil
}

func (g *Gateway) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (mcp.Result, error) {
	var contents []*mcp.ResourceContents
	if err := g.itemResult(ctx, req, "resource", service.CallRequest{Name: req.Params.URI, Item: port.ItemResource}, &contents); err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: contents}, nil
}

func (g *Gateway) complete(ctx context.Context, req *mcp.CompleteRequest) (mcp.Result, error) {
	p, err := principalOf(req)
	if err != nil {
		return nil, err
	}
	ref := req.Params.Ref
	if ref == nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "completion takes a ref"}
	}
	call := service.CallRequest{Name: ref.Name, Item: port.ItemPrompt}
	if ref.Type == "ref/resource" {
		call = service.CallRequest{Name: ref.URI, Item: port.ItemResource}
	}
	call.Arguments, _ = json.Marshal(map[string]any{"argument": req.Params.Argument, "context": req.Params.Context})
	raw, found, err := g.core.Complete(ctx, p, call)
	if err != nil {
		return nil, g.failure(err)
	}
	if !found {
		return nil, unknownItem(strings.TrimPrefix(ref.Type, "ref/"), call.Name)
	}
	res := &mcp.CompleteResult{}
	if err := json.Unmarshal(raw, &res.Completion); err != nil {
		return nil, g.failure(err)
	}
	if res.Completion.Values == nil {
		res.Completion.Values = []string{}
	}
	return res, nil
}
