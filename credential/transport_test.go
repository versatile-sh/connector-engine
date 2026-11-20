package credential_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/port"
)

func resolver(kind port.CredentialKind, secret string) credential.Resolver {
	return func(context.Context, port.SecretsRef) (port.CredentialKind, []byte, error) {
		return kind, []byte(secret), nil
	}
}

func TestInjectKinds(t *testing.T) {
	cases := []struct {
		name   string
		inj    credential.Injector
		kind   port.CredentialKind
		secret string
		header string
		want   string
	}{
		{"api key as bearer", credential.Injector{}, port.CredentialAPIKey, "k1", "Authorization", "Bearer k1"},
		{"api key in a named header", credential.Injector{APIKeyHeader: "X-Api-Key"}, port.CredentialAPIKey, "k1", "X-Api-Key", "k1"},
		{"basic", credential.Injector{}, port.CredentialBasic, "u:p", "Authorization", "Basic dTpw"},
		{"oauth token", credential.Injector{}, port.CredentialOAuth2User, "tok", "Authorization", "Bearer tok"},
		{"custom headers", credential.Injector{}, port.CredentialCustomHeaders, `{"X-Team":"a"}`, "X-Team", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := port.Outbound{}
			if err := c.inj.Inject(context.Background(), c.kind, []byte(c.secret), &out); err != nil {
				t.Fatal(err)
			}
			if got := out.Headers[c.header]; len(got) != 1 || got[0] != c.want {
				t.Fatalf("header %s = %v, want %q", c.header, got, c.want)
			}
		})
	}
	if err := (credential.Injector{}).Inject(context.Background(), "nope", nil, &port.Outbound{}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestTransportReplacesInboundCredentialAndRefusesOtherHosts(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r.Header.Clone() }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: credential.Transport{
		Applier:    credential.Applier{Resolve: resolver(port.CredentialAPIKey, "upstream"), Injector: credential.Injector{}},
		Ref:        port.SecretsRef{Credential: "c1"},
		AllowHosts: []string{u.Host},
	}}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Authorization", "Bearer agent-token")
	req.Header.Set("Cookie", "session=1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := seen.Get("Authorization"); got != "Bearer upstream" {
		t.Fatalf("Authorization = %q", got)
	}
	if seen.Get("Cookie") != "" {
		t.Fatal("cookie was forwarded")
	}
	if _, err := client.Get("http://example.invalid/x"); !port.IsCode(unwrap(err), port.CodePermissionDenied) {
		t.Fatalf("other host: %v", err)
	}
}

func TestEmptyAllowListDeniesEverything(t *testing.T) {
	client := &http.Client{Transport: credential.Transport{}}
	if _, err := client.Get("http://127.0.0.1:1/"); !port.IsCode(unwrap(err), port.CodePermissionDenied) {
		t.Fatalf("got %v", err)
	}
}

func unwrap(err error) error {
	if u, ok := err.(*url.Error); ok {
		return u.Err
	}
	return err
}
