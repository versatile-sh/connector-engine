package console

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

const redactedValue = "[redacted]"

var (
	secretKey   = regexp.MustCompile(`(?i)token|passw|secret|authori[sz]ation|api[-_]?key|bearer|cookie|credential|private[-_]?key|signature|session[-_]?id|^sid$|csrf|xsrf`)
	countKey    = regexp.MustCompile(`(?i)tokens|token_?count`)
	secretValue = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}` +
		`|\beyJ[a-z0-9_-]{8,}\.[a-z0-9_-]{8,}\.[a-z0-9_-]{8,}` +
		`|\b(?:sk|pk|rk)[-_][a-z0-9_-]{16,}` +
		`|\bgh[pousr]_[a-z0-9]{20,}|\bgithub_pat_[a-z0-9_]{20,}|\bglpat-[a-z0-9_-]{20,}` +
		`|\bxox[abprs]-[a-z0-9-]{10,}|\bakia[0-9a-z]{16}\b|\baiza[0-9a-z_-]{35}|\bya29\.[a-z0-9_-]{20,}` +
		`|\bbasic\s+[a-z0-9+/=]{8,}` +
		`|[a-z0-9_-]*(?:password|passwd|pwd|secret|api[-_]?key|token|authorization|session[-_]?id)\\?["']?\s*[:=]\s*\\?["']?(?:(?:basic|bearer)\s+)?[^\s"',;\\]{4,}`)
)

func hidden(key string, v any) bool {
	switch v.(type) {
	case nil, bool:
		return false
	case json.Number:
		return !countKey.MatchString(key)
	}
	return true
}

func redactAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			if secretKey.MatchString(k) && hidden(k, inner) {
				t[k] = redactedValue
				continue
			}
			t[k] = redactAny(inner)
		}
	case []any:
		for i := range t {
			t[i] = redactAny(t[i])
		}
	case string:
		if inner, ok := embeddedJSON(t); ok {
			return inner
		}
		return secretValue.ReplaceAllString(t, redactedValue)
	}
	return v
}

func embeddedJSON(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return "", false
	}
	return Redact([]byte(trimmed))
}

// Redact returns the JSON value raw with likely secrets replaced: values under secret-looking keys (but booleans and token counts), and bearer, JWT or API-key shaped strings; what does not parse yields false.
func Redact(raw []byte) (string, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return "", false
	}
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactAny(v)); err != nil {
		return "", false
	}
	return strings.TrimSuffix(out.String(), "\n"), true
}

func redactText(s string) string { return secretValue.ReplaceAllString(s, redactedValue) }

func cut(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	i := max
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i], true
}
