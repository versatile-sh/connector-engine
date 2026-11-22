package credential_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/port"
)

func TestResourceMetadataURLReadsTheChallenge(t *testing.T) {
	for in, want := range map[string][]string{
		`Bearer resource_metadata="https://a.example/.well-known/oauth-protected-resource", scope="x"`: {"https://a.example/.well-known/oauth-protected-resource"},
		`Bearer realm="x", resource_metadata=https://b.example/prm`:                                    {"https://b.example/prm"},
		`Basic realm="x"`: {""},
	} {
		if got := credential.ResourceMetadataURL([]string{in}); got != want[0] {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

type resourceServer struct {
	srv *httptest.Server
	prm func(w http.ResponseWriter)
}

func newResourceServer(t *testing.T, paths ...string) *resourceServer {
	rs := &resourceServer{}
	mux := http.NewServeMux()
	for _, p := range paths {
		mux.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) { rs.prm(w) })
	}
	rs.srv = httptest.NewTLSServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

func TestResourceFollowsTheChallengeThenTheWellKnownAddresses(t *testing.T) {
	rs := newResourceServer(t, "/.well-known/oauth-protected-resource/mcp", "/custom-prm")
	rs.prm = func(w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{"resource": rs.srv.URL + "/mcp/", "authorization_servers": []string{"https://as.example/"}})
	}
	d := &credential.Discoverer{HTTP: rs.srv.Client()}
	for name, challenges := range map[string][]string{"challenge": {`Bearer resource_metadata="` + rs.srv.URL + `/custom-prm"`}, "well-known with path": nil} {
		got, err := d.Resource(context.Background(), rs.srv.URL+"/mcp", challenges)
		if err != nil || got.Issuer != "https://as.example" || got.URL != rs.srv.URL+"/mcp" {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}
}

func TestResourceFallsBackToTheOriginWhenTheMetadataIsMissingOrNamesAnotherResource(t *testing.T) {
	rs := newResourceServer(t, "/.well-known/oauth-protected-resource")
	rs.prm = func(w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{"resource": "https://elsewhere.example/mcp", "authorization_servers": []string{"https://evil.example"}})
	}
	d := &credential.Discoverer{HTTP: rs.srv.Client()}
	got, err := d.Resource(context.Background(), rs.srv.URL+"/mcp", nil)
	if err != nil || got.Issuer != rs.srv.URL {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := d.Resource(context.Background(), "http://plain.example/mcp", nil); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("plain http: %v", err)
	}
}

func TestResourceRefusesRedirectsAndPrivateAddresses(t *testing.T) {
	target := newResourceServer(t, "/prm")
	target.prm = func(w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{"resource": "https://hop.example/mcp", "authorization_servers": []string{"https://as.example"}})
	}
	hop := httptest.NewTLSServer(http.RedirectHandler(target.srv.URL+"/prm", http.StatusFound))
	t.Cleanup(hop.Close)
	d := &credential.Discoverer{HTTP: hop.Client()}
	got, _ := d.Resource(context.Background(), hop.URL+"/mcp", []string{`Bearer resource_metadata="` + hop.URL + `/x"`})
	if got.Issuer != hop.URL {
		t.Fatalf("a redirect was followed: %+v", got)
	}
	guarded := &credential.Discoverer{}
	got, _ = guarded.Resource(context.Background(), target.srv.URL+"/mcp", nil)
	if got.Issuer != target.srv.URL {
		t.Fatalf("%+v", got)
	}
	if _, err := guarded.Server(context.Background(), target.srv.URL); err == nil {
		t.Fatal("the guard let a loopback issuer through")
	}
}

func TestServerTriesTheRFC8414ThenOIDCAddressesOfAPathIssuer(t *testing.T) {
	var asked []string
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.URL.Path != "/tenant1/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"issuer": srv.URL + "/tenant1", "token_endpoint": srv.URL + "/t", "code_challenge_methods_supported": []string{"S256"}})
	}))
	t.Cleanup(srv.Close)
	d := &credential.Discoverer{HTTP: srv.Client()}
	m, err := d.Server(context.Background(), srv.URL+"/tenant1")
	if err != nil || !m.SupportsS256() {
		t.Fatalf("%+v %v", m, err)
	}
	want := []string{"/.well-known/oauth-authorization-server/tenant1", "/.well-known/openid-configuration/tenant1", "/tenant1/.well-known/openid-configuration"}
	for i, p := range want {
		if asked[i] != p {
			t.Fatalf("asked %v, want %v", asked, want)
		}
	}
}

func TestServerRefusesMetadataOfAnotherIssuer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": "https://other.example", "token_endpoint": "https://other.example/t"})
	}))
	t.Cleanup(srv.Close)
	if _, err := (&credential.Discoverer{HTTP: srv.Client()}).Server(context.Background(), srv.URL); err == nil {
		t.Fatal("accepted another issuer's metadata")
	}
}
