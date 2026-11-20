package server_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	connectorenginev1 "github.com/versatile-sh/connector-engine/api/connectorengine/v1"
	"github.com/versatile-sh/connector-engine/server"
	"github.com/versatile-sh/connector-engine/server/grpcapi"
	"github.com/versatile-sh/connector-engine/server/httpapi"
	"github.com/versatile-sh/connector-engine/server/internal/harness"
)

func service() protoreflect.ServiceDescriptor {
	return connectorenginev1.File_connectorengine_v1_connectorengine_proto.Services().ByName(server.ProtoService)
}

func TestEveryOperationIsOnHTTPAndOnGRPCWithTheSameFields(t *testing.T) {
	rpcs := service().Methods()
	if rpcs.Len() != len(server.Operations()) {
		t.Fatalf("%d operations, %d rpcs", len(server.Operations()), rpcs.Len())
	}
	spec, err := server.OpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range server.Operations() {
		md := rpcs.ByName(protoreflect.Name(o.RPC))
		if md == nil {
			t.Errorf("%s has no rpc %s", o.ID, o.RPC)
			continue
		}
		fields := md.Input().Fields()
		if o.In != nil {
			for _, f := range server.Fields(typeOf(o.In())) {
				if fields.ByName(protoreflect.Name(f.Name)) == nil {
					t.Errorf("%s: field %s is on HTTP only", o.ID, f.Name)
				}
			}
			if len(server.Fields(typeOf(o.In()))) != fields.Len() {
				t.Errorf("%s: %d HTTP fields, %d gRPC fields", o.ID, len(server.Fields(typeOf(o.In()))), fields.Len())
			}
		}
		if !contains(spec, "operationId: "+o.ID+"\n") {
			t.Errorf("%s is not in the OpenAPI document", o.ID)
		}
	}
}

func TestAnAgentTokenIsRefusedOnEveryOperationOfHTTPAndGRPC(t *testing.T) {
	h := harness.New(t)
	handler, err := httpapi.New(httpapi.Options{Env: h.Server, Authn: h.Authn})
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(handler)
	t.Cleanup(web.Close)
	srv, err := grpcapi.New(h.Server, h.Authn, nil)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	agent := h.AgentToken(t)
	for _, o := range server.Operations() {
		if o.Public {
			continue
		}
		path := o.Path
		for _, name := range o.PathParams() {
			path = replace(path, "{"+name+"}", "x")
		}
		req, _ := http.NewRequest(o.Method, web.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+agent)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || resp.Header.Get("Content-Type") != server.ProblemMediaType {
			t.Errorf("http %s: %d", o.ID, resp.StatusCode)
		}
		if code := invoke(t, conn, o, agent); code != "agent_principal_refused" {
			t.Errorf("grpc %s: %q", o.ID, code)
		}
	}
}

func invoke(t *testing.T, conn *grpc.ClientConn, o server.Operation, token string) string {
	t.Helper()
	md := service().Methods().ByName(protoreflect.Name(o.RPC))
	req, reply := dynamicpb.NewMessage(md.Input()), dynamicpb.NewMessage(md.Output())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	var trailer metadata.MD
	method := "/" + string(service().FullName()) + "/" + o.RPC
	var err error
	if o.Live {
		var stream grpc.ClientStream
		if stream, err = conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, method); err == nil {
			if err = stream.SendMsg(req); err == nil {
				_ = stream.CloseSend()
				err = stream.RecvMsg(reply)
			}
			trailer = stream.Trailer()
		}
	} else {
		err = conn.Invoke(ctx, method, req, reply, grpc.Trailer(&trailer))
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("grpc %s: %v", o.ID, err)
	}
	return first(trailer.Get(grpcapi.TrailerErrorCode))
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}
