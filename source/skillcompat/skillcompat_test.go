package skillcompat_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/skillcompat"
)

type cases struct {
	Analyze []struct {
		Name    string            `json:"name"`
		SkillMD string            `json:"skill_md"`
		Paths   []string          `json:"paths"`
		Files   map[string]string `json:"files"`
		Want    port.SkillNeeds   `json:"want"`
	} `json:"analyze"`
	AllowedTools []struct {
		SkillMD string   `json:"skill_md"`
		Want    []string `json:"want"`
	} `json:"allowed_tools"`
	Compat []struct {
		Name       string            `json:"name"`
		Needs      port.SkillNeeds   `json:"needs"`
		HasScripts bool              `json:"has_scripts"`
		Want       map[string][2]any `json:"want"`
	} `json:"compat"`
	ChatStatus []struct {
		Needs      port.SkillNeeds `json:"needs"`
		HasScripts bool            `json:"has_scripts"`
		Want       string          `json:"want"`
	} `json:"chat_status"`
}

func load(t *testing.T) cases {
	t.Helper()
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c cases
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAnalyze(t *testing.T) {
	for _, c := range load(t).Analyze {
		t.Run(c.Name, func(t *testing.T) {
			got := skillcompat.Analyze(c.SkillMD, c.Paths, func(rel string) (string, bool) { s, ok := c.Files[rel]; return s, ok })
			if !reflect.DeepEqual(got, c.Want) {
				t.Fatalf("got %+v, want %+v", got, c.Want)
			}
		})
	}
}

func TestAllowedTools(t *testing.T) {
	for _, c := range load(t).AllowedTools {
		if got := skillcompat.AllowedTools(c.SkillMD); !reflect.DeepEqual(got, c.Want) {
			t.Errorf("%q: got %v, want %v", c.SkillMD, got, c.Want)
		}
	}
}

func TestCompatMatchesCatalogFixtures(t *testing.T) {
	for _, c := range load(t).Compat {
		t.Run(c.Name, func(t *testing.T) {
			got := map[string]port.SkillCompat{}
			for _, e := range skillcompat.Compat(c.Needs, c.HasScripts) {
				got[e.Client] = e
				if e.Source != "derived" {
					t.Errorf("%s source is %q, not derived", e.Client, e.Source)
				}
			}
			for client, want := range c.Want {
				e := got[client]
				reasons, _ := json.Marshal(e.Reasons)
				wantReasons, _ := json.Marshal(want[1])
				if e.Status != want[0] || string(reasons) != string(wantReasons) {
					t.Errorf("%s: got %s %s, want %v", client, e.Status, reasons, want)
				}
			}
		})
	}
}

func TestChatStatus(t *testing.T) {
	for _, c := range load(t).ChatStatus {
		if got := skillcompat.ChatStatus(c.Needs, c.HasScripts); got != c.Want {
			t.Errorf("got %s, want %s", got, c.Want)
		}
	}
}

func TestClientTableMatchesCatalog(t *testing.T) {
	raw, err := os.ReadFile("testdata/clients.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Clients []skillcompat.Client `json:"clients"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(file.Clients, skillcompat.Clients) {
		t.Fatalf("the client table drifted from the catalog's clients.json")
	}
}
