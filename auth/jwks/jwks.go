// Package jwks is a PrincipalVerifier over the key sets of configured issuers (§4.3).
package jwks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/auth/internal/wire"
	"github.com/agents-control/connector-engine/port"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

var algorithms = []jose.SignatureAlgorithm{jose.EdDSA, jose.RS256, jose.ES256}

// Issuer is a token issuer and where its public keys are published.
type Issuer struct {
	Name string
	URL  string
}

// Config configures a Verifier; zero durations take the defaults noted per field.
type Config struct {
	Issuers []Issuer
	// MaxLifetime bounds exp - iat of every agent token; 3600 s by default.
	MaxLifetime time.Duration
	// Leeway is the clock skew tolerated on exp and nbf; 30 s by default.
	Leeway time.Duration
	// CacheTTL is how long a fetched key set is trusted; 5 min by default.
	CacheTTL time.Duration
	// MinRefetch is the shortest gap between two fetches for an unknown key id; 30 s by default.
	MinRefetch time.Duration
	Client     *http.Client
	Now        func() time.Time
}

// Verifier verifies bearer tokens of the configured issuers.
type Verifier struct {
	cfg     Config
	issuers map[string]*keySet
}

var _ port.PrincipalVerifier = (*Verifier)(nil)

// New validates cfg: at least one issuer, each with a name and an http(s) key set URL.
func New(cfg Config) (*Verifier, error) {
	if len(cfg.Issuers) == 0 {
		return nil, port.Errorf(port.CodeInvalidArgument, "at least one issuer is configured")
	}
	cfg.MaxLifetime = durationOr(cfg.MaxLifetime, time.Hour)
	cfg.Leeway = durationOr(cfg.Leeway, 30*time.Second)
	cfg.CacheTTL = durationOr(cfg.CacheTTL, 5*time.Minute)
	cfg.MinRefetch = durationOr(cfg.MinRefetch, 30*time.Second)
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	v := &Verifier{cfg: cfg, issuers: map[string]*keySet{}}
	for _, i := range cfg.Issuers {
		u, err := url.Parse(i.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || i.Name == "" {
			return nil, port.Errorf(port.CodeInvalidArgument, "issuer %q needs a name and an http(s) key set URL", i.Name)
		}
		v.issuers[i.Name] = &keySet{url: i.URL, cfg: &v.cfg}
	}
	return v, nil
}

func durationOr(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

func unauthenticated(format string, args ...any) error {
	return port.Errorf(port.CodeUnauthenticated, format, args...)
}

// Verify checks the signature, issuer, audience, expiry and, for agent tokens, the lifetime, and returns the principal.
func (v *Verifier) Verify(ctx context.Context, token, audience string) (port.Principal, error) {
	tok, err := jwt.ParseSigned(token, algorithms)
	if err != nil || len(tok.Headers) != 1 {
		return port.Principal{}, unauthenticated("malformed token")
	}
	var claims wire.Claims
	if err := tok.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return port.Principal{}, unauthenticated("malformed token")
	}
	issuer, ok := v.issuers[claims.Issuer]
	if !ok {
		return port.Principal{}, unauthenticated("unknown issuer")
	}
	header := tok.Headers[0]
	key, err := issuer.key(ctx, header.KeyID)
	if err != nil {
		return port.Principal{}, err
	}
	if (key.Algorithm != "" && key.Algorithm != header.Algorithm) || (key.Use != "" && key.Use != "sig") {
		return port.Principal{}, unauthenticated("the key does not sign with %s", header.Algorithm)
	}
	var verified wire.Claims
	if err := tok.Claims(key.Key, &verified); err != nil {
		return port.Principal{}, unauthenticated("bad signature")
	}
	return v.principal(token, audience, verified)
}

func (v *Verifier) principal(token, audience string, w wire.Claims) (port.Principal, error) {
	want := jwt.Expected{Issuer: w.Issuer, AnyAudience: []string{audience}, Time: v.cfg.Now()}
	if err := w.Claims.ValidateWithLeeway(want, v.cfg.Leeway); err != nil {
		return port.Principal{}, unauthenticated("token refused: %v", err)
	}
	c := w.Port()
	switch {
	case c.ExpiresAt.IsZero() || c.IssuedAt.IsZero():
		return port.Principal{}, unauthenticated("a token carries iat and exp")
	case c.Tenant == "":
		return port.Principal{}, unauthenticated("a token carries a tenant")
	}
	kind := port.KindOf(c)
	if kind != port.PrincipalPerson {
		switch {
		case c.Workspace == "" || c.Session == "":
			return port.Principal{}, unauthenticated("an agent token carries workspace and session")
		case c.ExpiresAt.Sub(c.IssuedAt) > v.cfg.MaxLifetime:
			return port.Principal{}, unauthenticated("token lives longer than %s", v.cfg.MaxLifetime)
		}
	}
	p := port.Principal{Kind: kind, Claims: c, Token: token}
	if kind == port.PrincipalPerson {
		p.Person = port.PersonID(c.Subject)
	}
	return p, nil
}

type keySet struct {
	url string
	cfg *Config

	mu        sync.Mutex
	keys      jose.JSONWebKeySet
	fetchedAt time.Time
	triedAt   time.Time
}

func (k *keySet) key(ctx context.Context, kid string) (jose.JSONWebKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.cfg.Now()
	found := k.keys.Key(kid)
	if len(found) > 0 && now.Sub(k.fetchedAt) < k.cfg.CacheTTL {
		return found[0], nil
	}
	if k.triedAt.IsZero() || now.Sub(k.triedAt) >= k.cfg.MinRefetch {
		k.triedAt = now
		if err := k.fetch(ctx); err != nil && len(found) == 0 {
			return jose.JSONWebKey{}, port.Errorf(port.CodeUnavailable, "fetch key set: %v", err)
		} else if err == nil {
			k.fetchedAt = now
			found = k.keys.Key(kid)
		}
	}
	if len(found) == 0 {
		return jose.JSONWebKey{}, unauthenticated("unknown signing key")
	}
	return found[0], nil
}

func (k *keySet) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return err
	}
	res, err := k.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return port.Errorf(port.CodeUnavailable, "status %d", res.StatusCode)
	}
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&set); err != nil {
		return err
	}
	k.keys = set
	return nil
}
