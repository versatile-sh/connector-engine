package projection_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/core/projection"
	"github.com/versatile-sh/connector-engine/port"
)

const schema = `{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"id":{"type":"integer"},"a/b":{"type":"string"}}}},"total":{"type":"integer"},"meta":{"anyOf":[{"type":"object","properties":{"x":{}}}]}}}`

func TestValidate(t *testing.T) {
	if err := projection.Validate([]byte(schema), []string{"/items/0/id", "/total", "/items/1/a~1b", "/meta/x"}, 20); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/nope", "items", "/items/x/id", "/total~2"} {
		if err := projection.Validate([]byte(schema), []string{p}, 20); !port.IsCode(err, port.CodeInvalidPath) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if projection.Validate(nil, []string{"/total"}, 20) == nil {
		t.Fatal("no outputSchema, not projectable")
	}
	if projection.Validate([]byte(schema), []string{"/total", "/items"}, 1) == nil {
		t.Fatal("projection.maxPaths")
	}
}

func TestProject(t *testing.T) {
	in := `{"items":[{"id":12345678901234567890,"a/b":"x","big":"drop"},{"id":2},{"id":3}],"total":3,"secret":"s"}`
	out, err := projection.Project([]byte(in), []string{"/items/0/id", "/items/2", "/total", "/missing"})
	if err != nil || string(out) != `{"items":[{"id":12345678901234567890},{"id":3}],"total":3}` {
		t.Fatalf("%s %v", out, err)
	}
	if out, _ := projection.Project([]byte(in), []string{"/missing"}); string(out) != `{}` {
		t.Fatalf("nothing kept: %s", out)
	}
}
