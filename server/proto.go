package server

import (
	"fmt"
	"reflect"
	"strings"
)

// ProtoPackage and ProtoService name the generated gRPC API.
const (
	ProtoPackage = "connectorengine.v1"
	ProtoService = "ConnectorEngine"
)

func protoScalar(k reflect.Kind) (string, bool) {
	switch k {
	case reflect.String:
		return "string", true
	case reflect.Bool:
		return "bool", true
	case reflect.Int, reflect.Int64:
		return "int64", true
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return "int32", true
	case reflect.Uint, reflect.Uint64:
		return "uint64", true
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return "uint32", true
	case reflect.Float32, reflect.Float64:
		return "double", true
	}
	return "", false
}

func protoElem(t reflect.Type) (string, error) {
	t = Elem(t)
	if t == RawType {
		return "google.protobuf.Value", nil
	}
	if IsBytes(t) {
		return "bytes", nil
	}
	if s, ok := protoScalar(t.Kind()); ok {
		return s, nil
	}
	switch {
	case t == TimeType:
		return "google.protobuf.Timestamp", nil
	case t.Kind() == reflect.Struct && t.Name() != "":
		return t.Name(), nil
	case t.Kind() == reflect.Struct && t.NumField() == 0:
		return "google.protobuf.Empty", nil
	}
	return "", fmt.Errorf("%s has no protobuf type", t)
}

func protoField(t reflect.Type) (string, error) {
	t = Elem(t)
	switch {
	case t.Kind() == reflect.Interface:
		return "google.protobuf.Value", nil
	case t == RawType || IsBytes(t):
		return protoElem(t)
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array:
		elem, err := protoElem(t.Elem())
		return "repeated " + elem, err
	case t.Kind() == reflect.Map && t.Key().Kind() == reflect.String:
		if elem, err := protoElem(t.Elem()); err == nil {
			return "map<string, " + elem + ">", nil
		}
		return "google.protobuf.Struct", nil
	}
	return protoElem(t)
}

func protoMessage(t reflect.Type) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "message %s {\n", t.Name())
	for i, f := range Fields(t) {
		typ, err := protoField(f.Type)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", t.Name(), f.Name, err)
		}
		fmt.Fprintf(&b, "  %s %s = %d;\n", typ, f.Name, i+1)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// Proto renders the operation table as the connectorengine.v1.ConnectorEngine protobuf service; field names are the Go JSON names.
func Proto() ([]byte, error) {
	models, err := Models()
	if err != nil {
		return nil, err
	}
	var svc, msgs strings.Builder
	fmt.Fprintf(&svc, "service %s {\n", ProtoService)
	for _, o := range operations {
		in, err := protoElem(reflect.TypeOf(o.In()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.ID, err)
		}
		out, err := protoElem(reflect.TypeOf(o.Out))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.ID, err)
		}
		if o.Live {
			out = "stream " + out
		}
		fmt.Fprintf(&svc, "  rpc %s(%s) returns (%s);\n", o.RPC, in, out)
	}
	svc.WriteString("}\n")
	for _, t := range models {
		m, err := protoMessage(t)
		if err != nil {
			return nil, err
		}
		msgs.WriteString("\n" + m)
	}
	body := svc.String() + msgs.String()
	var b strings.Builder
	fmt.Fprintf(&b, "syntax = \"proto3\";\n\npackage %s;\n\noption go_package = \"github.com/agents-control/connector-engine/api/connectorengine/v1;connectorenginev1\";\n\n", ProtoPackage)
	for _, w := range []string{"empty", "struct", "timestamp"} {
		if strings.Contains(body, "google.protobuf."+strings.ToUpper(w[:1])+w[1:]) || (w == "struct" && strings.Contains(body, "google.protobuf.Value")) {
			fmt.Fprintf(&b, "import \"google/protobuf/%s.proto\";\n", w)
		}
	}
	return []byte(b.String() + "\n" + body), nil
}
