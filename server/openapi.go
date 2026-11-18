package server

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type doc = map[string]any

func ref(name string) doc { return doc{"$ref": "#/components/" + name} }

func schemaOf(t reflect.Type) doc { return shapeOf(t, false) }

// InlineSchema is the JSON Schema of a type with every named struct expanded in place, for clients without components.
func InlineSchema(t reflect.Type) map[string]any { return shapeOf(t, true) }

func shapeOf(t reflect.Type, inline bool) doc {
	t = Elem(t)
	switch {
	case t == RawType:
		return doc{}
	case IsBytes(t):
		return doc{"type": "string", "contentEncoding": "base64"}
	}
	switch t.Kind() {
	case reflect.Struct:
		switch {
		case t == TimeType:
			return doc{"type": "string", "format": "date-time"}
		case t.Name() != "" && !inline:
			return ref("schemas/" + t.Name())
		}
		return objectOf(Fields(t), nil, inline)
	case reflect.Slice, reflect.Array:
		return doc{"type": "array", "items": shapeOf(t.Elem(), inline)}
	case reflect.Map:
		if t.Elem().Kind() == reflect.Interface {
			return doc{"type": "object"}
		}
		return doc{"type": "object", "additionalProperties": shapeOf(t.Elem(), inline)}
	case reflect.Interface:
		return doc{}
	case reflect.String:
		return doc{"type": "string"}
	case reflect.Bool:
		return doc{"type": "boolean"}
	case reflect.Float32, reflect.Float64:
		return doc{"type": "number"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return doc{"type": "integer", "minimum": 0}
	}
	return doc{"type": "integer"}
}

func objectOf(fields []Field, skip []string, inline bool) doc {
	props, required := doc{}, []string{}
	for _, f := range fields {
		if slices.Contains(skip, f.Name) {
			continue
		}
		props[f.Name] = shapeOf(f.Type, inline)
		if !f.OmitEmpty {
			required = append(required, f.Name)
		}
	}
	out := doc{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func parameters(o Operation, fields []Field) []any {
	var out []any
	for _, name := range append(o.PathParams(), o.QueryParams()...) {
		in := "query"
		if slices.Contains(o.PathParams(), name) {
			in = "path"
		}
		f := fields[slices.IndexFunc(fields, func(f Field) bool { return f.Name == name })]
		out = append(out, doc{"name": name, "in": in, "required": in == "path", "schema": schemaOf(f.Type)})
	}
	if o.Method != http.MethodGet {
		out = append(out, ref("parameters/IdempotencyKey"))
	}
	return out
}

func responses(o Operation) doc {
	ok := doc{"description": "Success"}
	switch {
	case o.Status == http.StatusNoContent:
	case o.Live:
		ok["content"] = doc{"text/event-stream": doc{"schema": schemaOf(reflect.TypeOf(o.Out))}}
	default:
		ok["content"] = doc{"application/json": doc{"schema": schemaOf(reflect.TypeOf(o.Out))}}
	}
	return doc{fmt.Sprint(o.Status): ok, "default": ref("responses/Problem")}
}

func pathItem(o Operation) doc {
	fields := Fields(reflect.TypeOf(o.In()).Elem())
	item := doc{"operationId": o.ID, "tags": []string{"connector"}, "parameters": parameters(o, fields), "responses": responses(o)}
	if o.Public {
		item["security"] = []any{}
	}
	if o.HasBody() {
		body := objectOf(fields, append(o.PathParams(), o.QueryParams()...), false)
		if len(body["properties"].(doc)) > 0 {
			item["requestBody"] = doc{"content": doc{"application/json": doc{"schema": body}}}
		}
	}
	return item
}

// OpenAPI renders the operation table as an OpenAPI 3.1 document in YAML.
func OpenAPI() ([]byte, error) {
	models, err := Models()
	if err != nil {
		return nil, err
	}
	schemas := doc{}
	for _, t := range models {
		schemas[t.Name()] = objectOf(Fields(t), nil, false)
	}
	schemas["Problem"] = doc{"type": "object", "required": []string{"type", "title", "status", "detail", "code"}, "additionalProperties": true, "properties": doc{
		"type": doc{"type": "string"}, "title": doc{"type": "string"}, "status": doc{"type": "integer"}, "detail": doc{"type": "string"}, "code": doc{"type": "string"},
	}}
	paths := doc{}
	for _, o := range operations {
		item, _ := paths[o.Path].(doc)
		if item == nil {
			item = doc{}
			paths[o.Path] = item
		}
		item[strings.ToLower(o.Method)] = pathItem(o)
	}
	root := doc{
		"openapi":  "3.1.0",
		"info":     doc{"title": "connector-engine", "version": "1.0.0"},
		"security": []any{doc{"bearer": []string{}}},
		"paths":    paths,
		"components": doc{
			"schemas":         schemas,
			"securitySchemes": doc{"bearer": doc{"type": "http", "scheme": "bearer", "description": "A token of an issuer in auth.issuers[] for the audience auth.manageAudience; a token with an agent claim is refused."}},
			"parameters": doc{
				"IdempotencyKey": doc{"name": HeaderIdempotencyKey, "in": "header", "description": "A retry with the same key runs the mutation once.", "schema": doc{"type": "string"}},
			},
			"responses": doc{"Problem": doc{"description": "An RFC 9457 problem document.", "content": doc{ProblemMediaType: doc{"schema": ref("schemas/Problem")}}}},
		},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
