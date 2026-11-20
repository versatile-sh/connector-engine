package grpcdesc_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/versatile-sh/connector-engine/internal/grpcdesc"
	"github.com/versatile-sh/connector-engine/internal/grpctest"
)

func TestMethodsAndHints(t *testing.T) {
	methods := grpcdesc.Methods(grpctest.Files(t), nil)
	got := map[string]grpcdesc.Method{}
	for _, m := range methods {
		got[m.Function.Name] = m
	}
	if len(got) != 4 || got["Greeter_Watch"].Function.Name != "" {
		t.Fatalf("unary methods only: %v", got)
	}
	cases := map[string][2]string{
		"Greeter_SayHello": {"NO_SIDE_EFFECTS", ""},
		"Greeter_Rename":   {"IDEMPOTENT", ""},
		"Greeter_Lookup":   {"", "GET"},
		"Greeter_Purge":    {"", ""},
	}
	for name, want := range cases {
		if m := got[name]; m.Idempotency != want[0] || m.HTTPVerb != want[1] || m.Function.FullMethod != "/demo.v1.Greeter/"+strings.TrimPrefix(name, "Greeter_") {
			t.Fatalf("%s: %+v", name, m)
		}
	}
	if _, err := grpcdesc.Unary(grpctest.Files(t), "/demo.v1.Greeter/Watch"); err == nil || !strings.Contains(err.Error(), "grpc.unaryOnly") {
		t.Fatalf("streaming method: %v", err)
	}
	if names := grpcdesc.ServiceNames(methods); len(names) != 1 || names[0] != "demo.v1.Greeter" {
		t.Fatalf("services %v", names)
	}
}

func TestSchemaFollowsTheProtojsonMapping(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(grpcdesc.Methods(grpctest.Files(t), nil)[3].Function.InputSchema, &s); err != nil {
		t.Fatal(err)
	}
	props := s["properties"].(map[string]any)
	want := map[string]string{
		"name":   `{"type":"string"}`,
		"tags":   `{"items":{"type":"string"},"type":"array"}`,
		"count":  `{"type":["integer","string"]}`,
		"mode":   `{"enum":["MODE_UNSPECIFIED","MODE_LOUD"],"type":"string"}`,
		"scores": `{"additionalProperties":{"type":"integer"},"type":"object"}`,
		"blob":   `{"contentEncoding":"base64","type":"string"}`,
	}
	for k, w := range want {
		b, _ := json.Marshal(props[k])
		if string(b) != w {
			t.Fatalf("%s = %s, want %s", k, b, w)
		}
	}
	if s["additionalProperties"] != false || s["allOf"] == nil {
		t.Fatalf("object shape %v", s)
	}
	tree, _ := json.Marshal(props["tree"])
	if strings.Count(string(tree), `"label"`) != 3 {
		t.Fatalf("recursion is cut after three levels: %s", tree)
	}
}

func TestReflectionV1AndV1Alpha(t *testing.T) {
	for name, mode := range map[string]grpctest.Reflection{"v1": grpctest.ReflectionV1, "v1alpha": grpctest.ReflectionV1Alpha} {
		t.Run(name, func(t *testing.T) {
			srv := grpctest.Start(t, mode)
			conn, err := grpc.NewClient(srv.Target, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			files, err := grpcdesc.FromReflection(context.Background(), conn)
			if err != nil {
				t.Fatal(err)
			}
			methods := grpcdesc.Methods(files, nil)
			if len(methods) != 4 {
				t.Fatalf("%d methods", len(methods))
			}
			if _, err := files.FindFileByPath("google/protobuf/timestamp.proto"); err != nil {
				t.Fatalf("a transitive dependency is missing: %v", err)
			}
		})
	}
}

func TestReflectionAbsent(t *testing.T) {
	srv := grpctest.Start(t, grpctest.ReflectionNone)
	conn, _ := grpc.NewClient(srv.Target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	if _, err := grpcdesc.FromReflection(context.Background(), conn); err == nil {
		t.Fatal("a server without reflection must fail")
	}
}

func TestMarshalIsDeterministic(t *testing.T) {
	a, err := grpcdesc.Marshal(grpctest.Files(t))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := grpcdesc.Marshal(grpctest.Files(t))
	if string(a) != string(b) || len(a) == 0 {
		t.Fatal("equal sets must have equal bytes")
	}
}
