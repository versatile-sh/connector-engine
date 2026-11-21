package credential

import (
	"context"
	"net/http"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

var inboundCredentialHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie"}

// Resolver opens the credential a SecretsRef names; the plaintext lives only for the call it serves.
type Resolver func(ctx context.Context, ref port.SecretsRef) (port.CredentialKind, []byte, error)

// Applier opens a credential and renders it as outbound headers; the zero Applier injects nothing.
type Applier struct {
	Resolve  Resolver
	Injector port.CredentialInjector
}

// Headers returns the headers of ref's credential, or none when ref names no credential.
func (a Applier) Headers(ctx context.Context, ref port.SecretsRef) (map[string][]string, error) {
	if ref.Credential == "" || a.Resolve == nil || a.Injector == nil {
		return nil, nil
	}
	kind, secret, err := a.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := port.Outbound{}
	if err := a.Injector.Inject(ctx, kind, secret, &out); err != nil {
		return nil, err
	}
	return out.Headers, nil
}

// Transport is an http.RoundTripper that refuses hosts outside AllowHosts, drops any inbound credential and injects the upstream one.
type Transport struct {
	Base       http.RoundTripper
	Applier    Applier
	Ref        port.SecretsRef
	AllowHosts []string
}

// NewClient returns an HTTP client over rt that hands a 3xx answer back as the answer instead of following it.
func NewClient(rt http.RoundTripper) *http.Client {
	return &http.Client{Transport: rt, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// HostAllowed reports whether host, with or without a port, is listed in allow as host or host:port.
func HostAllowed(allow []string, host, hostname string) bool {
	return slices.Contains(allow, host) || slices.Contains(allow, hostname)
}

// RoundTrip sends req once the host is allowed, with the upstream credential and never the caller's.
func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !HostAllowed(t.AllowHosts, req.URL.Host, req.URL.Hostname()) {
		return nil, port.Errorf(port.CodePermissionDenied, "egress to %s is not allowed", req.URL.Host)
	}
	headers, err := t.Applier.Headers(req.Context(), t.Ref)
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	for _, h := range inboundCredentialHeaders {
		req.Header.Del(h)
	}
	for k, v := range headers {
		req.Header[http.CanonicalHeaderKey(k)] = slices.Clone(v)
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
