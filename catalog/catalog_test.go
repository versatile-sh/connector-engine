package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/catalog"
)

func TestEveryTypeHasASchema(t *testing.T) {
	all, err := catalog.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Published) != 40 || len(catalog.Consumed) != 3 || len(all) != 43 {
		t.Fatalf("%d published, %d consumed, want 40 and 3", len(catalog.Published), len(catalog.Consumed))
	}
	seen := map[string]bool{}
	for i, typ := range all {
		if seen[typ.Name] {
			t.Fatalf("%s twice", typ.Name)
		}
		seen[typ.Name] = true
		if !typ.Consumed && (typ.Number != i+1 || !strings.HasPrefix(typ.Name, "connector.")) {
			t.Fatalf("row %d is %d %s", i+1, typ.Number, typ.Name)
		}
		var s struct {
			ID         string                     `json:"$id"`
			Title      string                     `json:"title"`
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(typ.Schema, &s); err != nil || s.Title != typ.Name || s.Type != "object" || len(s.Properties) == 0 || !strings.HasSuffix(s.ID, typ.Name) {
			t.Fatalf("%s has no usable JSON Schema: %s (%v)", typ.Name, typ.Schema, err)
		}
		if strings.Contains(string(typ.Schema), `"arguments"`) || strings.Contains(string(typ.Schema), `"result"`) {
			t.Fatalf("arguments and results are never in an event: %s", typ.Name)
		}
	}
	for _, name := range []string{"connector.overlay.changed", "connector.overlay.suggested", "connector.connector.blocked_changed"} {
		if !catalog.Known(name) {
			t.Fatalf("%s missing", name)
		}
	}
	for _, name := range catalog.InstallationTypes {
		if !catalog.Known(name) {
			t.Fatalf("installation stream type %s is not published", name)
		}
	}
}

func TestCallFinishedCarriesUsage(t *testing.T) {
	typ, ok, err := catalog.Get("connector.call.finished")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var s struct {
		Usage      map[string]any            `json:"x-usage"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(typ.Schema, &s); err != nil {
		t.Fatal(err)
	}
	if s.Usage["count"] != "call" || s.Properties["duration_ms"]["x-usage"] == nil || s.Properties["agent"]["x-usage"] == nil || s.Properties["on_behalf_of"]["x-usage"] == nil {
		t.Fatalf("event 20: one call, attributed to the agent and the person acted for: %s", typ.Schema)
	}
	other, _, _ := catalog.Get("connector.call.started")
	if strings.Contains(string(other.Schema), "x-usage") {
		t.Fatal("only call.finished counts usage")
	}
}
