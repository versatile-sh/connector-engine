package credential

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
)

const (
	maxMetadataBytes = 1 << 20
	metadataTimeout  = 10 * time.Second
)

var resourceMetadataParam = regexp.MustCompile(`resource_metadata\s*=\s*(?:"([^"]*)"|([^\s,"]+))`)

// ServerMetadata is the part of an authorization server's metadata (RFC 8414, OpenID Connect Discovery) the engine reads.
type ServerMetadata struct {
	Issuer                   string   `json:"issuer"`
	AuthorizationEndpoint    string   `json:"authorization_endpoint"`
	TokenEndpoint            string   `json:"token_endpoint"`
	RevocationEndpoint       string   `json:"revocation_endpoint"`
	RegistrationEndpoint     string   `json:"registration_endpoint"`
	CodeChallengeMethods     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethods []string `json:"token_endpoint_auth_methods_supported"`
	ClientIDMetadataDocument bool     `json:"client_id_metadata_document_supported"`
}

// Resource is what a protected MCP server says about its authorization: its canonical address, the resource indicator, and the issuer to sign in with.
type Resource struct {
	URL    string
	Issuer string
}

// Discoverer reads protected resource metadata (RFC 9728) and authorization server metadata (RFC 8414, OIDC) over HTTP, or a zero egress guard when nil.
// It follows no redirect, reads at most 1 MiB, accepts https or loopback http only and caches server metadata per issuer.
type Discoverer struct {
	HTTP *http.Client

	once  sync.Once
	http  *http.Client
	mu    sync.Mutex
	found map[string]ServerMetadata
}

func (d *Discoverer) client() *http.Client {
	d.once.Do(func() {
		c := egress.Guard{}.Client()
		if d.HTTP != nil {
			c = new(*d.HTTP)
		}
		c.CheckRedirect = noRedirect
		if c.Timeout == 0 {
			c.Timeout = metadataTimeout
		}
		d.http = c
	})
	return d.http
}

func fetchable(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, false
	}
	return u, u.Scheme == "https" || u.Scheme == "http" && egress.IsLoopback(u.Hostname())
}

func (d *Discoverer) get(ctx context.Context, raw string, out any) error {
	if _, ok := fetchable(raw); !ok {
		return port.Errorf(port.CodeInvalidArgument, "metadata address is not https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return port.Errorf(port.CodeInvalidArgument, "metadata address is invalid")
	}
	req.Header.Set("Accept", "application/json")
	res, err := d.client().Do(req)
	if err != nil {
		return port.Errorf(port.CodeUnavailable, "metadata is unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return port.Errorf(port.CodeUnavailable, "metadata answered %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxMetadataBytes+1))
	if err != nil || len(body) > maxMetadataBytes {
		return port.Errorf(port.CodeUnavailable, "metadata is unreadable or too large")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return port.Errorf(port.CodeInvalidArgument, "metadata is not valid JSON")
	}
	return nil
}

// ResourceMetadataURL returns the resource_metadata address a WWW-Authenticate challenge names, or "".
func ResourceMetadataURL(challenges []string) string {
	for _, h := range challenges {
		if m := resourceMetadataParam.FindStringSubmatch(h); m != nil {
			return m[1] + m[2]
		}
	}
	return ""
}

func canonical(u *url.URL) string {
	c := *u
	c.Scheme, c.Host, c.Fragment = strings.ToLower(c.Scheme), strings.ToLower(c.Host), ""
	c.Path = strings.TrimRight(c.Path, "/")
	return c.String()
}

func origin(u *url.URL) string { return u.Scheme + "://" + u.Host }

func metadataCandidates(u *url.URL, challenge string) []string {
	var out []string
	if challenge != "" {
		out = append(out, challenge)
	}
	o, path := origin(u), strings.TrimRight(u.Path, "/")
	if path != "" {
		out = append(out, o+"/.well-known/oauth-protected-resource"+path)
	}
	return append(out, o+"/.well-known/oauth-protected-resource")
}

// Resource finds the issuer of the MCP server at mcpURL from the WWW-Authenticate values of its 401, then from the well-known protected resource addresses; failing both it names the server's own origin, the pre-RFC 9728 convention.
// The metadata must name the same resource it was fetched for.
func (d *Discoverer) Resource(ctx context.Context, mcpURL string, challenges []string) (Resource, error) {
	u, ok := fetchable(mcpURL)
	if !ok {
		return Resource{}, port.Errorf(port.CodeInvalidArgument, "the server address is not https")
	}
	want := canonical(u)
	for _, candidate := range metadataCandidates(u, ResourceMetadataURL(challenges)) {
		var prm struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
		}
		if d.get(ctx, candidate, &prm) != nil {
			continue
		}
		r, err := url.Parse(prm.Resource)
		if err != nil || canonical(r) != want {
			continue
		}
		for _, as := range prm.AuthorizationServers {
			if _, ok := fetchable(as); ok {
				return Resource{URL: want, Issuer: strings.TrimRight(as, "/")}, nil
			}
		}
	}
	return Resource{URL: want, Issuer: origin(u)}, nil
}

func serverCandidates(issuer string) []string {
	u, err := url.Parse(issuer)
	if err != nil {
		return nil
	}
	o, path := origin(u), strings.TrimRight(u.Path, "/")
	if path == "" {
		return []string{o + "/.well-known/oauth-authorization-server", o + "/.well-known/openid-configuration"}
	}
	return []string{o + "/.well-known/oauth-authorization-server" + path, o + "/.well-known/openid-configuration" + path, o + path + "/.well-known/openid-configuration"}
}

func usable(m ServerMetadata, issuer string) bool {
	if m.Issuer != "" && strings.TrimRight(m.Issuer, "/") != strings.TrimRight(issuer, "/") {
		return false
	}
	for _, e := range []string{m.TokenEndpoint, m.RevocationEndpoint, m.RegistrationEndpoint} {
		if e != "" && !secure(e) {
			return false
		}
	}
	return secure(m.TokenEndpoint)
}

// Server returns the authorization server metadata of an issuer, trying the RFC 8414 address then the OpenID Connect ones, and refuses metadata that names another issuer or an endpoint that is not https.
func (d *Discoverer) Server(ctx context.Context, issuer string) (ServerMetadata, error) {
	d.mu.Lock()
	m, ok := d.found[issuer]
	d.mu.Unlock()
	if ok {
		return m, nil
	}
	var lastErr error
	for _, candidate := range serverCandidates(issuer) {
		m = ServerMetadata{}
		if err := d.get(ctx, candidate, &m); err != nil {
			lastErr = port.Errorf(port.CodeOf(err), "issuer metadata is unreachable")
			continue
		}
		if !usable(m, issuer) {
			lastErr = port.Errorf(port.CodeInvalidArgument, "issuer metadata has no https token endpoint")
			continue
		}
		d.mu.Lock()
		if d.found == nil {
			d.found = map[string]ServerMetadata{}
		}
		d.found[issuer] = m
		d.mu.Unlock()
		return m, nil
	}
	if lastErr == nil {
		lastErr = port.Errorf(port.CodeInvalidArgument, "the issuer is not an address")
	}
	return ServerMetadata{}, lastErr
}

// SupportsS256 reports whether the server advertises PKCE with S256, which every automatic registration requires.
func (m ServerMetadata) SupportsS256() bool { return slices.Contains(m.CodeChallengeMethods, "S256") }
