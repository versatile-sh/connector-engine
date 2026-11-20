// Package harness wires the service over the memory store and a local token issuer, for the tests of every surface.
package harness

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agents-control/plugkit/discovery"

	"github.com/versatile-sh/connector-engine/auth/jwks"
	"github.com/versatile-sh/connector-engine/auth/local"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/server"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

// Audience is the audience of the management surface in tests.
const Audience = "https://manage.test"

// Harness is an Env behind a verifier of tokens its own issuer signs.
type Harness struct {
	*st.Env
	Server *server.Env
	Authn  server.Authenticator
	Issuer *local.Issuer
}

// New builds a Harness whose verifier trusts one local issuer.
func New(t *testing.T) *Harness {
	t.Helper()
	e := st.New(t)
	issuer, err := local.New("test-issuer", e.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	keys := httptest.NewServer(issuer.Handler())
	t.Cleanup(keys.Close)
	verifier, err := jwks.New(jwks.Config{Issuers: []jwks.Issuer{{Name: issuer.Name(), URL: keys.URL}}, Now: e.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	authn := func(ctx context.Context, token string) (port.Principal, error) {
		return verifier.Verify(ctx, token, Audience)
	}
	return &Harness{Env: e, Server: &server.Env{Svc: e.Svc, Adapters: func() discovery.Document { return discovery.Document{Engine: "connector-engine"} }}, Authn: authn, Issuer: issuer}
}

// Token signs a token for the claims, with the management audience unless the claims name another.
func (h *Harness) Token(t *testing.T, c port.Claims) string {
	t.Helper()
	if len(c.Audience) == 0 {
		c.Audience = []string{Audience}
	}
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = h.Clock.Now().Add(time.Hour)
	}
	tok, err := h.Issuer.Issue(c)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// AdminToken is a token of a person who is admin of tenant t1.
func (h *Harness) AdminToken(t *testing.T) string {
	t.Helper()
	h.TenantAdmin("t1")
	return h.Token(t, port.Claims{Subject: "admin", Tenant: "t1"})
}

// AgentToken is a per-run token of an agent, for the audience given or the management one.
func (h *Harness) AgentToken(t *testing.T, audience ...string) string {
	t.Helper()
	return h.Token(t, port.Claims{Subject: "x", Tenant: "t1", Workspace: "w1", Agent: "x", Session: "s1", Audience: audience})
}
