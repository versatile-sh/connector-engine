package gateway

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxInstructions = 4096
	indexHead       = "\n\nSkills you can load with skill_load (skill_list has them all):"
	indexMore       = "\n- more skills: call skill_list."
)

type listedSkill struct {
	Name           string              `json:"name"`
	Title          string              `json:"title,omitempty"`
	Description    string              `json:"description,omitempty"`
	Access         string              `json:"access"`
	HasScripts     bool                `json:"has_scripts,omitempty"`
	Redistribution port.Redistribution `json:"redistribution,omitempty"`
	Note           string              `json:"note,omitempty"`
}

func note(s service.ServedSkill) string {
	var parts []string
	if len(s.Unbound) > 0 {
		parts = append(parts, "needs connector "+strings.Join(s.Unbound, ", ")+", not connected")
	}
	if s.Redistribution == port.RedistributionMetadataOnly {
		parts = append(parts, "listed without its files (no redistributable licence)")
	}
	return strings.Join(parts, "; ")
}

func listed(skills []service.ServedSkill) []listedSkill {
	out := make([]listedSkill, len(skills))
	for i, s := range skills {
		access := "on"
		if s.Mode != port.ModeAllow {
			access = "ask"
		}
		out[i] = listedSkill{Name: s.Name, Title: s.Title, Description: s.Description, Access: access, HasScripts: s.HasScripts, Redistribution: s.Redistribution, Note: note(s)}
	}
	return out
}

func (g *Gateway) skill(ctx context.Context, p port.Principal, d *deliveries, tool string, raw json.RawMessage) (*mcp.CallToolResult, error) {
	if tool == toolSkillList {
		fl, err := g.core.ListFunctions(ctx, p)
		if err != nil {
			return nil, g.failure(err)
		}
		return structured(map[string]any{"skills": listed(fl.Skills)}), nil
	}
	var a skillArgs
	if err := json.Unmarshal(raw, &a); err != nil || a.Skill == "" {
		return toolError(port.CodeInvalidArgument, "skill_load takes a skill and an optional path"), nil
	}
	args, _ := json.Marshal(map[string]string{"path": a.Path})
	return g.forward(ctx, p, d, service.CallRequest{Name: a.Skill, Arguments: args, Skill: true})
}

// SkillIndex is the engine's instructions followed by one line per skill, cut to 4,096 characters with a pointer to skill_list.
func SkillIndex(base string, skills []service.ServedSkill) string {
	if len(skills) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(base + indexHead)
	for _, s := range listed(skills) {
		line := "\n- " + s.Name + ": " + strings.Join(strings.Fields(s.Description), " ")
		if s.Note != "" {
			line += " (" + s.Note + ")"
		}
		if b.Len()+len(line)+len(indexMore) > maxInstructions {
			b.WriteString(indexMore)
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func (g *Gateway) instructed(ctx context.Context, req mcp.Request, res mcp.Result) {
	var instructions *string
	switch r := res.(type) {
	case *mcp.InitializeResult:
		instructions = &r.Instructions
	case *mcp.DiscoverResult:
		instructions = &r.Instructions
	default:
		return
	}
	p, err := principalOf(req)
	if err != nil {
		return
	}
	if fl, err := g.core.ListFunctions(ctx, p); err == nil {
		*instructions = SkillIndex(*instructions, fl.Skills)
	}
}
