// Package outcome tells a call that may have reached its upstream from one that never left.
package outcome

import (
	"context"
	"errors"
	"net"

	"github.com/versatile-sh/connector-engine/port"
)

// Of maps the transport error of a call: the context's own error, unavailable when it never left or the engine refused it, otherwise outcome_unknown.
func Of(ctx context.Context, err error, format string, args ...any) error {
	var op *net.OpError
	var dns *net.DNSError
	var refused *port.Error
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case (errors.As(err, &op) && op.Op == "dial") || errors.As(err, &dns) || errors.As(err, &refused):
		return port.Errorf(port.CodeUnavailable, format, append(args, err)...)
	}
	return port.Errorf(port.CodeOutcomeUnknown, format, append(args, err)...)
}
