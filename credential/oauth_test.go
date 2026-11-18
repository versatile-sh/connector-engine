package credential_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/credential"
	"github.com/agents-control/connector-engine/port"
)

type issuer struct {
	srv       *httptest.Server
	refreshes atomic.Int32
	revoked   atomic.Value
	lastForm  atomic.Value
}

func newIssuer(t *testing.T) *issuer {
	i := &issuer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": i.srv.URL + "/authorize",
			"token_endpoint":         i.srv.URL + "/token",
			"revocation_endpoint":    i.srv.URL + "/revoke",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		i.lastForm.Store(r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at1", "refresh_token": "rt1", "expires_in": 3600, "scope": "read write"})
		case "refresh_token":
			if r.PostForm.Get("refresh_token") != "rt1" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			i.refreshes.Add(1)
			time.Sleep(50 * time.Millisecond)
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at2", "expires_in": 3600})
		}
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		i.revoked.Store(r.PostForm.Get("token"))
	})
	i.srv = httptest.NewTLSServer(mux)
	t.Cleanup(i.srv.Close)
	return i
}

func TestBrokerAuthorizationRefreshAndRevoke(t *testing.T) {
	iss := newIssuer(t)
	b := &credential.Broker{HTTP: iss.srv.Client()}
	client := port.OAuthClient{Issuer: iss.srv.URL, ClientID: "cid", ClientSecret: []byte("sec"), RedirectURL: "https://engine.example/callback"}
	req := port.AuthorizationRequest{Client: client, Resource: "https://mcp.example", Scopes: []string{"read"}, State: "st", Verifier: credential.NewVerifier()}
	ctx := context.Background()

	addr, err := b.Begin(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, addr)
	for k, want := range map[string]string{"state": "st", "code_challenge_method": "S256", "resource": "https://mcp.example", "client_id": "cid", "scope": "read"} {
		if q.Get(k) != want {
			t.Fatalf("authorize %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Get("code_challenge") == "" {
		t.Fatal("no code challenge")
	}

	set, err := b.Complete(ctx, req, "code1")
	if err != nil {
		t.Fatal(err)
	}
	form := iss.lastForm.Load().(url.Values)
	if form.Get("code_verifier") != req.Verifier || form.Get("resource") != "https://mcp.example" {
		t.Fatalf("exchange form %v", form)
	}
	if string(set.AccessToken) != "at1" || string(set.RefreshToken) != "rt1" || len(set.Scopes) != 2 || set.ExpiresAt.IsZero() {
		t.Fatalf("token set %+v", set)
	}

	var wg sync.WaitGroup
	results := make([]port.TokenSet, 5)
	errs := make([]error, 5)
	for n := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[n], errs[n] = b.Refresh(ctx, client, []byte("rt1"))
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if iss.refreshes.Load() != 1 {
		t.Fatalf("%d refresh requests for five concurrent callers", iss.refreshes.Load())
	}
	if string(results[4].AccessToken) != "at2" || string(results[4].RefreshToken) != "rt1" {
		t.Fatalf("refreshed %+v", results[4])
	}

	if _, err := b.Refresh(ctx, client, []byte("stale")); !port.IsCode(err, port.CodeCredentialRequired) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if err := b.Revoke(ctx, client, []byte("rt1")); err != nil || iss.revoked.Load() != "rt1" {
		t.Fatalf("revoke: %v %v", err, iss.revoked.Load())
	}
}

func TestBeginNeedsStateAndVerifier(t *testing.T) {
	if _, err := (&credential.Broker{}).Begin(context.Background(), port.AuthorizationRequest{}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("got %v", err)
	}
}

func TestRegistrarPrefersTheTenantClient(t *testing.T) {
	r := credential.NewRegistrar(
		credential.Registered{Client: port.OAuthClient{Issuer: "https://i", ClientID: "default"}},
		credential.Registered{Tenant: "t1", Client: port.OAuthClient{Issuer: "https://i", ClientID: "mine"}},
	)
	for tenant, want := range map[port.TenantID]string{"t1": "mine", "t2": "default"} {
		c, err := r.Resolve(context.Background(), tenant, "https://i")
		if err != nil || c.ClientID != want || c.Registration != port.RegistrationPreRegistered {
			t.Fatalf("%s: %+v %v", tenant, c, err)
		}
	}
	if _, err := r.Resolve(context.Background(), "t1", "https://other"); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("got %v", err)
	}
}

func mustQuery(t *testing.T, addr string) url.Values {
	u, err := url.Parse(addr)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestBrokerRefusesRedirectsPrivateIssuersAndPlainEndpoints(t *testing.T) {
	iss := newIssuer(t)
	hop := httptest.NewTLSServer(http.RedirectHandler(iss.srv.URL+"/.well-known/oauth-authorization-server", http.StatusFound))
	t.Cleanup(hop.Close)
	plain := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"token_endpoint": "http://plain.example/token"})
	}))
	t.Cleanup(plain.Close)
	req := port.AuthorizationRequest{State: "st", Verifier: credential.NewVerifier()}
	for name, c := range map[string]struct {
		broker *credential.Broker
		issuer string
	}{
		"redirect":       {&credential.Broker{HTTP: hop.Client()}, hop.URL},
		"private issuer": {&credential.Broker{}, iss.srv.URL},
		"plain endpoint": {&credential.Broker{HTTP: plain.Client()}, plain.URL},
	} {
		req.Client = port.OAuthClient{Issuer: c.issuer, ClientID: "cid"}
		_, err := c.broker.Begin(context.Background(), req)
		if err == nil {
			t.Fatalf("%s: the broker followed it", name)
		}
		if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "302") {
			t.Fatalf("%s: the error echoes upstream details: %v", name, err)
		}
	}
}
