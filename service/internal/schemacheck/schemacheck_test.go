package schemacheck_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/service/internal/schemacheck"
)

func TestValidate(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","required":["repo"],"additionalProperties":false,"properties":{"repo":{"type":"string","minLength":1},"limit":{"type":"integer","maximum":10},"tags":{"type":"array","items":{"enum":["a","b"]}}}}`)
	cases := map[string]string{
		`{"repo":"x"}`:                        "",
		`{"repo":"x","limit":3,"tags":["a"]}`: "",
		`{}`:                                  "repo is required",
		`{"repo":1}`:                          "repo must be of type",
		`{"repo":"x","limit":2.5}`:            "limit must be of type",
		`{"repo":"x","limit":11}`:             "limit is outside",
		`{"repo":"x","extra":1}`:              "extra is not allowed",
		`{"repo":"x","tags":["c"]}`:           "tags[0] is not one of",
	}
	for args, want := range cases {
		err := schemacheck.Validate(schema, json.RawMessage(args))
		if (want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), want)) {
			t.Errorf("%s: got %v, want %q", args, err, want)
		}
	}
	if err := schemacheck.Validate(nil, json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatalf("an empty schema accepts anything: %v", err)
	}
}
