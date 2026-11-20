// Package log is an EventSink that logs the identity of each event and never its data.
package log

import (
	"context"
	"log/slog"

	"github.com/versatile-sh/connector-engine/port"
)

// Sink logs each event at info level; it never fails.
type Sink struct{ log *slog.Logger }

var _ port.EventSink = Sink{}

// New returns a sink that writes to log.
func New(log *slog.Logger) Sink { return Sink{log: log} }

// Deliver logs id, type, subject, tenant and sequence of each event.
func (s Sink) Deliver(ctx context.Context, events []port.Event) error {
	for _, e := range events {
		s.log.InfoContext(ctx, "outbox event", "id", e.ID, "type", e.Type, "subject", e.Subject, "tenant", e.Tenant, "stream", e.Stream, "sequence", e.Sequence)
	}
	return nil
}
