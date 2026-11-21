package args_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/core/args"
)

func TestParseRefusesDuplicateKeys(t *testing.T) {
	for name, in := range map[string]string{
		"top level":           `{"a":1,"a":2}`,
		"nested in object":    `{"o":{"x":1,"y":2,"x":3}}`,
		"in array of object":  `{"l":[{"k":1},{"k":1,"k":2}]}`,
		"deep array":          `{"l":[[{"q":{"z":1,"z":1}}]]}`,
		"escaped same key":    `{"a":1,"a":2}`,
		"empty key twice":     `{"":1,"":2}`,
		"duplicate after kid": `{"a":{"b":1},"a":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := args.Parse([]byte(in)); err == nil {
				t.Fatalf("%s accepted", in)
			}
		})
	}
}

func TestParseAcceptsWhatTheGatewayAcceptsToday(t *testing.T) {
	for name, in := range map[string]string{
		"absent":                  ``,
		"blank":                   "  \n",
		"null":                    `null`,
		"empty object":            `{}`,
		"same key in siblings":    `{"a":{"k":1},"b":{"k":2}}`,
		"same key in array items": `{"l":[{"k":1},{"k":2}]}`,
		"keys differ by case":     `{"a":1,"A":2}`,
		"keys differ by escape":   `{"a":1,"á":2}`,
		"trailing whitespace":     `{"a":1} ` + "\n",
		"scalars inside":          `{"a":[1,"x",null,true,{"b":[]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := args.Parse([]byte(in)); err != nil {
				t.Fatalf("%q refused: %v", in, err)
			}
		})
	}
}

func TestParseRefusesWhatIsNotAnObject(t *testing.T) {
	for _, in := range []string{`[]`, `[{"a":1}]`, `1`, `"x"`, `true`, `{"a":1} {"a":2}`, `{"a":1}x`, `{"a":1`, `{"a":}`, `{a:1}`, `{"a":1,}`} {
		if err := args.Parse([]byte(in)); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}
