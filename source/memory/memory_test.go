package memory_test

import (
	"context"
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/memory"
)

func TestServesManifestsAsGivenAndPages(t *testing.T) {
	a := port.Manifest{Name: "a.b/one", Version: "1", TrustLevel: port.TrustFirstParty}
	b := port.Manifest{Name: "a.b/two", Version: "1"}
	src := memory.New("fake", 1, a, b)
	page, err := src.List(context.Background(), "")
	if err != nil || len(page.Manifests) != 1 || page.Manifests[0].TrustLevel != port.TrustFirstParty || page.Next == "" {
		t.Fatalf("page: %+v %v", page, err)
	}
	src.Set(b)
	if m, err := src.Fetch(context.Background(), "a.b/two", ""); err != nil || m.Name != "a.b/two" {
		t.Fatalf("fetch: %v %v", m, err)
	}
	if _, err := src.Fetch(context.Background(), "a.b/one", ""); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("a removed manifest: %v", err)
	}
}
