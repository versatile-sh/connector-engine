package credential_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/port"
	secretsmem "github.com/versatile-sh/connector-engine/secrets/memory"
	storemem "github.com/versatile-sh/connector-engine/store/memory"
)

type enrolling struct {
	srv        *httptest.Server
	registered atomic.Int32
	lastBody   atomic.Value
	code       atomic.Value
}

type enrollOpts struct {
	registration bool
	cimd         bool
	refuse       bool
	noPKCE       bool
	authMethods  []string
}

func newEnrolling(t *testing.T, o enrollOpts) *enrolling {
	e := &enrolling{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		m := map[string]any{"issuer": e.srv.URL, "authorization_endpoint": e.srv.URL + "/authorize", "token_endpoint": e.srv.URL + "/token", "client_id_metadata_document_supported": o.cimd}
		if !o.noPKCE {
			m["code_challenge_methods_supported"] = []string{"S256"}
		}
		if o.registration {
			m["registration_endpoint"] = e.srv.URL + "/register"
		}
		if o.authMethods != nil {
			m["token_endpoint_auth_methods_supported"] = o.authMethods
		}
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		e.lastBody.Store(string(body))
		if o.refuse {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		e.registered.Add(1)
		var in map[string]any
		json.Unmarshal(body, &in)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "dyn-id", "client_secret": "dyn-secret", "registration_access_token": "rat", "registration_client_uri": e.srv.URL + "/register/dyn-id", "client_secret_expires_at": 0, "redirect_uris": in["redirect_uris"]})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		id := r.PostForm.Get("client_id")
		if user, _, ok := r.BasicAuth(); ok {
			id = user
		}
		e.code.Store(id + "|" + r.PostForm.Get("code_verifier"))
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "refresh_token": "rt", "expires_in": 3600})
	})
	e.srv = httptest.NewTLSServer(mux)
	t.Cleanup(e.srv.Close)
	return e
}

func resolverFor(e *enrolling) (*credential.ClientResolver, *credential.Broker) {
	d := &credential.Discoverer{HTTP: e.srv.Client()}
	r := credential.NewClientResolver().WithStore(storemem.New(), secretsmem.New()).WithDiscovery(d)
	return r, &credential.Broker{HTTP: e.srv.Client(), Discovery: d}
}

var app = port.ClientApp{Name: "connector-engine", RedirectURL: "https://console.example/oauth/callback"}

func TestAcquireRegistersDynamicallyThenAuthorizesAndExchanges(t *testing.T) {
	e := newEnrolling(t, enrollOpts{registration: true})
	r, b := resolverFor(e)
	ctx := context.Background()
	c, err := r.Acquire(ctx, "t1", e.srv.URL, app)
	if err != nil || c.ClientID != "dyn-id" || string(c.ClientSecret) != "dyn-secret" || c.Registration != port.RegistrationDCR || c.RedirectURL != app.RedirectURL {
		t.Fatalf("%+v %v", c, err)
	}
	var sent map[string]any
	json.Unmarshal([]byte(e.lastBody.Load().(string)), &sent)
	if sent["token_endpoint_auth_method"] != "none" || sent["application_type"] != "web" || sent["redirect_uris"].([]any)[0] != app.RedirectURL {
		t.Fatalf("request %v", sent)
	}
	again, err := r.Acquire(ctx, "t1", e.srv.URL, app)
	if err != nil || again.ClientID != "dyn-id" || e.registered.Load() != 1 {
		t.Fatalf("registered %d times: %v", e.registered.Load(), err)
	}
	if stored, err := r.Resolve(ctx, "t1", e.srv.URL); err != nil || stored.ClientID != "dyn-id" || string(stored.ClientSecret) != "dyn-secret" {
		t.Fatalf("resolve %+v %v", stored, err)
	}
	if _, err := r.Resolve(ctx, "t2", e.srv.URL); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("another tenant: %v", err)
	}
	req := port.AuthorizationRequest{Client: c, Resource: "https://mcp.example/mcp", State: "st", Verifier: credential.NewVerifier()}
	addr, err := b.Begin(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, addr)
	if q.Get("client_id") != "dyn-id" || q.Get("code_challenge_method") != "S256" || q.Get("resource") != "https://mcp.example/mcp" || q.Get("state") != "st" {
		t.Fatalf("authorize %v", q)
	}
	if _, err := b.Complete(ctx, req, "code1"); err != nil || !strings.HasPrefix(e.code.Load().(string), "dyn-id|") {
		t.Fatalf("token %v %v", e.code.Load(), err)
	}
}

func TestAcquirePrefersCIMDOnlyWithAnHTTPSMetadataAddress(t *testing.T) {
	e := newEnrolling(t, enrollOpts{registration: true, cimd: true})
	r, _ := resolverFor(e)
	withDoc := app
	withDoc.MetadataURL = "https://console.example/oauth/client.json"
	c, err := r.Acquire(context.Background(), "t1", e.srv.URL, withDoc)
	if err != nil || c.ClientID != withDoc.MetadataURL || c.Registration != port.RegistrationCIMD || e.registered.Load() != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	r2, _ := resolverFor(e)
	c, err = r2.Acquire(context.Background(), "t1", e.srv.URL, app)
	if err != nil || c.Registration != port.RegistrationDCR {
		t.Fatalf("without a document: %+v %v", c, err)
	}
}

func TestAcquireReregistersWhenTheRedirectChanges(t *testing.T) {
	e := newEnrolling(t, enrollOpts{registration: true})
	r, _ := resolverFor(e)
	other := app
	other.RedirectURL = "https://other.example/oauth/callback"
	for _, a := range []port.ClientApp{app, other} {
		if _, err := r.Acquire(context.Background(), "t1", e.srv.URL, a); err != nil {
			t.Fatal(err)
		}
	}
	if e.registered.Load() != 2 {
		t.Fatalf("registered %d times", e.registered.Load())
	}
}

func TestAcquireAsksForAClientWhenTheIssuerAllowsNoRegistration(t *testing.T) {
	for name, o := range map[string]enrollOpts{
		"no registration endpoint": {},
		"refused registration":     {registration: true, refuse: true},
		"no PKCE":                  {registration: true, noPKCE: true},
	} {
		e := newEnrolling(t, o)
		r, _ := resolverFor(e)
		if _, err := r.Acquire(context.Background(), "t1", e.srv.URL, app); !port.IsCode(err, port.CodeClientRegistrationRequired) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestAcquireUsesTheConfiguredClientBeforeRegistering(t *testing.T) {
	e := newEnrolling(t, enrollOpts{registration: true})
	r := credential.NewClientResolver(credential.Registered{Client: port.OAuthClient{Issuer: e.srv.URL, ClientID: "mine"}}).WithStore(storemem.New(), secretsmem.New()).WithDiscovery(&credential.Discoverer{HTTP: e.srv.Client()})
	c, err := r.Acquire(context.Background(), "t1", e.srv.URL, app)
	if err != nil || c.ClientID != "mine" || e.registered.Load() != 0 {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestAcquireAsksForSecretAuthWhenNoneIsNotOffered(t *testing.T) {
	e := newEnrolling(t, enrollOpts{registration: true, authMethods: []string{"client_secret_basic", "client_secret_post"}})
	r, _ := resolverFor(e)
	if _, err := r.Acquire(context.Background(), "t1", e.srv.URL, app); err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	json.Unmarshal([]byte(e.lastBody.Load().(string)), &sent)
	if sent["token_endpoint_auth_method"] != "client_secret_post" {
		t.Fatalf("request %v", sent)
	}
}
