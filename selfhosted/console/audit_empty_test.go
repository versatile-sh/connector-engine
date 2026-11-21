package console_test

import (
	"encoding/json"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestAFreshConsoleAuditEncodesAsAnEmptyList(t *testing.T) {
	s, err := console.OpenState(t.TempDir()+"/s.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(s.Audit(50))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("Audit on a fresh console encodes as %s, want []", raw)
	}
}
