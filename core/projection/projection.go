// Package projection validates RFC 6901 paths against a pinned outputSchema and trims structuredContent to them (§10.5).
package projection

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// Parse splits an RFC 6901 JSON Pointer into unescaped reference tokens.
func Parse(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, port.Errorf(port.CodeInvalidPath, "%q does not start with /", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	for i, p := range parts {
		for j := 0; j < len(p); j++ {
			if p[j] == '~' && (j+1 == len(p) || (p[j+1] != '0' && p[j+1] != '1')) {
				return nil, port.Errorf(port.CodeInvalidPath, "%q has a bad ~ escape", pointer)
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

type schema struct {
	Type                 any                `json:"type"`
	Properties           map[string]*schema `json:"properties"`
	AdditionalProperties json.RawMessage    `json:"additionalProperties"`
	Items                json.RawMessage    `json:"items"`
	PrefixItems          []*schema          `json:"prefixItems"`
	AllOf                []*schema          `json:"allOf"`
	AnyOf                []*schema          `json:"anyOf"`
	OneOf                []*schema          `json:"oneOf"`
}

func sub(raw json.RawMessage) *schema {
	var s schema
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return nil
	}
	return &s
}

func (s *schema) resolves(tokens []string) bool {
	if s == nil {
		return false
	}
	if len(tokens) == 0 {
		return true
	}
	for _, alt := range slices.Concat(s.AllOf, s.AnyOf, s.OneOf) {
		if alt.resolves(tokens) {
			return true
		}
	}
	tok, rest := tokens[0], tokens[1:]
	if p, ok := s.Properties[tok]; ok {
		return p.resolves(rest)
	}
	if i, err := strconv.Atoi(tok); err == nil && i >= 0 && tok == strconv.Itoa(i) {
		if i < len(s.PrefixItems) {
			return s.PrefixItems[i].resolves(rest)
		}
		if items := sub(s.Items); items != nil {
			return items.resolves(rest)
		}
	}
	return sub(s.AdditionalProperties).resolves(rest)
}

// Validate checks that each path resolves in the pinned outputSchema; a function without one is not projectable (§10.5).
func Validate(outputSchema json.RawMessage, paths []string, max int) error {
	if len(outputSchema) == 0 {
		return port.Errorf(port.CodeInvalidPath, "the function has no outputSchema and is not projectable")
	}
	if len(paths) > max {
		return port.Errorf(port.CodeInvalidPath, "%d paths, at most %d", len(paths), max)
	}
	root := sub(outputSchema)
	if root == nil {
		return port.Errorf(port.CodeInvalidPath, "the outputSchema is not a JSON Schema object")
	}
	for _, p := range paths {
		tokens, err := Parse(p)
		if err != nil {
			return err
		}
		if !root.resolves(tokens) {
			return port.Errorf(port.CodeInvalidPath, "%s does not resolve in the outputSchema", p)
		}
	}
	return nil
}

type keep struct {
	all  bool
	next map[string]*keep
}

func (k *keep) add(tokens []string) {
	for _, t := range tokens {
		if k.all {
			return
		}
		if k.next == nil {
			k.next = map[string]*keep{}
		}
		if k.next[t] == nil {
			k.next[t] = &keep{}
		}
		k = k.next[t]
	}
	k.all, k.next = true, nil
}

func (k *keep) trim(v any) (any, bool) {
	if k.all {
		return v, true
	}
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, sub := range k.next {
			if e, ok := x[key]; ok {
				if t, ok := sub.trim(e); ok {
					out[key] = t
				}
			}
		}
		return out, len(out) > 0
	case []any:
		var out []any
		for i, e := range x {
			if sub, ok := k.next[strconv.Itoa(i)]; ok {
				if t, ok := sub.trim(e); ok {
					out = append(out, t)
				}
			}
		}
		return out, len(out) > 0
	}
	return nil, false
}

// Project keeps only the named paths of structuredContent; kept array elements stay in index order, compacted.
func Project(structured json.RawMessage, paths []string) (json.RawMessage, error) {
	root := &keep{}
	for _, p := range paths {
		tokens, err := Parse(p)
		if err != nil {
			return nil, err
		}
		root.add(tokens)
	}
	d := json.NewDecoder(bytes.NewReader(structured))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "structuredContent is not JSON: %v", err)
	}
	out, ok := root.trim(v)
	if !ok {
		out = map[string]any{}
	}
	return json.Marshal(out)
}
