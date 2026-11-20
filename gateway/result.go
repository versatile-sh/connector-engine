package gateway

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/internal/tokenscan"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

const redacted = "[redacted]"

func text(s string) []mcp.Content { return []mcp.Content{&mcp.TextContent{Text: s}} }

func structured(v any) *mcp.CallToolResult {
	raw, _ := json.Marshal(v)
	return &mcp.CallToolResult{Content: text(string(raw)), StructuredContent: v}
}

func toolError(code port.Code, detail string) *mcp.CallToolResult {
	r := structured(map[string]any{"code": code, "detail": detail})
	r.Content, r.IsError, r.Meta = text(string(code)+": "+detail), true, mcp.Meta{port.ErrorMeta: code}
	return r
}

func resultOf(res port.Result, token string) (*mcp.CallToolResult, error) {
	content := tokenscan.Scrub(res.Content, token, redacted)
	if len(content) == 0 {
		content = json.RawMessage("[]")
	}
	wire := map[string]any{"content": content, "isError": res.IsError}
	if len(res.Structured) > 0 {
		wire["structuredContent"] = tokenscan.Scrub(res.Structured, token, redacted)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	var out mcp.CallToolResult
	return &out, json.Unmarshal(raw, &out)
}

func pendingResult(out service.Outcome) *mcp.CallToolResult {
	poll := max(int(out.PollAfter/time.Second), 1)
	r := structured(map[string]any{"status": "pending_approval", "approval_id": out.Approval.ID, "poll_after_seconds": poll, "expires_at": out.Approval.ExpiresAt.UTC().Format(time.RFC3339)})
	r.Content = text("This call waits for a person's approval. Ask approval_status with the approval_id after " + out.PollAfter.String() + ".")
	return r
}

func errorResult(out service.Outcome) *mcp.CallToolResult {
	body := map[string]any{"code": out.Code, "detail": out.Detail}
	if len(out.Scopes) > 0 {
		body["scopes"] = out.Scopes
	}
	if out.ConnectAddress {
		body["connect_address"] = true
	}
	if out.Approval != nil {
		body["approval_id"] = out.Approval.ID
	}
	r := structured(body)
	r.Content, r.IsError, r.Meta = text(string(out.Code)+": "+out.Detail), true, mcp.Meta{port.ErrorMeta: out.Code}
	return r
}

func cancelled(out service.Outcome) bool {
	return out.Approval != nil && out.Approval.State == port.ApprovalCancelled && out.Status == service.OutcomeError
}

func (g *Gateway) answer(name string, out service.Outcome, token string) (*mcp.CallToolResult, error) {
	switch out.Status {
	case service.OutcomeResult:
		return resultOf(*out.Result, token)
	case service.OutcomePending:
		return pendingResult(out), nil
	case service.OutcomeUnknownTool:
		return nil, unknownTool(name)
	}
	if token != "" {
		out.Detail = strings.ReplaceAll(out.Detail, token, redacted)
	}
	return errorResult(out), nil
}
