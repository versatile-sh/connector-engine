// Package tokenscan finds and masks a bearer token inside the decoded strings of a JSON document, escapes included.
package tokenscan

import (
	"bytes"
	"encoding/json"
	"strings"
)

func walk(v any, fn func(string) string) any {
	switch x := v.(type) {
	case string:
		return fn(x)
	case []any:
		for i := range x {
			x[i] = walk(x[i], fn)
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[fn(k)] = walk(e, fn)
		}
		return out
	}
	return v
}

func decode(raw json.RawMessage) (any, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	return v, d.Decode(&v) == nil
}

// Contains reports whether token is in any decoded string or key of the document, or in its bytes when it does not parse.
func Contains(raw json.RawMessage, token string) bool {
	if token == "" {
		return false
	}
	v, ok := decode(raw)
	if !ok {
		return bytes.Contains(raw, []byte(token))
	}
	found := false
	walk(v, func(s string) string {
		found = found || strings.Contains(s, token)
		return s
	})
	return found
}

// Scrub replaces token with mask in every decoded string and key; a document that does not parse is scrubbed as bytes.
func Scrub(raw json.RawMessage, token, mask string) json.RawMessage {
	if !Contains(raw, token) {
		return raw
	}
	v, ok := decode(raw)
	if !ok {
		return bytes.ReplaceAll(raw, []byte(token), []byte(mask))
	}
	out, err := json.Marshal(walk(v, func(s string) string { return strings.ReplaceAll(s, token, mask) }))
	if err != nil {
		return bytes.ReplaceAll(raw, []byte(token), []byte(mask))
	}
	return out
}
