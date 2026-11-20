package grpcapi

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stream struct{ grpc.ServerStream }

func (stream) Context() context.Context { return context.Background() }

func TestAPanickingHandlerAnswersInternalAndIsLogged(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	_, err := recoverUnary(log)(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x/Unary"}, func(context.Context, any) (any, error) { panic("handler bug") })
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "handler bug") {
		t.Fatalf("unary: %v", err)
	}
	err = recoverStream(log)(nil, stream{}, &grpc.StreamServerInfo{FullMethod: "/x/Stream"}, func(any, grpc.ServerStream) error { panic("stream bug") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("stream: %v", err)
	}
	if !strings.Contains(logs.String(), "handler bug") || !strings.Contains(logs.String(), "/x/Stream") {
		t.Fatalf("logs: %q", logs.String())
	}
}
