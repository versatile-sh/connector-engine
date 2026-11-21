package credential

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/fetch"
	"github.com/versatile-sh/connector-engine/port"
)

type metadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
}

// Broker is the OAuthBroker for pre-registered clients: PKCE S256, a bound state, single-flight refresh and revocation. Issuer requests go through HTTP, or a zero egress guard when nil, and never follow a redirect.
type Broker struct {
	HTTP *http.Client

	once    sync.Once
	http    *http.Client
	mu      sync.Mutex
	found   map[string]metadata
	refresh singleflight.Group
}

// NewVerifier returns a fresh PKCE code verifier for an AuthorizationRequest.
func NewVerifier() string { return oauth2.GenerateVerifier() }

func (b *Broker) client() *http.Client {
	b.once.Do(func() {
		c := egress.Guard{}.Client()
		if b.HTTP != nil {
			c = new(*b.HTTP)
		}
		c.CheckRedirect = noRedirect
		b.http = c
	})
	return b.http
}

func (b *Broker) discover(ctx context.Context, issuer string) (metadata, error) {
	b.mu.Lock()
	m, ok := b.found[issuer]
	b.mu.Unlock()
	if ok {
		return m, nil
	}
	base := strings.TrimRight(issuer, "/")
	var lastErr error
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		body, err := fetch.Do(ctx, b.client(), http.MethodGet, base+path, nil, nil)
		if err != nil {
			lastErr = port.Errorf(port.CodeOf(err), "issuer metadata is unreachable")
			continue
		}
		if err := json.Unmarshal(body, &m); err != nil || !secure(m.TokenEndpoint) || (m.RevocationEndpoint != "" && !secure(m.RevocationEndpoint)) {
			lastErr = port.Errorf(port.CodeInvalidArgument, "issuer metadata has no https token endpoint")
			continue
		}
		b.mu.Lock()
		if b.found == nil {
			b.found = map[string]metadata{}
		}
		b.found[issuer] = m
		b.mu.Unlock()
		return m, nil
	}
	return m, lastErr
}

func (b *Broker) config(ctx context.Context, c port.OAuthClient, scopes []string) (*oauth2.Config, metadata, context.Context, error) {
	m, err := b.discover(ctx, c.Issuer)
	if err != nil {
		return nil, m, ctx, err
	}
	cfg := &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: string(c.ClientSecret),
		RedirectURL:  c.RedirectURL,
		Scopes:       scopes,
		Endpoint:     oauth2.Endpoint{AuthURL: m.AuthorizationEndpoint, TokenURL: m.TokenEndpoint},
	}
	return cfg, m, context.WithValue(ctx, oauth2.HTTPClient, b.client()), nil
}

func withResource(r port.AuthorizationRequest, opts ...oauth2.AuthCodeOption) []oauth2.AuthCodeOption {
	if r.Resource != "" {
		opts = append(opts, oauth2.SetAuthURLParam("resource", r.Resource))
	}
	return opts
}

func checkRequest(r port.AuthorizationRequest) error {
	if r.State == "" || r.Verifier == "" {
		return port.Errorf(port.CodeInvalidArgument, "an authorization carries a state and a PKCE verifier")
	}
	return nil
}

// Begin returns the provider address to send the person to, with PKCE S256, the state and the resource indicator.
func (b *Broker) Begin(ctx context.Context, r port.AuthorizationRequest) (string, error) {
	if err := checkRequest(r); err != nil {
		return "", err
	}
	cfg, _, _, err := b.config(ctx, r.Client, r.Scopes)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(r.State, withResource(r, oauth2.S256ChallengeOption(r.Verifier))...), nil
}

// Complete exchanges the code for tokens, proving the verifier.
func (b *Broker) Complete(ctx context.Context, r port.AuthorizationRequest, code string) (port.TokenSet, error) {
	if err := checkRequest(r); err != nil {
		return port.TokenSet{}, err
	}
	cfg, _, ctx, err := b.config(ctx, r.Client, r.Scopes)
	if err != nil {
		return port.TokenSet{}, err
	}
	tok, err := cfg.Exchange(ctx, code, withResource(r, oauth2.VerifierOption(r.Verifier))...)
	if err != nil {
		return port.TokenSet{}, mapTokenError(err)
	}
	return tokenSet(tok, nil), nil
}

// Refresh trades a refresh token for new tokens; concurrent calls with the same token share one request.
func (b *Broker) Refresh(ctx context.Context, c port.OAuthClient, refreshToken []byte) (port.TokenSet, error) {
	sum := sha256.Sum256(refreshToken)
	key := c.Issuer + "|" + c.ClientID + "|" + hex.EncodeToString(sum[:])
	v, err, _ := b.refresh.Do(key, func() (any, error) {
		cfg, _, ctx, err := b.config(ctx, c, nil)
		if err != nil {
			return nil, err
		}
		tok, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: string(refreshToken)}).Token()
		if err != nil {
			return nil, mapTokenError(err)
		}
		return tokenSet(tok, refreshToken), nil
	})
	if err != nil {
		return port.TokenSet{}, err
	}
	return v.(port.TokenSet), nil
}

// Revoke calls the issuer's revocation endpoint when its metadata lists one; without one it does nothing.
func (b *Broker) Revoke(ctx context.Context, c port.OAuthClient, token []byte) error {
	m, err := b.discover(ctx, c.Issuer)
	if err != nil || m.RevocationEndpoint == "" {
		return err
	}
	form := url.Values{"token": {string(token)}, "client_id": {c.ClientID}}
	if len(c.ClientSecret) > 0 {
		form.Set("client_secret", string(c.ClientSecret))
	}
	if _, err := fetch.Do(ctx, b.client(), http.MethodPost, m.RevocationEndpoint, strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); err != nil {
		return port.Errorf(port.CodeOf(err), "revocation endpoint refused the request")
	}
	return nil
}

func secure(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func tokenSet(t *oauth2.Token, previousRefresh []byte) port.TokenSet {
	set := port.TokenSet{AccessToken: []byte(t.AccessToken), RefreshToken: []byte(t.RefreshToken), ExpiresAt: t.Expiry}
	if len(set.RefreshToken) == 0 {
		set.RefreshToken = previousRefresh
	}
	if scope, ok := t.Extra("scope").(string); ok {
		set.Scopes = strings.Fields(scope)
	}
	return set
}

func mapTokenError(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.ErrorCode == "invalid_grant" {
		return port.Errorf(port.CodeCredentialRequired, "invalid_grant: the refresh token or the code is no longer valid")
	}
	return port.Errorf(port.CodeUnavailable, "token endpoint refused the request")
}
