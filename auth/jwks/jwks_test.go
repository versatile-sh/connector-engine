package jwks_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/auth/internal/wire"
	"github.com/agents-control/connector-engine/auth/jwks"
	"github.com/agents-control/connector-engine/auth/local"
	"github.com/agents-control/connector-engine/port"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	aud = "https://engine.example/mcp"
)

func agent() port.Claims {
	return port.Claims{Audience: []string{aud}, IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute), Tenant: "ta", Workspace: "w", Agent: "x", Session: "s1", AgentOwner: "u1"}
}

type fixture struct {
	issuer *local.Issuer
	srv    *httptest.Server
	hits   *atomic.Int32
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	iss, err := local.New("https://issuer.example", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		iss.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return fixture{issuer: iss, srv: srv, hits: &hits}
}

func (f fixture) verifier(t *testing.T, edit func(*jwks.Config)) *jwks.Verifier {
	t.Helper()
	cfg := jwks.Config{Issuers: []jwks.Issuer{{Name: f.issuer.Name(), URL: f.srv.URL}}, Now: func() time.Time { return now }}
	if edit != nil {
		edit(&cfg)
	}
	v, err := jwks.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) token(t *testing.T, c port.Claims) string {
	t.Helper()
	tok, err := f.issuer.Issue(c)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func refused(t *testing.T, v *jwks.Verifier, token, audience string, want port.Code) {
	t.Helper()
	if _, err := v.Verify(ctx, token, audience); !port.IsCode(err, want) {
		t.Fatalf("got %v, want %s", err, want)
	}
}

func TestVerifyReturnsTheAgentPrincipal(t *testing.T) {
	f := newFixture(t)
	p, err := f.verifier(t, nil).Verify(ctx, f.token(t, agent()), aud)
	if err != nil {
		t.Fatal(err)
	}
	c := p.Claims
	if p.Kind != port.PrincipalAgent || c.Tenant != "ta" || c.Workspace != "w" || c.Agent != "x" || c.AgentOwner != "u1" || c.Session != "s1" || c.Issuer != f.issuer.Name() || p.Person != "" {
		t.Fatalf("principal: %+v", p)
	}
}

func TestVerifyClassifiesViewAndPerson(t *testing.T) {
	f := newFixture(t)
	v := f.verifier(t, nil)
	view := agent()
	view.Caller = port.CallerView
	if p, err := v.Verify(ctx, f.token(t, view), aud); err != nil || p.Kind != port.PrincipalView {
		t.Fatalf("view: %+v %v", p, err)
	}
	person := port.Claims{Audience: []string{aud}, ExpiresAt: now.Add(8 * time.Hour), Tenant: "ta", Subject: "u7"}
	p, err := v.Verify(ctx, f.token(t, person), aud)
	if err != nil || p.Kind != port.PrincipalPerson || p.Person != "u7" {
		t.Fatalf("person with a long session: %+v %v", p, err)
	}
}

func TestVerifyRefusesBadTokens(t *testing.T) {
	f := newFixture(t)
	v := f.verifier(t, nil)
	expired := agent()
	expired.IssuedAt, expired.ExpiresAt = now.Add(-2*time.Hour), now.Add(-time.Hour)
	long := agent()
	long.ExpiresAt = now.Add(48 * time.Hour)
	noTenant := agent()
	noTenant.Tenant = ""
	noSession := agent()
	noSession.Session = ""
	future := agent()
	future.IssuedAt, future.ExpiresAt = now.Add(time.Hour), now.Add(2*time.Hour)
	nested := agent()
	nested.Workspace = "w1/x"
	reserved := agent()
	reserved.Tenant = "installation"
	slashed := port.Claims{Audience: []string{aud}, ExpiresAt: now.Add(time.Hour), Tenant: "ta", Subject: "u7/p/x"}
	for name, tc := range map[string]struct {
		token, audience string
	}{
		"expired":        {f.token(t, expired), aud},
		"wrong audience": {f.token(t, agent()), "https://other.example/mcp"},
		"too long":       {f.token(t, long), aud},
		"no tenant":      {f.token(t, noTenant), aud},
		"no session":     {f.token(t, noSession), aud},
		"issued ahead":   {f.token(t, future), aud},
		"nested id":      {f.token(t, nested), aud},
		"reserved id":    {f.token(t, reserved), aud},
		"slashed sub":    {f.token(t, slashed), aud},
		"garbage":        {"not.a.token", aud},
		"empty":          {"", aud},
	} {
		t.Run(name, func(t *testing.T) { refused(t, v, tc.token, tc.audience, port.CodeUnauthenticated) })
	}
}

func TestAgentTokenLifetimeIsAlwaysCapped(t *testing.T) {
	f := newFixture(t)
	long := agent()
	long.ExpiresAt = now.Add(48 * time.Hour)
	if _, err := f.verifier(t, func(c *jwks.Config) { c.MaxLifetime = 48 * time.Hour }).Verify(ctx, f.token(t, long), aud); err != nil {
		t.Fatal(err)
	}
	long.ExpiresAt = now.Add(49 * time.Hour)
	refused(t, f.verifier(t, func(c *jwks.Config) { c.MaxLifetime = 48 * time.Hour }), f.token(t, long), aud, port.CodeUnauthenticated)
	refused(t, f.verifier(t, func(c *jwks.Config) { c.MaxLifetime = time.Minute }), f.token(t, agent()), aud, port.CodeUnauthenticated)
}

func TestVerifyRefusesAnUnknownKeyAndAnUnknownIssuer(t *testing.T) {
	f := newFixture(t)
	v := f.verifier(t, nil)
	impostor, _ := local.New(f.issuer.Name(), func() time.Time { return now })
	tok, _ := impostor.Issue(agent())
	refused(t, v, tok, aud, port.CodeUnauthenticated)
	stranger, _ := local.New("https://stranger.example", func() time.Time { return now })
	tok, _ = stranger.Issue(agent())
	refused(t, v, tok, aud, port.CodeUnauthenticated)
}

func TestUnknownKeyIdsRefetchNoMoreThanOncePerInterval(t *testing.T) {
	f := newFixture(t)
	v := f.verifier(t, nil)
	impostor, _ := local.New(f.issuer.Name(), func() time.Time { return now })
	tok, _ := impostor.Issue(agent())
	for range 5 {
		refused(t, v, tok, aud, port.CodeUnauthenticated)
	}
	if n := f.hits.Load(); n != 1 {
		t.Fatalf("key set fetched %d times", n)
	}
	if _, err := v.Verify(ctx, f.token(t, agent()), aud); err != nil || f.hits.Load() != 1 {
		t.Fatalf("a cached key verifies without a fetch: %v hits=%d", err, f.hits.Load())
	}
}

func TestUnreachableIssuerIsUnavailable(t *testing.T) {
	f := newFixture(t)
	tok := f.token(t, agent())
	f.srv.Close()
	refused(t, f.verifier(t, nil), tok, aud, port.CodeUnavailable)
}

func sign(t *testing.T, alg jose.SignatureAlgorithm, key any, public any, kid string) (string, jose.JSONWebKeySet) {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	c := agent()
	c.Issuer = "https://issuer.example"
	tok, err := jwt.Signed(signer).Claims(wire.From(c)).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: kid, Algorithm: string(alg), Use: "sig"}}}
}

func TestVerifyAcceptsRS256AndES256AndRefusesHMAC(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for name, tc := range map[string]struct {
		alg     jose.SignatureAlgorithm
		private any
		public  any
	}{"RS256": {jose.RS256, rsaKey, &rsaKey.PublicKey}, "ES256": {jose.ES256, ecKey, &ecKey.PublicKey}} {
		t.Run(name, func(t *testing.T) {
			tok, set := sign(t, tc.alg, tc.private, tc.public, "k1")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(set) }))
			defer srv.Close()
			v, err := jwks.New(jwks.Config{Issuers: []jwks.Issuer{{Name: "https://issuer.example", URL: srv.URL}}, Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.Verify(ctx, tok, aud); err != nil {
				t.Fatal(err)
			}
			hmacTok, _ := sign(t, jose.HS256, []byte("0123456789abcdef0123456789abcdef"), nil, "k1")
			refused(t, v, hmacTok, aud, port.CodeUnauthenticated)
		})
	}
}

func TestNewFailsFast(t *testing.T) {
	for _, cfg := range []jwks.Config{{}, {Issuers: []jwks.Issuer{{Name: "x", URL: "ftp://h"}}}, {Issuers: []jwks.Issuer{{URL: "http://h"}}}} {
		if _, err := jwks.New(cfg); !port.IsCode(err, port.CodeInvalidArgument) {
			t.Errorf("accepted %+v", cfg)
		}
	}
}
