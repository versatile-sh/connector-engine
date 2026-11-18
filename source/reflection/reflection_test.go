package reflection_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/internal/grpctest"
	"github.com/agents-control/connector-engine/internal/sourcetest"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/reflection"
)

func TestImportsAServerThroughReflection(t *testing.T) {
	v1 := grpctest.Start(t, grpctest.ReflectionV1)
	alpha := grpctest.Start(t, grpctest.ReflectionV1Alpha)
	got := sourcetest.Import(t, reflection.New(reflection.Config{Targets: []string{v1.Target, alpha.Target}, Insecure: true}))
	if len(got) != 2 {
		t.Fatalf("imported %d", len(got))
	}
	var versions []string
	for name, m := range got {
		if !strings.HasPrefix(name, "grpc.reflection/") || m.Kind != port.KindGRPCDeclarative || m.GRPC.Descriptor.From != port.DescriptorReflection ||
			m.GRPC.Services[0] != "demo.v1.Greeter" || len(m.GRPC.Methods) != 4 {
			t.Fatalf("manifest %s: %+v", name, m)
		}
		for _, f := range m.GRPC.Methods {
			if f.Name == "Greeter_Watch" || len(f.InputSchema) == 0 || len(f.OutputSchema) == 0 {
				t.Fatalf("function %+v", f)
			}
		}
		versions = append(versions, m.Version)
	}
	if versions[0] != versions[1] {
		t.Fatalf("the same descriptors must give the same version: %v", versions)
	}
}

func TestFailsWhenReflectionIsOff(t *testing.T) {
	off := grpctest.Start(t, grpctest.ReflectionNone)
	if _, err := reflection.New(reflection.Config{Targets: []string{off.Target}, Insecure: true}).List(context.Background(), ""); err == nil {
		t.Fatal("a server without reflection cannot be imported")
	}
}
