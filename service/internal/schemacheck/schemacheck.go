// Package schemacheck validates call arguments against the JSON Schema subset connector input schemas use.
package schemacheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"unicode/utf8"
)

type schema struct {
	Type                 json.RawMessage            `json:"type"`
	Enum                 []any                      `json:"enum"`
	Const                *any                       `json:"const"`
	Required             []string                   `json:"required"`
	Properties           map[string]json.RawMessage `json:"properties"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	Items                json.RawMessage            `json:"items"`
	Minimum              *float64                   `json:"minimum"`
	Maximum              *float64                   `json:"maximum"`
	MinLength            *int                       `json:"minLength"`
	MaxLength            *int                       `json:"maxLength"`
	MinItems             *int                       `json:"minItems"`
	MaxItems             *int                       `json:"maxItems"`
	AnyOf                []json.RawMessage          `json:"anyOf"`
	OneOf                []json.RawMessage          `json:"oneOf"`
	AllOf                []json.RawMessage          `json:"allOf"`
}

// Validate returns an error naming the first invalid field of args, or nil; an empty schema accepts anything.
func Validate(raw json.RawMessage, args json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var v any
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	if err := json.Unmarshal(args, &v); err != nil {
		return fmt.Errorf("arguments are not JSON: %v", err)
	}
	return check(raw, v, "")
}

func at(path string) string {
	if path == "" {
		return "arguments"
	}
	return path
}

func check(raw json.RawMessage, v any, path string) error {
	var s schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	if err := checkType(s.Type, v, path); err != nil {
		return err
	}
	if s.Enum != nil && !slices.ContainsFunc(s.Enum, func(e any) bool { return reflect.DeepEqual(e, v) }) {
		return fmt.Errorf("%s is not one of the allowed values", at(path))
	}
	if s.Const != nil && !reflect.DeepEqual(*s.Const, v) {
		return fmt.Errorf("%s is not the required constant", at(path))
	}
	if err := checkCombinators(s, v, path); err != nil {
		return err
	}
	switch x := v.(type) {
	case map[string]any:
		return checkObject(s, x, path)
	case []any:
		return checkArray(s, x, path)
	case string:
		if n := utf8.RuneCountInString(x); (s.MinLength != nil && n < *s.MinLength) || (s.MaxLength != nil && n > *s.MaxLength) {
			return fmt.Errorf("%s has a length outside the allowed range", at(path))
		}
	case float64:
		if (s.Minimum != nil && x < *s.Minimum) || (s.Maximum != nil && x > *s.Maximum) {
			return fmt.Errorf("%s is outside the allowed range", at(path))
		}
	}
	return nil
}

func checkCombinators(s schema, v any, path string) error {
	for _, sub := range s.AllOf {
		if err := check(sub, v, path); err != nil {
			return err
		}
	}
	matches := func(list []json.RawMessage) int {
		n := 0
		for _, sub := range list {
			if check(sub, v, path) == nil {
				n++
			}
		}
		return n
	}
	if len(s.AnyOf) > 0 && matches(s.AnyOf) == 0 {
		return fmt.Errorf("%s matches none of the allowed shapes", at(path))
	}
	if len(s.OneOf) > 0 && matches(s.OneOf) != 1 {
		return fmt.Errorf("%s matches %d of the allowed shapes, one is required", at(path), matches(s.OneOf))
	}
	return nil
}

func checkObject(s schema, o map[string]any, path string) error {
	for _, r := range s.Required {
		if _, ok := o[r]; !ok {
			return fmt.Errorf("%s is required", join(path, r))
		}
	}
	for _, k := range slices.Sorted(keys(o)) {
		sub, known := s.Properties[k]
		if !known {
			if bytes.Equal(bytes.TrimSpace(s.AdditionalProperties), []byte("false")) {
				return fmt.Errorf("%s is not allowed", join(path, k))
			}
			if len(s.AdditionalProperties) == 0 || bytes.Equal(bytes.TrimSpace(s.AdditionalProperties), []byte("true")) {
				continue
			}
			sub = s.AdditionalProperties
		}
		if err := check(sub, o[k], join(path, k)); err != nil {
			return err
		}
	}
	return nil
}

func checkArray(s schema, a []any, path string) error {
	if (s.MinItems != nil && len(a) < *s.MinItems) || (s.MaxItems != nil && len(a) > *s.MaxItems) {
		return fmt.Errorf("%s has a number of items outside the allowed range", at(path))
	}
	if len(s.Items) == 0 {
		return nil
	}
	for i, e := range a {
		if err := check(s.Items, e, fmt.Sprintf("%s[%d]", at(path), i)); err != nil {
			return err
		}
	}
	return nil
}

func keys(m map[string]any) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

func checkType(raw json.RawMessage, v any, path string) error {
	if len(raw) == 0 {
		return nil
	}
	var types []string
	var one string
	if json.Unmarshal(raw, &one) == nil {
		types = []string{one}
	} else if json.Unmarshal(raw, &types) != nil {
		return nil
	}
	if slices.ContainsFunc(types, func(t string) bool { return isType(t, v) }) {
		return nil
	}
	return fmt.Errorf("%s must be of type %v", at(path), types)
}

func isType(t string, v any) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	}
	return true
}
