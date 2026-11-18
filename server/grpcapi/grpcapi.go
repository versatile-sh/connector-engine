// Package grpcapi serves the operation table as the connectorengine.v1.ConnectorEngine gRPC service; every RPC runs the same call as its HTTP route.
package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	connectorenginev1 "github.com/agents-control/connector-engine/api/connectorengine/v1"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/server"
)

// TrailerErrorCode is the trailer that carries the error code string of a failed call.
const TrailerErrorCode = "connector-error-code"

const errorDomain = "connector-engine"

// Codes maps every error code to its gRPC status code; a code outside the map is internal.
var Codes = map[port.Code]codes.Code{
	port.CodeInvalidArgument:       codes.InvalidArgument,
	port.CodeInvalidManifest:       codes.InvalidArgument,
	port.CodeInvalidPath:           codes.InvalidArgument,
	port.CodeNameTooLong:           codes.InvalidArgument,
	port.CodeInvalidAlias:          codes.InvalidArgument,
	port.CodeRowRefused:            codes.InvalidArgument,
	port.CodePolicyRefused:         codes.InvalidArgument,
	port.CodeTokenInArguments:      codes.InvalidArgument,
	port.CodeUnauthenticated:       codes.Unauthenticated,
	port.CodePermissionDenied:      codes.PermissionDenied,
	port.CodeAgentPrincipalRefused: codes.PermissionDenied,
	port.CodeNotAnApprover:         codes.PermissionDenied,
	port.CodeNotTheBoundPerson:     codes.PermissionDenied,
	port.CodeNotFound:              codes.NotFound,
	port.CodeTenantRemoved:         codes.NotFound,
	port.CodeConflict:              codes.Aborted,
	port.CodeRevisionMismatch:      codes.Aborted,
	port.CodeInvalidTransition:     codes.FailedPrecondition,
	port.CodeStaleApproval:         codes.FailedPrecondition,
	port.CodeApprovalExpired:       codes.FailedPrecondition,
	port.CodeDefinitionChanged:     codes.FailedPrecondition,
	port.CodeClassChanged:          codes.FailedPrecondition,
	port.CodeGrantRevoked:          codes.FailedPrecondition,
	port.CodeGrantExhausted:        codes.FailedPrecondition,
	port.CodeListingDeprecated:     codes.FailedPrecondition,
	port.CodeCredentialRequired:    codes.FailedPrecondition,
	port.CodeIsolationUnmet:        codes.FailedPrecondition,
	port.CodeNoApprover:            codes.FailedPrecondition,
	port.CodeTooManyPending:        codes.ResourceExhausted,
	port.CodeUnavailable:           codes.Unavailable,
	port.CodeInternal:              codes.Internal,
}

type handler struct {
	env    *server.Env
	authn  server.Authenticator
	log    *slog.Logger
	op     server.Operation
	method string
	in     protoreflect.MessageType
	out    protoreflect.MessageType
}

// New builds the gRPC server over env; it fails when the operation table and the connectorengine.v1 descriptors disagree.
func New(env *server.Env, authn server.Authenticator, log *slog.Logger, opts ...grpc.ServerOption) (*grpc.Server, error) {
	if env == nil || env.Svc == nil || authn == nil {
		return nil, errors.New("grpcapi: an environment and an authenticator are required")
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	file := connectorenginev1.File_connectorengine_v1_connectorengine_proto
	sd := file.Services().ByName(server.ProtoService)
	desc := &grpc.ServiceDesc{ServiceName: string(sd.FullName()), HandlerType: (*any)(nil), Metadata: file.Path()}
	ops := server.Operations()
	if sd.Methods().Len() != len(ops) {
		return nil, fmt.Errorf("grpcapi: %d operations but %d rpcs", len(ops), sd.Methods().Len())
	}
	for _, op := range ops {
		md := sd.Methods().ByName(protoreflect.Name(op.RPC))
		if md == nil || md.IsStreamingServer() != op.Live || md.IsStreamingClient() {
			return nil, fmt.Errorf("grpcapi: operation %s does not match rpc %s", op.ID, op.RPC)
		}
		h := handler{env: env, authn: authn, log: log, op: op, method: "/" + desc.ServiceName + "/" + op.RPC}
		var err error
		if h.in, err = protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName()); err != nil {
			return nil, err
		}
		if h.out, err = protoregistry.GlobalTypes.FindMessageByName(md.Output().FullName()); err != nil {
			return nil, err
		}
		if op.Live {
			desc.Streams = append(desc.Streams, grpc.StreamDesc{StreamName: op.RPC, Handler: h.watch, ServerStreams: true})
		} else {
			desc.Methods = append(desc.Methods, grpc.MethodDesc{MethodName: op.RPC, Handler: h.unary})
		}
	}
	s := grpc.NewServer(opts...)
	s.RegisterService(desc, struct{}{})
	return s, nil
}

func (h handler) unary(_ any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
	req := h.in.New().Interface()
	if err := dec(req); err != nil {
		return nil, err
	}
	if ic == nil {
		return h.call(ctx, req)
	}
	return ic(ctx, req, &grpc.UnaryServerInfo{FullMethod: h.method}, func(ctx context.Context, req any) (any, error) { return h.call(ctx, req.(proto.Message)) })
}

func (h handler) call(ctx context.Context, req proto.Message) (any, error) {
	in, p, key, err := h.input(ctx, req)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	res, err := h.op.Invoke(ctx, h.env, p, key, in)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	reply := h.out.New().Interface()
	if err := encode(res, reply); err != nil {
		return nil, h.fail(ctx, err)
	}
	return reply, nil
}

func (h handler) watch(_ any, stream grpc.ServerStream) error {
	req, ctx := h.in.New().Interface(), stream.Context()
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	in, p, _, err := h.input(ctx, req)
	if err != nil {
		return h.fail(ctx, err)
	}
	send := func(e port.Event) error {
		msg := h.out.New().Interface()
		if err := encode(e, msg); err != nil {
			return err
		}
		return stream.SendMsg(msg)
	}
	if err := h.op.Watch(ctx, h.env, p, in, func() { _ = stream.SendHeader(nil) }, send); err != nil {
		return h.fail(ctx, err)
	}
	return nil
}

func (h handler) input(ctx context.Context, req proto.Message) (any, port.Principal, string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	first := func(name string) string {
		if v := md.Get(strings.ToLower(name)); len(v) > 0 {
			return v[0]
		}
		return ""
	}
	p, err := server.Authenticate(ctx, h.op, h.authn, server.Bearer(first(server.HeaderAuthorization)))
	if err != nil {
		return nil, p, "", err
	}
	in := h.op.In()
	if err := decode(req.ProtoReflect(), in); err != nil {
		return nil, p, "", port.Errorf(port.CodeInvalidArgument, "request: %v", err)
	}
	return in, p, first(server.HeaderIdempotencyKey), nil
}

func (h handler) fail(ctx context.Context, err error) error {
	p := server.ProblemOf(err)
	if p.Code == port.CodeInternal {
		h.log.ErrorContext(ctx, "rpc failed", "method", h.method, "error", err)
	}
	_ = grpc.SetTrailer(ctx, metadata.Pairs(TrailerErrorCode, string(p.Code)))
	st := status.New(Codes[p.Code], p.Detail)
	if with, err := st.WithDetails(&errdetails.ErrorInfo{Reason: string(p.Code), Domain: errorDomain}); err == nil {
		st = with
	}
	return st.Err()
}
