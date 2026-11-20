package console

import (
	"encoding/json"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// ItemKind names what a connector item is in the console: the server's instructions, a prompt or a resource.
type ItemKind string

// The three kinds of a connector item.
const (
	ItemInstructions ItemKind = "instructions"
	ItemPrompt       ItemKind = "prompt"
	ItemResource     ItemKind = "resource"
)

var itemKinds = map[string]ItemKind{port.ItemInstructions: ItemInstructions, port.ItemPrompt: ItemPrompt, port.ItemResource: ItemResource}

// Item is one prompt, resource or the server instructions of a connector, with the text the owner reads before choosing and the choice: proposed before an import, read back from the grants after.
type Item struct {
	Name        string   `json:"name"`
	Kind        ItemKind `json:"kind"`
	Label       string   `json:"label"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Preview     string   `json:"preview"`
	Template    bool     `json:"template,omitempty"`
	State       string   `json:"state,omitempty"`
	Access      Access   `json:"access"`
}

// Proposed is the console's pre-filled choice for an item (SK-1): resources and instructions on, prompts off, since prompt text reaches the chat as the person's own.
func Proposed(kind ItemKind) Access {
	if kind == ItemPrompt {
		return AccessOff
	}
	return AccessOn
}

func validFor(kind ItemKind, a Access) bool {
	return a.Valid() && (kind != ItemInstructions || a != AccessAsk)
}

// ItemOf reads an item from its engine name and its list entry as the server sent it, and proposes the choice of its kind.
func ItemOf(name string, def json.RawMessage) (Item, bool) {
	kind, ok := itemKinds[port.ItemKind(name)]
	if !ok {
		return Item{}, false
	}
	var d struct {
		Name         string `json:"name"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
		URI          string `json:"uri"`
		URITemplate  string `json:"uriTemplate"`
		MIMEType     string `json:"mimeType"`
		Arguments    []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
		} `json:"arguments"`
	}
	_ = json.Unmarshal(def, &d)
	it := Item{Name: name, Kind: kind, Title: d.Title, Description: d.Description, Access: Proposed(kind)}
	lines := []string{d.Description}
	switch kind {
	case ItemInstructions:
		it.Preview = d.Instructions
		return it, true
	case ItemPrompt:
		it.Label = d.Name
		for _, a := range d.Arguments {
			arg := "- " + a.Name
			if a.Required {
				arg += " (required)"
			}
			if a.Description != "" {
				arg += ": " + a.Description
			}
			lines = append(lines, arg)
		}
	case ItemResource:
		it.Label, it.Template = firstOf(d.URI, d.URITemplate), d.URITemplate != ""
		if d.MIMEType != "" {
			lines = append(lines, d.MIMEType)
		}
	}
	it.Preview = strings.TrimSpace(strings.Join(lines, "\n"))
	return it, true
}
