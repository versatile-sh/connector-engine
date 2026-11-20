package service

import (
	"encoding/json"

	"github.com/agents-control/connector-engine/port"
)

// ServedInstructions is the accepted server instructions text of one connector the agent can reach, under the connector's alias.
type ServedInstructions struct {
	Alias     string
	Connector port.ConnectorID
	Text      string
}

func (s *Service) serveItem(out *FunctionList, c port.Connector, f port.Function, mode port.Mode) {
	if port.ItemKind(f.Name) == port.ItemInstructions && s.cfg.RelayInstructions && mode == port.ModeAllow {
		var d struct {
			Instructions string `json:"instructions"`
		}
		_ = json.Unmarshal(f.Definition, &d)
		out.Instructions = append(out.Instructions, ServedInstructions{Alias: c.Alias, Connector: c.ID, Text: d.Instructions})
	}
}
