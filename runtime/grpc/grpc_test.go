package grpc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/credential"
	"github.com/agents-control/connector-engine/internal/digest"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/internal/grpcdesc"
	"github.com/agents-control/connector-engine/internal/grpctest"
	"github.com/agents-control/connector-engine/internal/hints"
	"github.com/agents-control/connector-engine/port"
	rt "github.com/agents-control/connector-engine/runtime/grpc"
	st "github.com/agents-control/connector-engine/service/servicetest"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func runtime() *rt.Runtime {
	return rt.New(rt.Options{
		Insecure: true,
		Guard:    egress.Guard{AllowPrivate: true},
		Applier: credential.Applier{
			Resolve: func(context.Context, port.SecretsRef) (port.CredentialKind, []byte, error) {
				return port.CredentialAPIKey, []byte("upstream"), nil
			},
			Injector: credential.Injector{},
		},
	})
}

func manifest(t *testing.T, target string, d port.Descriptor) port.Manifest {
	t.Helper()
	g := &port.GRPC{Target: target, Descriptor: d}
	for _, m := range grpcdesc.Methods(grpctest.Files(t), nil) {
		g.Methods = append(g.Methods, m.Function)
	}
	return port.Manifest{Kind: port.KindGRPCDeclarative, GRPC: g}
}

func TestDescriptorSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.binpb")
	if err := os.WriteFile(path, grpctest.DescriptorSet, 0o600); err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(grpctest.DescriptorSet) }))
	defer web.Close()
	sum := digest.Sum(grpctest.DescriptorSet)

	for name, tc := range map[string]struct {
		mode grpctest.Reflection
		desc port.Descriptor
	}{
		"reflection v1":      {grpctest.ReflectionV1, port.Descriptor{From: port.DescriptorReflection}},
		"reflection v1alpha": {grpctest.ReflectionV1Alpha, port.Descriptor{From: port.DescriptorReflection}},
		"descriptor file":    {grpctest.ReflectionNone, port.Descriptor{From: port.DescriptorFileSet, Ref: path, Digest: sum}},
		"descriptor url":     {grpctest.ReflectionNone, port.Descriptor{From: port.DescriptorFileSet, Ref: web.URL, Digest: "sha256:" + sum}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := grpctest.Start(t, tc.mode)
			r := runtime()
			ctx := context.Background()
			h, err := r.Start(ctx, port.Connector{}, manifest(t, srv.Target, tc.desc), port.SecretsRef{Credential: "c1"})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Stop(ctx, h)

			defs, _, err := r.ListFunctions(ctx, h)
			if err != nil || len(defs) != 4 {
				t.Fatalf("definitions: %v %d", err, len(defs))
			}
			byName := map[string]port.RawDefinition{}
			for _, d := range defs {
				byName[d.Name] = d
			}
			var lookup struct {
				Meta map[string]map[string]string `json:"_meta"`
			}
			json.Unmarshal(byName["Greeter_Lookup"].Raw, &lookup)
			if lookup.Meta[hints.Key]["http_method"] != "GET" {
				t.Fatalf("hints %s", byName["Greeter_Lookup"].Raw)
			}
			if h1, err := pinning.DefinitionHash(byName["Greeter_SayHello"].Raw); err != nil || h1 == "" {
				t.Fatalf("a definition must hash: %v", err)
			}

			res, err := r.Call(ctx, h, "Greeter_SayHello", json.RawMessage(`{"name":"ana","count":"41"}`), time.Now().Add(5*time.Second))
			if err != nil || res.IsError {
				t.Fatalf("call: %v %+v", err, res)
			}
			var body map[string]any
			json.Unmarshal(res.Structured, &body)
			if body["message"] != "hello ana" || body["total"] != "41" {
				t.Fatalf("structured %s", res.Structured)
			}
			if !strings.Contains(string(res.Content), "hello ana") {
				t.Fatalf("content %s", res.Content)
			}
			if auth := srv.Authorizations(); len(auth) == 0 || auth[0] != "Bearer upstream" {
				t.Fatalf("authorization metadata %v", auth)
			}
		})
	}
}

func TestCallOutcomes(t *testing.T) {
	srv := grpctest.Start(t, grpctest.ReflectionV1)
	r := runtime()
	ctx := context.Background()
	h, err := r.Start(ctx, port.Connector{}, manifest(t, srv.Target, port.Descriptor{From: port.DescriptorReflection}), port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := r.Call(ctx, h, "Greeter_SayHello", json.RawMessage(`{"name":"fail"}`), time.Time{}); err != nil || !res.IsError || !strings.Contains(string(res.Content), "NotFound") {
		t.Fatalf("a status is an error result: %v %+v", err, res)
	}
	if _, err := r.Call(ctx, h, "Greeter_SayHello", json.RawMessage(`{"name":"down"}`), time.Time{}); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("unavailable: %v", err)
	}
	if _, err := r.Call(ctx, h, "Greeter_SayHello", json.RawMessage(`{"nope":1}`), time.Time{}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("an unknown field: %v", err)
	}
	if res, err := r.Call(ctx, h, "Greeter_Purge", nil, time.Time{}); err != nil || string(res.Structured) != `{}` {
		t.Fatalf("empty message: %v %+v", err, res)
	}
	if _, err := r.Call(ctx, h, "Greeter_Watch", nil, time.Time{}); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("a method outside the manifest: %v", err)
	}
}

func TestStartRefusals(t *testing.T) {
	srv := grpctest.Start(t, grpctest.ReflectionV1)
	r := runtime()
	ctx := context.Background()

	streaming := manifest(t, srv.Target, port.Descriptor{From: port.DescriptorReflection})
	streaming.GRPC.Methods = append(streaming.GRPC.Methods, port.GRPCFunction{Name: "Greeter_Watch", FullMethod: "/demo.v1.Greeter/Watch"})
	if _, err := r.Start(ctx, port.Connector{}, streaming, port.SecretsRef{}); err == nil || !strings.Contains(err.Error(), "grpc.unaryOnly") {
		t.Fatalf("streaming: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "demo.binpb")
	os.WriteFile(path, grpctest.DescriptorSet, 0o600)
	badDigest := manifest(t, srv.Target, port.Descriptor{From: port.DescriptorFileSet, Ref: path, Digest: "00"})
	if _, err := r.Start(ctx, port.Connector{}, badDigest, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("digest mismatch: %v", err)
	}
	noDigest := manifest(t, srv.Target, port.Descriptor{From: port.DescriptorFileSet, Ref: path})
	if _, err := r.Start(ctx, port.Connector{}, noDigest, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("no digest: %v", err)
	}
	bsr := manifest(t, srv.Target, port.Descriptor{From: port.DescriptorBSR, Ref: "acme/demo", Digest: "00"})
	if _, err := r.Start(ctx, port.Connector{}, bsr, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("bsr without a fetcher: %v", err)
	}
	if caps := r.Describe(); !caps.UnaryOnly {
		t.Fatal("the runtime states unary only")
	}
}

func TestListFunctionsSeesUpstreamDrift(t *testing.T) {
	srv := grpctest.Start(t, grpctest.ReflectionV1)
	r := runtime()
	ctx := context.Background()
	h, err := r.Start(ctx, port.Connector{}, manifest(t, srv.Target, port.Descriptor{From: port.DescriptorReflection}), port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop(ctx, h)
	hashOf := func() port.Hash {
		defs, _, err := r.ListFunctions(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range defs {
			if d.Name == "Greeter_SayHello" {
				return st.Ok(pinning.DefinitionHash(d.Raw))
			}
		}
		t.Fatal("Greeter_SayHello is not listed")
		return ""
	}
	before := hashOf()
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(grpctest.DescriptorSet, &set); err != nil {
		t.Fatal(err)
	}
	for _, f := range set.File {
		for _, m := range f.MessageType {
			if f.GetName() == "demo/v1/demo.proto" && m.GetName() == "HelloRequest" {
				m.Field = append(m.Field, &descriptorpb.FieldDescriptorProto{Name: proto.String("extra"), JsonName: proto.String("extra"), Number: proto.Int32(10),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()})
			}
		}
	}
	srv.Describe(st.Ok(grpcdesc.FromSet(st.Ok(proto.Marshal(&set)))))
	if hashOf() == before {
		t.Fatal("a re-list must see the changed upstream descriptors")
	}
}
