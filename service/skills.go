package service

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

// ServedSkill is one entry of skill_list: a skill granted to the agent, named by its connector alias, with the required connectors the agent has no binding to.
type ServedSkill struct {
	Name           string
	Connector      port.ConnectorID
	Function       string
	Title          string
	Description    string
	Mode           port.Mode
	HasScripts     bool
	Redistribution port.Redistribution
	Requires       []string
	Unbound        []string
}

func (s *Service) isSkill(tx port.Tx, c port.Connector) (bool, error) {
	if v, ok := s.kinds.Load(c.Listing); ok {
		return v.(bool), nil
	}
	lv, err := tx.GetVersion(c.Listing)
	if err != nil {
		return false, err
	}
	skill := lv.Manifest.Kind == port.KindSkill
	s.kinds.Store(c.Listing, skill)
	return skill, nil
}

func servedSkill(c port.Connector, f port.Function, mode port.Mode) ServedSkill {
	var d struct {
		Title       string                     `json:"title"`
		Description string                     `json:"description"`
		Meta        map[string]json.RawMessage `json:"_meta"`
	}
	_ = json.Unmarshal(f.Definition, &d)
	var m port.SkillMeta
	_ = json.Unmarshal(d.Meta[port.SkillMetaKey], &m)
	return ServedSkill{Name: c.Alias, Connector: c.ID, Function: f.Name, Title: d.Title, Description: d.Description, Mode: mode,
		HasScripts: m.HasScripts, Redistribution: m.Redistribution, Requires: m.Requires}
}

func markUnbound(skills []ServedSkill, bound map[string]bool) {
	for i := range skills {
		skills[i].Unbound = port.Missing(skills[i].Requires, bound)
	}
	slices.SortFunc(skills, func(a, b ServedSkill) int { return cmp.Compare(a.Name, b.Name) })
}

func (s *Service) resolveSkill(tx port.Tx, conns []port.Connector, name string) (port.Connector, port.Function, bool, error) {
	for _, c := range conns {
		if c.Alias != name {
			continue
		}
		if skill, err := s.isSkill(tx, c); err != nil || !skill {
			return c, port.Function{}, false, err
		}
		fns, err := tx.ListFunctions(c.ID)
		if err != nil {
			return c, port.Function{}, false, err
		}
		for _, f := range fns {
			if f.State != port.FunctionRemoved {
				return c, f, true, nil
			}
		}
	}
	return port.Connector{}, port.Function{}, false, nil
}
