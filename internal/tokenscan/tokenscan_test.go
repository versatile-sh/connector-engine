package tokenscan_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/internal/tokenscan"
)

const token = "ab<cd>/ef"

func TestEscapedTokensAreFoundAndScrubbed(t *testing.T) {
	escaped := json.RawMessage(`{"a":["x ab<cd>\/ef y"],"ab<cd>/ef":1}`)
	if !tokenscan.Contains(escaped, token) {
		t.Fatal("a JSON-escaped token is found")
	}
	if tokenscan.Contains(json.RawMessage(`{"a":"clean"}`), token) {
		t.Fatal("a clean document holds no token")
	}
	out := tokenscan.Scrub(escaped, token, "[redacted]")
	if tokenscan.Contains(out, token) || !strings.Contains(string(out), "[redacted]") {
		t.Fatalf("scrubbed: %s", out)
	}
	if got := tokenscan.Scrub(json.RawMessage(`not json ab<cd>/ef`), token, "*"); string(got) != "not json *" {
		t.Fatalf("bytes fallback: %s", got)
	}
}
