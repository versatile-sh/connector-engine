package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Field is one JSON field of a struct type, with the index path to reach it.
type Field struct {
	Name      string
	Type      reflect.Type
	Index     []int
	OmitEmpty bool
}

// Fields lists the JSON fields of a struct type, flattening embedded structs and skipping unexported and "-" fields.
func Fields(t reflect.Type) []Field {
	var out []Field
	for i := range t.NumField() {
		f := t.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case f.Anonymous && f.Type.Kind() == reflect.Struct && name == "":
			for _, in := range Fields(f.Type) {
				in.Index = append([]int{i}, in.Index...)
				out = append(out, in)
			}
		case f.IsExported() && name != "-":
			if name == "" {
				name = f.Name
			}
			out = append(out, Field{name, f.Type, []int{i}, strings.Contains(opts, "omitempty")})
		}
	}
	return out
}

// Elem strips pointer levels from t.
func Elem(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// RawType is json.RawMessage, carried as any JSON value.
var RawType = reflect.TypeFor[json.RawMessage]()

// TimeType is the Go type of timestamps, carried as RFC 3339 strings and google.protobuf.Timestamp.
var TimeType = reflect.TypeFor[time.Time]()

// Models lists, by name, every named struct reachable from the operations' inputs and outputs; a name used by two types is an error.
func Models() ([]reflect.Type, error) {
	seen := map[string]reflect.Type{}
	var walk func(t reflect.Type) error
	walk = func(t reflect.Type) error {
		t = Elem(t)
		switch t.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return walk(t.Elem())
		case reflect.Struct:
			if t == TimeType || t.Name() == "" {
				return nil
			}
			if prev, ok := seen[t.Name()]; ok {
				if prev != t {
					return fmt.Errorf("%s and %s share the name %s", prev, t, t.Name())
				}
				return nil
			}
			seen[t.Name()] = t
			for _, f := range Fields(t) {
				if err := walk(f.Type); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, o := range operations {
		for _, t := range []reflect.Type{reflect.TypeOf(o.In()), reflect.TypeOf(o.Out)} {
			if err := walk(t); err != nil {
				return nil, err
			}
		}
	}
	out := make([]reflect.Type, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b reflect.Type) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

// IsBytes reports whether t is a byte slice that is not a raw JSON value.
func IsBytes(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 && t != RawType
}
