package grpcdesc

import (
	"context"
	"maps"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/versatile-sh/connector-engine/port"
)

var reflectionMethods = []string{
	"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
	"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
}

// FromReflection reads every service and its transitive dependencies from a server, trying reflection v1 and then v1alpha.
func FromReflection(ctx context.Context, conn grpc.ClientConnInterface) (*protoregistry.Files, error) {
	var lastErr error
	for _, method := range reflectionMethods {
		set, err := collect(ctx, conn, method)
		if err == nil {
			return Resolve(set)
		}
		if status.Code(err) != codes.Unimplemented {
			return nil, port.Errorf(port.CodeUnavailable, "reflection: %v", err)
		}
		lastErr = err
	}
	return nil, port.Errorf(port.CodeUnavailable, "the server offers neither reflection v1 nor v1alpha: %v", lastErr)
}

func collect(ctx context.Context, conn grpc.ClientConnInterface, method string) (*descriptorpb.FileDescriptorSet, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{StreamName: "ServerReflectionInfo", ClientStreams: true, ServerStreams: true}, method)
	if err != nil {
		return nil, err
	}
	ask := func(req *rpb.ServerReflectionRequest) (*rpb.ServerReflectionResponse, error) {
		if err := stream.SendMsg(req); err != nil {
			return nil, err
		}
		resp := &rpb.ServerReflectionResponse{}
		if err := stream.RecvMsg(resp); err != nil {
			return nil, err
		}
		if e := resp.GetErrorResponse(); e != nil {
			return nil, status.Error(codes.Code(e.ErrorCode), e.ErrorMessage)
		}
		return resp, nil
	}
	listing, err := ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_ListServices{ListServices: "*"}})
	if err != nil {
		return nil, err
	}
	byName := map[string]*descriptorpb.FileDescriptorProto{}
	add := func(resp *rpb.ServerReflectionResponse) error {
		for _, raw := range resp.GetFileDescriptorResponse().GetFileDescriptorProto() {
			fd := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(raw, fd); err != nil {
				return err
			}
			byName[fd.GetName()] = fd
		}
		return nil
	}
	for _, s := range listing.GetListServicesResponse().GetService() {
		if strings.HasPrefix(s.GetName(), "grpc.reflection.") {
			continue
		}
		resp, err := ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: s.GetName()}})
		if err != nil {
			return nil, err
		}
		if err := add(resp); err != nil {
			return nil, err
		}
	}
	for {
		missing := ""
		for _, fd := range byName {
			for _, dep := range fd.GetDependency() {
				if _, ok := byName[dep]; !ok {
					missing = dep
				}
			}
		}
		if missing == "" {
			break
		}
		resp, err := ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_FileByFilename{FileByFilename: missing}})
		if err != nil {
			return nil, err
		}
		if err := add(resp); err != nil {
			return nil, err
		}
		if _, ok := byName[missing]; !ok {
			return nil, status.Errorf(codes.NotFound, "the server did not return %s", missing)
		}
	}
	set := &descriptorpb.FileDescriptorSet{}
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		set.File = append(set.File, byName[name])
	}
	return set, stream.CloseSend()
}
