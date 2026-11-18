package server

import (
	"context"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// Header names, used as HTTP headers and as gRPC metadata keys (lower-cased).
const (
	HeaderAuthorization  = "Authorization"
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderTraceParent    = "traceparent"
)

// Authenticator turns a bearer token into the principal it names, for the audience of one surface.
type Authenticator func(ctx context.Context, token string) (port.Principal, error)

// Bearer returns the token of an Authorization header value, empty when it is not a bearer token.
func Bearer(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// Authenticate resolves the caller of o: no token is unauthenticated except on a public operation, and a token with an agent
// claim is refused before anyone asks the Authorizer (§13.2 P1); a view token is an agent token too.
func Authenticate(ctx context.Context, o Operation, authn Authenticator, token string) (port.Principal, error) {
	if token == "" {
		if o.Public {
			return port.Principal{}, nil
		}
		return port.Principal{}, port.Errorf(port.CodeUnauthenticated, "no bearer token")
	}
	p, err := authn(ctx, token)
	if err != nil {
		return p, err
	}
	if p.Kind == port.PrincipalAgent || p.Kind == port.PrincipalView || p.Claims.Agent != "" {
		return port.Principal{}, port.Errorf(port.CodeAgentPrincipalRefused, "an agent principal is refused on every management surface")
	}
	return p, nil
}
