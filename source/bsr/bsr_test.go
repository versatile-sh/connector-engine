package bsr_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/agents-control/connector-engine/internal/grpctest"
	"github.com/agents-control/connector-engine/internal/sourcetest"
	"github.com/agents-control/connector-engine/port"
	runtime "github.com/agents-control/connector-engine/runtime/grpc"
	"github.com/agents-control/connector-engine/source/bsr"
)

type fakeBSR struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	auth     string
	refs     []string
}

func newFake(t *testing.T) *fakeBSR {
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(grpctest.DescriptorSet, set); err != nil {
		t.Fatal(err)
	}
	setJSON, err := protojson.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeBSR{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		json.Unmarshal(body, &in)
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type")+" "+r.Header.Get("Connect-Protocol-Version"))
		f.auth = r.Header.Get("Authorization")
		f.mu.Unlock()
		switch r.URL.Path {
		case "/buf.registry.module.v1.ModuleService/ListModules":
			owner := in["ownerRefs"].([]any)[0].(map[string]any)["name"]
			switch {
			case owner == "acme" && in["pageToken"] == "":
				w.Write([]byte(`{"nextPageToken":"p2","modules":[{"name":"payments","state":"MODULE_STATE_ACTIVE","description":"Payments"},{"name":"nottargeted","state":"MODULE_STATE_ACTIVE"}]}`))
			case owner == "acme":
				w.Write([]byte(`{"modules":[{"name":"old","state":"MODULE_STATE_DEPRECATED"}]}`))
			default:
				w.Write([]byte(`{"modules":[{"name":"ledger","state":"MODULE_STATE_ACTIVE"}]}`))
			}
		case "/buf.registry.module.v1.FileDescriptorSetService/GetFileDescriptorSet":
			ref := in["resourceRef"].(map[string]any)["name"].(map[string]any)
			f.mu.Lock()
			if v, ok := ref["ref"].(string); ok {
				f.refs = append(f.refs, v)
			}
			f.mu.Unlock()
			w.Write([]byte(`{"fileDescriptorSet":` + string(setJSON) + `,"commit":{"id":"c0ffee"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func TestImportsModulesWithATarget(t *testing.T) {
	fake := newFake(t)
	srv := grpctest.Start(t, grpctest.ReflectionNone)
	src := bsr.New(bsr.Config{
		BaseURL: fake.URL,
		Token:   "buf-token",
		Owners:  []string{"acme", "globex"},
		Targets: map[string]string{"acme/payments": srv.Target, "acme/old": srv.Target, "globex/ledger": srv.Target},
	})
	got := sourcetest.Import(t, src)
	if len(got) != 2 {
		t.Fatalf("imported %d: a module without a target and a deprecated one are left out", len(got))
	}
	pay := got["build.buf/acme.payments"]
	if pay.Version != "c0ffee" || pay.GRPC.Descriptor.From != port.DescriptorBSR || pay.GRPC.Descriptor.Ref != "acme/payments" || pay.GRPC.Descriptor.Digest == "" ||
		pay.GRPC.Target != srv.Target || len(pay.GRPC.Methods) != 4 || pay.Description != "Payments" || pay.Publisher.Name != "acme" {
		t.Fatalf("manifest %+v", pay)
	}
	if got["build.buf/globex.ledger"].Name == "" {
		t.Fatal("the second owner was not listed")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.auth != "Bearer buf-token" || fake.requests[0] != "POST /buf.registry.module.v1.ModuleService/ListModules application/json 1" {
		t.Fatalf("requests %v auth %q", fake.requests, fake.auth)
	}
}

func TestRuntimeCallsThroughABSRDescriptor(t *testing.T) {
	fake := newFake(t)
	srv := grpctest.Start(t, grpctest.ReflectionNone)
	src := bsr.New(bsr.Config{BaseURL: fake.URL, Owners: []string{"acme"}, Targets: map[string]string{"acme/payments": srv.Target}})
	m, err := src.Fetch(context.Background(), "build.buf/acme.payments", "c0ffee")
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	refs := fake.refs
	fake.mu.Unlock()
	if len(refs) != 1 || refs[0] != "c0ffee" {
		t.Fatalf("a version is requested as a commit ref: %v", refs)
	}

	rt := runtime.New(runtime.Options{Insecure: true, Fetch: src.FetchDescriptor})
	h, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.Call(context.Background(), h, "Greeter_SayHello", json.RawMessage(`{"name":"bsr"}`), time.Now().Add(5*time.Second))
	if err != nil || res.IsError || string(res.Structured) == "" {
		t.Fatalf("call: %v %+v", err, res)
	}

	m.GRPC.Descriptor.Digest = "00"
	if _, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("a changed module must fail the digest check: %v", err)
	}
	if _, err := src.Fetch(context.Background(), "build.buf/acme.unknown", ""); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("a module without a target: %v", err)
	}
}
