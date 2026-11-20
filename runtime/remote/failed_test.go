package remote

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/port"
)

func TestAClosedConnectionLosesTheAnswerOfACallOnly(t *testing.T) {
	closed := fmt.Errorf("%w: calling: server is closing", mcp.ErrConnectionClosed)
	if err := failed(context.Background(), closed, "tools/call", "f"); !port.IsCode(err, port.CodeOutcomeUnknown) {
		t.Fatalf("tools/call: %v", err)
	}
	if err := failed(context.Background(), closed, "tools/list", ""); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("tools/list: %v", err)
	}
}
