// Package builtin is the ResultFilter that masks personal data, Brazilian CPF and CNPJ, card numbers and common secrets, and counts what it masked per kind.
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

type rule struct {
	kind  string
	re    *regexp.Regexp
	valid func(string) bool
}

var secretPatterns = []string{
	`\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\b`,
	`\bgh[pousr]_[0-9a-zA-Z]{36}\b`,
	`\bgithub_pat_[0-9a-zA-Z_]{82}\b`,
	`\bglpat-[\w-]{20}\b`,
	`\bxox[baprs]-[0-9a-zA-Z-]{10,}`,
	`\b(?:sk|rk)_(?:test|live|prod)_[a-zA-Z0-9]{10,99}\b`,
	`\bAIza[\w-]{35}\b`,
	`\bey[a-zA-Z0-9]{17,}\.ey[a-zA-Z0-9/\\_-]{17,}\.[a-zA-Z0-9/\\_-]{10,}={0,2}`,
	`(?i)\bbearer\s+[a-z0-9._~+/=-]{20,}`,
	`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----[\s\S]{64,}?-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`,
}

var rules = []rule{
	{kind: "secret", re: regexp.MustCompile(`(?:` + strings.Join(secretPatterns, "|") + `)`)},
	{kind: "email", re: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)},
	{kind: "cnpj", re: regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`), valid: validCNPJ},
	{kind: "cpf", re: regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`), valid: validCPF},
	{kind: "card", re: regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`), valid: validCard},
	{kind: "phone", re: regexp.MustCompile(`\+\d[\d\s().-]{7,17}\d|(?:\(\d{2}\)\s?|\b\d{2}[\s.-])9?\d{4}[\s.-]?\d{4}\b|\(?\b\d{3}\)?[\s.-]\d{3}[\s.-]\d{4}\b`)},
}

// Filter masks the text blocks and the strings of the structured content of a result.
type Filter struct{}

// New returns the built-in filter.
func New() Filter { return Filter{} }

func redact(s string, found map[string]int) string {
	for _, r := range rules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			if r.valid != nil && !r.valid(m) {
				return m
			}
			found[r.kind]++
			return "[REDACTED:" + r.kind + "]"
		})
	}
	return s
}

func walk(v any, found map[string]int, textOnly bool) any {
	switch x := v.(type) {
	case string:
		if !textOnly {
			return redact(x, found)
		}
	case []any:
		for i, e := range x {
			x[i] = walk(e, found, textOnly)
		}
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && textOnly && k == "text" {
				x[k] = redact(s, found)
				continue
			}
			x[k] = walk(e, found, textOnly)
		}
	}
	return v
}

func scrub(raw json.RawMessage, found map[string]int, textOnly bool) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "result is not JSON: %v", err)
	}
	before := total(found)
	v = walk(v, found, textOnly)
	if total(found) == before {
		return raw, nil
	}
	return json.Marshal(v)
}

func total(m map[string]int) int {
	n := 0
	for _, c := range m {
		n += c
	}
	return n
}

// Filter returns the result with every finding replaced by [REDACTED:kind] and the count of findings per kind, never their values.
func (Filter) Filter(_ context.Context, r port.Result) (port.Result, map[string]int, error) {
	found := map[string]int{}
	content, err := scrub(r.Content, found, true)
	if err != nil {
		return port.Result{}, nil, err
	}
	structured, err := scrub(r.Structured, found, false)
	if err != nil {
		return port.Result{}, nil, err
	}
	r.Content, r.Structured = content, structured
	return r, found, nil
}
