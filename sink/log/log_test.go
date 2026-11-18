package log_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/port"
	sinklog "github.com/agents-control/connector-engine/sink/log"
)

func TestSinkLogsIdentityNeverData(t *testing.T) {
	var buf bytes.Buffer
	s := sinklog.New(slog.New(slog.NewTextHandler(&buf, nil)))
	err := s.Deliver(context.Background(), []port.Event{{ID: "e1", Type: "t", Tenant: "ta", Data: map[string]any{"password": "hunter2"}}})
	out := buf.String()
	if err != nil || !strings.Contains(out, "id=e1") || !strings.Contains(out, "type=t") || strings.Contains(out, "hunter2") {
		t.Fatalf("%v %s", err, out)
	}
}
