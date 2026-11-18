package gateway

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolFind           = "find"
	toolCall           = "call"
	toolApprovalStatus = "approval_status"
	toolApprovalCancel = "approval_cancel"
)

func object(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var approvalID = map[string]any{"type": "string", "description": "The approval_id of a call that waits for approval."}

var searchTools = []*mcp.Tool{
	{Name: toolFind, Description: "Searches the functions granted to you by words and returns their definitions; call one with the call tool.",
		InputSchema: object([]string{"query"}, map[string]any{"query": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1}})},
	{Name: toolCall, Description: "Calls a function returned by find. fields lists JSON Pointer paths of the result to keep.",
		InputSchema: object([]string{"name"}, map[string]any{"name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"},
			"fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})},
}

var approvalTools = []*mcp.Tool{
	{Name: toolApprovalStatus, Description: "Returns the state, or the result once there is one, of a call that waits for a person's approval.",
		InputSchema: object([]string{"approval_id"}, map[string]any{"approval_id": approvalID})},
	{Name: toolApprovalCancel, Description: "Withdraws a call that waits for a person's approval.",
		InputSchema: object([]string{"approval_id"}, map[string]any{"approval_id": approvalID})},
}

type findArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type callArgs struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Fields    []string        `json:"fields"`
}

type approvalArgs struct {
	ApprovalID string `json:"approval_id"`
}
