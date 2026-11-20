// Package grpctest serves a small dynamic gRPC service with reflection, for the tests of the adapters that read descriptors.
package grpctest

import (
	"context"
	_ "embed"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	v1grpc "google.golang.org/grpc/reflection/grpc_reflection_v1"
	v1alphagrpc "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/versatile-sh/connector-engine/internal/grpcdesc"
)

// DescriptorSet is demo/v1/demo.proto compiled by protoc with its imports.
//
//go:embed testdata/demo.binpb
var DescriptorSet []byte

// Reflection selects which reflection service a Server registers.
type Reflection int

// Reflection modes.
const (
	ReflectionV1 Reflection = iota
	ReflectionV1Alpha
	ReflectionNone
)

// Server is a running demo.v1.Greeter that records the authorization metadata it received.
type Server struct {
	Target string

	mu    sync.Mutex
	auth  []string
	calls int
	files swap
}

type swap struct {
	atomic.Pointer[protoregistry.Files]
}

func (w *swap) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	return w.Load().FindFileByPath(path)
}

func (w *swap) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	return w.Load().FindDescriptorByName(name)
}

// Describe makes reflection answer from files from now on; the served methods do not change.
func (s *Server) Describe(files *protoregistry.Files) { s.files.Store(files) }

// Calls returns how many unary calls the server received.
func (s *Server) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Authorizations returns the authorization metadata of every unary call received.
func (s *Server) Authorizations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.auth...)
}

// Files resolves DescriptorSet.
func Files(t testing.TB) *protoregistry.Files {
	t.Helper()
	files, err := grpcdesc.FromSet(DescriptorSet)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// Start serves the demo service on a local port until the test ends.
func Start(t testing.TB, mode Reflection) *Server {
	t.Helper()
	files := Files(t)
	srv := &Server{}
	srv.files.Store(files)
	gs := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		srv.mu.Lock()
		srv.calls++
		srv.auth = append(srv.auth, md.Get("authorization")...)
		srv.mu.Unlock()
		return h(ctx, req)
	}))
	d, err := files.FindDescriptorByName("demo.v1.Greeter")
	if err != nil {
		t.Fatal(err)
	}
	gs.RegisterService(serviceDesc(d.(protoreflect.ServiceDescriptor)), struct{}{})
	opts := reflection.ServerOptions{Services: gs, DescriptorResolver: &srv.files}
	switch mode {
	case ReflectionV1:
		v1grpc.RegisterServerReflectionServer(gs, reflection.NewServerV1(opts))
	case ReflectionV1Alpha:
		v1alphagrpc.RegisterServerReflectionServer(gs, reflection.NewServer(opts))
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go gs.Serve(lis)
	t.Cleanup(gs.Stop)
	srv.Target = lis.Addr().String()
	return srv
}

func serviceDesc(sd protoreflect.ServiceDescriptor) *grpc.ServiceDesc {
	out := &grpc.ServiceDesc{ServiceName: string(sd.FullName()), HandlerType: (*any)(nil), Metadata: "demo/v1/demo.proto"}
	for i := range sd.Methods().Len() {
		md := sd.Methods().Get(i)
		if md.IsStreamingServer() {
			out.Streams = append(out.Streams, grpc.StreamDesc{StreamName: string(md.Name()), ServerStreams: true, Handler: watch(md)})
			continue
		}
		out.Methods = append(out.Methods, grpc.MethodDesc{MethodName: string(md.Name()), Handler: unary(md)})
	}
	return out
}

func unary(md protoreflect.MethodDescriptor) func(any, context.Context, func(any) error, grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
		call := func(ctx context.Context, req any) (any, error) { return reply(md, req.(*dynamicpb.Message)) }
		in := dynamicpb.NewMessage(md.Input())
		if err := dec(in); err != nil {
			return nil, err
		}
		if ic == nil {
			return call(ctx, in)
		}
		return ic(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: grpcdesc.FullMethod(md)}, call)
	}
}

func watch(md protoreflect.MethodDescriptor) grpc.StreamHandler {
	return func(_ any, stream grpc.ServerStream) error {
		in := dynamicpb.NewMessage(md.Input())
		if err := stream.RecvMsg(in); err != nil {
			return err
		}
		out, err := reply(md, in)
		if err != nil {
			return err
		}
		return stream.SendMsg(out)
	}
}

func reply(md protoreflect.MethodDescriptor, in *dynamicpb.Message) (*dynamicpb.Message, error) {
	out := dynamicpb.NewMessage(md.Output())
	fields := in.Descriptor().Fields()
	name := fields.ByName("name")
	if name == nil {
		return out, nil
	}
	switch in.Get(name).String() {
	case "fail":
		return nil, status.Error(codes.NotFound, "no such greeting")
	case "down":
		return nil, status.Error(codes.Unavailable, "backend down")
	}
	o := out.Descriptor().Fields()
	out.Set(o.ByName("message"), protoreflect.ValueOfString("hello "+in.Get(name).String()))
	out.Set(o.ByName("total"), protoreflect.ValueOfInt64(in.Get(fields.ByName("count")).Int()))
	return out, nil
}
