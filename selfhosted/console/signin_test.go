package console_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"

	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
)

type protectedServer struct {
	*httptest.Server
	registrations atomic.Int32
	registerBody  atomic.Value
	registration  bool
}

func newProtectedServer(t *testing.T, registration bool) *protectedServer {
	t.Helper()
	p := &protectedServer{registration: registration}
	tools := mcp.NewServer(&mcp.Implementation{Name: "vault", Title: "Vault", Version: "1.0.0"}, nil)
	tools.AddTool(&mcp.Tool{Name: "lookup", Description: "looks up", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	inner := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return tools }, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-1" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+p.URL+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"resource": p.URL + "/mcp", "authorization_servers": []string{p.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		m := map[string]any{"issuer": p.URL, "authorization_endpoint": p.URL + "/authorize", "token_endpoint": p.URL + "/token", "code_challenge_methods_supported": []string{"S256"}}
		if p.registration {
			m["registration_endpoint"] = p.URL + "/register"
		}
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		p.registerBody.Store(in)
		p.registrations.Add(1)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "dyn-client", "redirect_uris": in["redirect_uris"]})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("client_id") != "dyn-client" || r.PostForm.Get("code") != "code-1" || r.PostForm.Get("code_verifier") == "" || r.PostForm.Get("resource") != p.URL+"/mcp" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 3600})
	})
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Close)
	return p
}

func signedInBrowser(t *testing.T) *browser {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base)
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", ServicePerson: "console"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Secrets: &sealed.Box{}}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	store, err := secretslocal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.Secrets.Unlock(store)
	return b
}

func importProtected(t *testing.T, b *browser, p *protectedServer) string {
	t.Helper()
	var pv console.Preview
	b.ok("POST", "/api/preview", map[string]any{"mcp": p.URL + "/mcp", "local": true}, &pv)
	if len(pv.Tools) != 0 || pv.Warning != "" || len(pv.Auth) != 1 || pv.Auth[0] != "oauth2_user" || pv.Title == "" {
		t.Fatalf("preview of a server that asks to sign in: %+v", pv)
	}
	var imported struct{ Connector string }
	b.ok("POST", "/api/import", map[string]any{"mcp": p.URL + "/mcp", "local": true}, &imported)
	if imported.Connector == "" {
		t.Fatal("no connector")
	}
	return imported.Connector
}

func functionNames(t *testing.T, b *browser) []string {
	t.Helper()
	var list struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &list)
	if len(list.Connectors) != 1 {
		t.Fatalf("connectors: %+v", list)
	}
	var names []string
	for _, f := range list.Connectors[0].Functions {
		names = append(names, f.Name)
	}
	return names
}

func TestAServerThatAsksToSignInIsImportedEmptyThenRegistersItselfAndListsAfterSignIn(t *testing.T) {
	p := newProtectedServer(t, true)
	b := signedInBrowser(t)
	conn := importProtected(t, b, p)
	if names := functionNames(t, b); len(names) != 0 {
		t.Fatalf("functions before sign-in: %v", names)
	}
	var start struct{ URL string }
	b.ok("POST", "/api/connectors/"+conn+"/oauth", map[string]string{}, &start)
	if p.registrations.Load() != 1 {
		t.Fatalf("registrations: %d", p.registrations.Load())
	}
	body := p.registerBody.Load().(map[string]any)
	if body["redirect_uris"].([]any)[0] != b.base+"/oauth/callback" {
		t.Fatalf("registered %v", body)
	}
	u, err := url.Parse(start.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Path != "/authorize" || q.Get("client_id") != "dyn-client" || q.Get("code_challenge_method") != "S256" || q.Get("resource") != p.URL+"/mcp" || q.Get("state") == "" {
		t.Fatalf("authorization address %s", start.URL)
	}
	status, _ := b.send("GET", "/oauth/callback?code=code-1&state="+url.QueryEscape(q.Get("state")), nil, nil)
	if status != http.StatusSeeOther {
		t.Fatalf("callback: %d", status)
	}
	if names := functionNames(t, b); len(names) != 1 || names[0] != "lookup" {
		t.Fatalf("functions after sign-in: %v", names)
	}
}

func TestAServerWithoutRegistrationAsksForAClient(t *testing.T) {
	p := newProtectedServer(t, false)
	b := signedInBrowser(t)
	conn := importProtected(t, b, p)
	status, raw := b.send("POST", "/api/connectors/"+conn+"/oauth", map[string]string{}, nil)
	if status != http.StatusConflict || !strings.Contains(string(raw), "client_registration_required") {
		t.Fatalf("%d %s", status, raw)
	}
}

func TestTheConsoleServesItsClientMetadataOnlyWithAPublicHTTPSAddress(t *testing.T) {
	for public, wantStatus := range map[string]int{"https://console.example": http.StatusOK, "http://127.0.0.1:9123": http.StatusNotFound, "https://localhost": http.StatusNotFound} {
		c := &console.Console{Engine: &console.Engine{PublicURL: "http://127.0.0.1:1"}, PublicURL: public, MCPURL: public + "/mcp", OAuthRedirect: public + "/oauth/callback", Guard: console.Guard{Hosts: []string{strings.SplitN(public, "//", 2)[1]}}}
		req := httptest.NewRequest(http.MethodGet, public+"/oauth/client.json", nil)
		rec := httptest.NewRecorder()
		c.Handler().ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("%s: %d %s", public, rec.Code, rec.Body)
		}
		if wantStatus == http.StatusOK {
			var doc map[string]any
			json.Unmarshal(rec.Body.Bytes(), &doc)
			if doc["client_id"] != public+"/oauth/client.json" || doc["redirect_uris"].([]any)[0] != public+"/oauth/callback" || doc["token_endpoint_auth_method"] != "none" {
				t.Fatalf("document %v", doc)
			}
		}
	}
}
