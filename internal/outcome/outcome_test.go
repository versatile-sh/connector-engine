package outcome_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/agents-control/connector-engine/internal/outcome"
	"github.com/agents-control/connector-engine/port"
)

func TestOf(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	_, dial := net.Dial("tcp", addr)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		ctx  context.Context
		err  error
		want port.Code
	}{
		"never left":     {context.Background(), dial, port.CodeUnavailable},
		"after the send": {context.Background(), errors.New("connection reset by peer"), port.CodeOutcomeUnknown},
	} {
		if got := port.CodeOf(outcome.Of(tc.ctx, tc.err, "call: %v")); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
	if err := outcome.Of(cancelled, dial, "call: %v"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a done context answers its own error: %v", err)
	}
}
