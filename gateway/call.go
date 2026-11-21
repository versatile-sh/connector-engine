package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/core/args"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

var callKeys = []string{"name", "arguments", "fields"}

func (g *Gateway) call(ctx context.Context, req *mcp.CallToolRequest) (mcp.Result, error) {
	p, err := principalOf(req)
	if err != nil {
		return nil, err
	}
	params, d := req.Params, deliveriesOf(req)
	if g.isMeta(params.Name) {
		if err := args.Parse(params.Arguments); err != nil {
			return toolError(port.CodeInvalidArgument, err.Error()), nil
		}
	}
	switch params.Name {
	case port.ToolFind:
		return g.find(ctx, p, params.Arguments)
	case port.ToolCall:
		var a callArgs
		if err := json.Unmarshal(params.Arguments, &a); err != nil || a.Name == "" || caseVariantKey(params.Arguments) {
			return toolError(port.CodeInvalidArgument, "call takes a name and arguments"), nil
		}
		return g.forward(ctx, p, d, service.CallRequest{Name: a.Name, Arguments: a.Arguments, Fields: fieldsOf(a.Fields, params.Meta)})
	case port.ToolApprovalStatus, port.ToolApprovalCancel:
		return g.approval(ctx, p, d, params.Name, params.Arguments)
	case port.ToolSkillList, port.ToolSkillLoad:
		if g.cfg.Skills {
			return g.skill(ctx, p, d, params.Name, params.Arguments)
		}
	}
	return g.forward(ctx, p, d, service.CallRequest{Name: params.Name, Arguments: params.Arguments, Fields: fieldsOf(nil, params.Meta)})
}

func (g *Gateway) isMeta(name string) bool {
	switch name {
	case port.ToolFind, port.ToolCall, port.ToolApprovalStatus, port.ToolApprovalCancel:
		return true
	case port.ToolSkillList, port.ToolSkillLoad:
		return g.cfg.Skills
	}
	return false
}

func caseVariantKey(raw json.RawMessage) bool {
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(raw, &keys)
	for k := range keys {
		for _, declared := range callKeys {
			if k != declared && strings.EqualFold(k, declared) {
				return true
			}
		}
	}
	return false
}

func fieldsOf(fromArgs []string, meta mcp.Meta) []string {
	if len(fromArgs) > 0 {
		return fromArgs
	}
	var out []string
	list, _ := meta[fieldsMetaKey].([]any)
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (g *Gateway) run(ctx context.Context, p port.Principal, d *deliveries, req service.CallRequest) (service.Outcome, error) {
	out, err := g.core.Call(ctx, p, req)
	if err != nil {
		return out, g.failure(err)
	}
	if out, err = g.hold(ctx, p, out); err != nil {
		return out, g.failure(err)
	}
	owed(d, out)
	return out, nil
}

func (g *Gateway) forward(ctx context.Context, p port.Principal, d *deliveries, req service.CallRequest) (*mcp.CallToolResult, error) {
	out, err := g.run(ctx, p, d, req)
	if err != nil {
		return nil, err
	}
	return g.answer(req.Name, out, p.Token)
}

func (g *Gateway) hold(ctx context.Context, p port.Principal, out service.Outcome) (service.Outcome, error) {
	deadline := time.Now().Add(g.cfg.HoldWindow)
	for out.Status == service.OutcomePending && out.Approval != nil && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return out, nil
		case <-time.After(g.cfg.HoldPoll):
		}
		var err error
		if out, err = g.core.ApprovalStatus(ctx, p, out.Approval.ID); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (g *Gateway) approval(ctx context.Context, p port.Principal, d *deliveries, tool string, raw json.RawMessage) (*mcp.CallToolResult, error) {
	var a approvalArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.ApprovalID == "" {
		return toolError(port.CodeInvalidArgument, tool+" takes an approval_id"), nil
	}
	ask := g.core.ApprovalStatus
	if tool == port.ToolApprovalCancel {
		ask = g.core.ApprovalCancelByAgent
	}
	out, err := ask(ctx, p, port.ApprovalID(a.ApprovalID))
	if port.IsCode(err, port.CodeNotFound) {
		return toolError(port.CodeNotFound, "no such approval for this agent and session"), nil
	}
	if err != nil {
		return nil, g.failure(err)
	}
	if tool == port.ToolApprovalCancel && cancelled(out) {
		return structured(map[string]any{"status": "cancelled", "approval_id": a.ApprovalID}), nil
	}
	owed(d, out)
	return g.answer(tool, out, p.Token)
}
