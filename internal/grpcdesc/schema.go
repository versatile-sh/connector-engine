// Package grpcdesc loads protobuf descriptors and derives function definitions from their unary methods.
package grpcdesc

import (
	"encoding/json"

	"google.golang.org/protobuf/reflect/protoreflect"
)

const maxRecursion = 3

var wellKnown = map[protoreflect.FullName]string{
	"google.protobuf.Timestamp":   `{"type":"string","format":"date-time"}`,
	"google.protobuf.Duration":    `{"type":"string","pattern":"^-?[0-9]+(\\.[0-9]{1,9})?s$"}`,
	"google.protobuf.FieldMask":   `{"type":"string"}`,
	"google.protobuf.Struct":      `{"type":"object"}`,
	"google.protobuf.Value":       `{}`,
	"google.protobuf.ListValue":   `{"type":"array"}`,
	"google.protobuf.Any":         `{"type":"object","properties":{"@type":{"type":"string"}},"required":["@type"]}`,
	"google.protobuf.Empty":       `{"type":"object","additionalProperties":false}`,
	"google.protobuf.BoolValue":   `{"type":"boolean"}`,
	"google.protobuf.Int32Value":  `{"type":"integer"}`,
	"google.protobuf.UInt32Value": `{"type":"integer"}`,
	"google.protobuf.Int64Value":  `{"type":["integer","string"]}`,
	"google.protobuf.UInt64Value": `{"type":["integer","string"]}`,
	"google.protobuf.FloatValue":  `{"type":["number","string"]}`,
	"google.protobuf.DoubleValue": `{"type":["number","string"]}`,
	"google.protobuf.StringValue": `{"type":"string"}`,
	"google.protobuf.BytesValue":  `{"type":"string","contentEncoding":"base64"}`,
}

// Schema is the JSON Schema of a message in its protojson form: 64-bit integers as numbers or strings, bytes as base64, enums by name, oneofs as at most one member, recursion cut after three levels.
func Schema(md protoreflect.MessageDescriptor) json.RawMessage {
	b, _ := json.Marshal(message(md, map[protoreflect.FullName]int{}))
	return b
}

func message(md protoreflect.MessageDescriptor, depth map[protoreflect.FullName]int) any {
	if s, ok := wellKnown[md.FullName()]; ok {
		return json.RawMessage(s)
	}
	if depth[md.FullName()] >= maxRecursion {
		return map[string]any{"type": "object"}
	}
	depth[md.FullName()]++
	defer func() { depth[md.FullName()]-- }()
	props := map[string]any{}
	for i := range md.Fields().Len() {
		f := md.Fields().Get(i)
		props[f.JSONName()] = field(f, depth)
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	var exclusive []any
	for i := range md.Oneofs().Len() {
		if o := md.Oneofs().Get(i); !o.IsSynthetic() {
			exclusive = append(exclusive, atMostOne(o))
		}
	}
	if len(exclusive) > 0 {
		schema["allOf"] = exclusive
	}
	return schema
}

func atMostOne(o protoreflect.OneofDescriptor) any {
	var members, none []any
	for i := range o.Fields().Len() {
		required := map[string]any{"required": []string{o.Fields().Get(i).JSONName()}}
		members = append(members, required)
		none = append(none, required)
	}
	return map[string]any{"oneOf": append(members, map[string]any{"not": map[string]any{"anyOf": none}})}
}

func field(f protoreflect.FieldDescriptor, depth map[protoreflect.FullName]int) any {
	switch {
	case f.IsMap():
		return map[string]any{"type": "object", "additionalProperties": single(f.MapValue(), depth)}
	case f.IsList():
		return map[string]any{"type": "array", "items": single(f, depth)}
	}
	return single(f, depth)
}

func single(f protoreflect.FieldDescriptor, depth map[protoreflect.FullName]int) any {
	switch f.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind, protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer"}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": []string{"integer", "string"}}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": []string{"number", "string"}}
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	case protoreflect.EnumKind:
		values := f.Enum().Values()
		names := make([]string, values.Len())
		for i := range names {
			names[i] = string(values.Get(i).Name())
		}
		return map[string]any{"type": "string", "enum": names}
	}
	return message(f.Message(), depth)
}
