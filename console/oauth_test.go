package console_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/console"
)

const callback = "http://127.0.0.1:33418/callback"

type oauthRig struct {
	t      *testing.T
	base   string
	b      *browser
	noRedo *http.Client
	eng    *console.Engine
	public string
	alias  string
}

func newOAuthRig(t *testing.T) *oauthRig {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := lis.Addr().String()
	base := "http://" + host
	public, operator := bootEngine(t, "", func(c *config.Config) {
		c.Auth.Audience, c.Auth.ManageAudience = "urn:connector-engine:local:mcp", "urn:connector-engine:local"
	})
	up := newUpstream(t)
	state, err := console.OpenState(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{host}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true, "choices": map[string]string{"search": "on"}}, nil)
	var list struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &list)
	noRedo := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &oauthRig{t: t, base: base, b: b, noRedo: noRedo, eng: eng, public: public, alias: list.Connectors[0].Alias}
}

func (o *oauthRig) register(redirects ...string) (int, map[string]any) {
	o.t.Helper()
	raw, _ := json.Marshal(map[string]any{"client_name": "Claude Code", "redirect_uris": redirects})
	resp, err := http.Post(o.base+"/oauth/register", "application/json", strings.NewReader(string(raw)))
	if err != nil {
		o.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (o *oauthRig) startAuthorize(params url.Values) *http.Response {
	o.t.Helper()
	resp, err := o.noRedo.Get(o.base + "/oauth/authorize?" + params.Encode())
	if err != nil {
		o.t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func (o *oauthRig) approve(authURL string) *url.URL {
	o.t.Helper()
	resp, err := o.noRedo.Get(authURL)
	if err != nil {
		o.t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || loc.Path != "/connect" {
		o.t.Fatalf("authorize: %d %s", resp.StatusCode, loc)
	}
	id := loc.Query().Get("request")
	var info struct {
		Client, Redirect string
		Destination      console.Destination
	}
	o.b.ok("GET", "/api/oauth/requests/"+id, nil, &info)
	if info.Client != "Claude Code" || info.Redirect != "http://127.0.0.1:33418" || info.Destination != (console.Destination{Kind: "loopback", Port: "33418"}) {
		o.t.Fatalf("consent info: %+v", info)
	}
	if status, _ := o.b.send("POST", "/api/oauth/requests/"+id, map[string]bool{"approve": true}, map[string]string{"X-CSRF-Token": ""}); status != http.StatusForbidden {
		o.t.Fatalf("a consent without the CSRF token: %d", status)
	}
	var out struct{ Redirect string }
	o.b.ok("POST", "/api/oauth/requests/"+id, map[string]bool{"approve": true}, &out)
	if status, _ := o.b.send("POST", "/api/oauth/requests/"+id, map[string]bool{"approve": true}, nil); status != http.StatusNotFound {
		o.t.Fatalf("a consent answered twice: %d", status)
	}
	back, _ := url.Parse(out.Redirect)
	return back
}

func (o *oauthRig) code(clientID, verifier string) string {
	o.t.Helper()
	back := o.approve(o.base + "/oauth/authorize?" + url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback}, "state": {"s1"},
		"code_challenge": {challengeOf(verifier)}, "code_challenge_method": {"S256"}, "resource": {o.base + "/mcp"}}.Encode())
	if back.Query().Get("state") != "s1" || back.Query().Get("iss") != o.base {
		o.t.Fatalf("redirect back: %s", back)
	}
	return back.Query().Get("code")
}

func (o *oauthRig) token(form url.Values, origin string) (int, map[string]any) {
	o.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, o.base+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		o.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (o *oauthRig) listTools(token string) ([]string, error) {
	o.t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: o.base + "/mcp", HTTPClient: &http.Client{Transport: bearer(token)}}, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names, nil
}

func TestAnMCPClientConnectsWithTheURLOnlyThroughOAuth(t *testing.T) {
	o := newOAuthRig(t)
	ctx := context.Background()

	resp, err := http.Post(o.base+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	challenges, _ := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	if resp.StatusCode != http.StatusUnauthorized || len(challenges) != 1 || challenges[0].Params["resource_metadata"] != o.base+"/.well-known/oauth-protected-resource/mcp" {
		t.Fatalf("an MCP client without a token: %d %v", resp.StatusCode, resp.Header.Values("WWW-Authenticate"))
	}
	req, _ := http.NewRequest(http.MethodGet, o.base+"/mcp", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	if resp, err := o.noRedo.Do(req); err != nil || resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/access" {
		t.Fatalf("a browser on /mcp: %v %v", resp, err)
	}

	prm, err := oauthex.GetProtectedResourceMetadata(ctx, o.base+"/.well-known/oauth-protected-resource/mcp", o.base+"/mcp", http.DefaultClient)
	if err != nil || !slices.Equal(prm.AuthorizationServers, []string{o.base}) {
		t.Fatalf("protected resource metadata: %+v %v", prm, err)
	}
	asm, err := oauthex.GetAuthServerMeta(ctx, o.base+"/.well-known/oauth-authorization-server", o.base, http.DefaultClient)
	if err != nil || !slices.Contains(asm.CodeChallengeMethodsSupported, "S256") || asm.RegistrationEndpoint != o.base+"/oauth/register" {
		t.Fatalf("authorization server metadata: %+v %v", asm, err)
	}
	var engineMeta struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	r, err := http.Get(o.public + "/.well-known/oauth-protected-resource")
	if err != nil || r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&engineMeta) != nil {
		t.Fatalf("the engine's own metadata: %v %v", r, err)
	}
	r.Body.Close()
	if slices.Contains(engineMeta.AuthorizationServers, o.base) {
		t.Fatalf("the engine's own metadata changed: %v", engineMeta)
	}

	t.Run("the go-sdk client runs the whole flow", func(t *testing.T) {
		h, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
			DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "Claude Code", RedirectURIs: []string{callback}, GrantTypes: []string{"authorization_code", "refresh_token"}}},
			RedirectURL:                     callback,
			RequestRefreshToken:             true,
			AuthorizationCodeFetcher: func(_ context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
				back := o.approve(args.URL)
				return &auth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state"), Iss: back.Query().Get("iss")}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: o.base + "/mcp", OAuthHandler: h}, nil)
		if err != nil {
			t.Fatalf("connect with OAuth: %v", err)
		}
		defer cs.Close()
		res, err := cs.ListTools(ctx, nil)
		if err != nil || !slices.ContainsFunc(res.Tools, func(tool *mcp.Tool) bool { return tool.Name == o.alias+"__search" }) {
			t.Fatalf("tools/list after OAuth: %v %v", res, err)
		}
	})

	status, reg := o.register(callback, "https://vscode.dev/redirect", "javascript:alert(1)")
	clientID, _ := reg["client_id"].(string)
	if status != http.StatusCreated || clientID == "" || reg["token_endpoint_auth_method"] != "none" {
		t.Fatalf("register: %d %v", status, reg)
	}
	if uris, _ := reg["redirect_uris"].([]any); len(uris) != 1 || uris[0] != callback {
		t.Fatalf("kept redirect URIs: %v", reg["redirect_uris"])
	}

	verifier := strings.Repeat("v", 50)
	code := o.code(clientID, verifier)
	exchange := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}, "redirect_uri": {callback}, "code_verifier": {strings.Repeat("w", 50)}, "resource": {o.base + "/mcp"}}
	if status, out := o.token(exchange, ""); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("a wrong verifier: %d %v", status, out)
	}
	exchange.Set("code_verifier", verifier)
	if status, out := o.token(exchange, ""); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("a code used twice: %d %v", status, out)
	}

	exchange.Set("code", o.code(clientID, verifier))
	if status, _ := o.token(exchange, "https://evil.example"); status != http.StatusForbidden {
		t.Fatalf("a token request from a web origin: %d", status)
	}
	status, tok := o.token(exchange, "")
	access, _ := tok["access_token"].(string)
	refresh, _ := tok["refresh_token"].(string)
	if status != http.StatusOK || access == "" || refresh == "" || tok["token_type"] != "Bearer" {
		t.Fatalf("exchange: %d %v", status, tok)
	}
	if names, err := o.listTools(access); err != nil || !slices.Contains(names, o.alias+"__search") {
		t.Fatalf("tools/list with the OAuth token: %v %v", names, err)
	}
	var agents struct{ Agents []console.Agent }
	o.b.ok("GET", "/api/agents", nil, &agents)
	if len(agents.Agents) != 1 || agents.Agents[0].Name != "claude-code" {
		t.Fatalf("the approved app is not an access key: %+v", agents.Agents)
	}

	renew := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
	status, next := o.token(renew, "http://localhost:6274")
	access2, _ := next["access_token"].(string)
	refresh2, _ := next["refresh_token"].(string)
	if status != http.StatusOK || access2 == "" || refresh2 == "" || refresh2 == refresh {
		t.Fatalf("refresh: %d %v", status, next)
	}
	if status, out := o.token(renew, ""); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("a rotated refresh token reused: %d %v", status, out)
	}
	if _, err := o.listTools(access2); err != nil {
		t.Fatalf("tools/list after refresh: %v", err)
	}

	o.b.ok("DELETE", "/api/agents/claude-code", nil, nil)
	req, _ = http.NewRequest(http.MethodPost, o.base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer "+access2)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("a revoked app on /mcp: %d %s %v", resp.StatusCode, body, resp.Header)
	}
	renew.Set("refresh_token", refresh2)
	if status, out := o.token(renew, ""); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("a refresh after a revoke: %d %v", status, out)
	}
}

func (o *oauthRig) requestConsent(clientID, redirect string) string {
	o.t.Helper()
	resp := o.startAuthorize(url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect}, "state": {"s1"},
		"code_challenge": {challengeOf(strings.Repeat("v", 50))}, "code_challenge_method": {"S256"}})
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || loc.Path != "/connect" {
		o.t.Fatalf("authorize %s: %d %s", redirect, resp.StatusCode, loc)
	}
	return "/api/oauth/requests/" + loc.Query().Get("request")
}

func TestTheConsentPage(t *testing.T) {
	o := newOAuthRig(t)
	redirects := []string{callback, "cursor://anysphere.cursor-mcp/oauth/callback", "vscode://vscode.github-authentication/did-authenticate", "myapp:/oauth"}
	_, reg := o.register(redirects...)
	clientID, _ := reg["client_id"].(string)

	t.Run("says where the access goes in plain words", func(t *testing.T) {
		want := []console.Destination{{Kind: "loopback", Port: "33418"}, {Kind: "app", App: "Cursor"}, {Kind: "app", App: "VS Code"}, {Kind: "scheme", Scheme: "myapp"}}
		for i, redirect := range redirects {
			var info struct{ Destination console.Destination }
			o.b.ok("GET", o.requestConsent(clientID, redirect), nil, &info)
			if info.Destination != want[i] {
				t.Errorf("%s: %+v, want %+v", redirect, info.Destination, want[i])
			}
		}
	})
}

func TestTheAuthorizationServerRefusesWhatTheSpecForbids(t *testing.T) {
	o := newOAuthRig(t)
	for _, uris := range [][]string{{"https://evil.example/cb"}, {"javascript:alert(1)"}, {"http://evil.example/cb"}, {"file:///etc/passwd"}, {}} {
		if status, out := o.register(uris...); status != http.StatusBadRequest || out["error"] != "invalid_redirect_uri" {
			t.Errorf("register %v: %d %v", uris, status, out)
		}
	}
	_, reg := o.register(callback)
	clientID, _ := reg["client_id"].(string)
	good := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback}, "state": {"s"}, "code_challenge": {challengeOf(strings.Repeat("v", 50))}, "code_challenge_method": {"S256"}}
	with := func(k, v string) url.Values {
		out := url.Values{}
		for key, vals := range good {
			out[key] = slices.Clone(vals)
		}
		if v == "" {
			out.Del(k)
		} else {
			out.Set(k, v)
		}
		return out
	}
	for _, redirect := range []string{"http://127.0.0.1:33418/other", "http://localhost:33418/callback", "cursor://evil/cb", "https://evil.example/callback"} {
		if resp := o.startAuthorize(with("redirect_uri", redirect)); resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Location") != "" {
			t.Errorf("an unregistered redirect %s: %d %s", redirect, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	status, vscode := o.register("https://insiders.vscode.dev/redirect", "https://vscode.dev/redirect", "http://127.0.0.1/", "http://127.0.0.1:33418/")
	if uris, _ := vscode["redirect_uris"].([]any); status != http.StatusCreated || len(uris) != 2 {
		t.Fatalf("the VS Code registration: %d %v", status, vscode)
	}
	vs := with("client_id", vscode["client_id"].(string))
	vs.Set("redirect_uri", "http://127.0.0.1:51234/")
	if resp := o.startAuthorize(vs); resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/connect?request=") {
		t.Errorf("VS Code's loopback server on a free port: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := o.startAuthorize(with("client_id", "nobody")); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an unknown client: %d", resp.StatusCode)
	}
	if resp := o.startAuthorize(with("redirect_uri", "http://127.0.0.1:50000/callback")); resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/connect?request=") {
		t.Errorf("a loopback redirect on another port (RFC 8252): %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	for name, c := range map[string]struct {
		params url.Values
		want   string
	}{
		"no PKCE":          {with("code_challenge", ""), "invalid_request"},
		"plain PKCE":       {with("code_challenge_method", "plain"), "invalid_request"},
		"implicit flow":    {with("response_type", "token"), "unsupported_response_type"},
		"another resource": {with("resource", "https://api.example/mcp"), "invalid_target"},
	} {
		resp := o.startAuthorize(c.params)
		loc, _ := url.Parse(resp.Header.Get("Location"))
		if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc.String(), callback) || loc.Query().Get("error") != c.want || loc.Query().Get("state") != "s" {
			t.Errorf("%s: %d %s", name, resp.StatusCode, loc)
		}
	}
	if status, out := o.token(url.Values{"grant_type": {"password"}}, ""); status != http.StatusBadRequest || out["error"] != "unsupported_grant_type" {
		t.Errorf("the password grant: %d %v", status, out)
	}
	if status, out := o.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"made-up"}, "client_id": {clientID}}, ""); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Errorf("a made-up refresh token: %d %v", status, out)
	}
	if resp, err := http.Get(o.base + "/api/oauth/requests/nope"); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the consent API without a session: %v %v", resp, err)
	}
	limited := false
	for range 12 {
		if status, _ := o.register(callback); status == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Error("registration is not rate limited")
	}
}

func TestRedirectURIsAreLoopbackOrAppSchemes(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://127.0.0.1:33418/": true, "http://127.0.0.1/callback": true, "http://localhost:8787/callback": true, "http://[::1]:9/cb": true,
		"cursor://anysphere.cursor-mcp/oauth/callback": true, "vscode://vscode.github-authentication/did-authenticate": true, "com.example.app:/cb": true,
		"https://vscode.dev/redirect": false, "http://example.com/cb": false, "http://127.0.0.1.evil.example/cb": false, "javascript:alert(1)": false,
		"data:text/html,x": false, "file:///tmp/x": false, "http://u:p@127.0.0.1/cb": false, "http://127.0.0.1/cb#frag": false, "/relative": false,
	} {
		if got := console.RedirectAllowed(raw); got != want {
			t.Errorf("%s: %v, want %v", raw, got, want)
		}
	}
}
