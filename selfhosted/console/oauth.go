package console

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	authRequestTTL      = 10 * time.Minute
	authCodeTTL         = time.Minute
	accessTokenTTL      = time.Hour
	refreshTokenTTL     = 90 * 24 * time.Hour
	registrationsPerMin = 10
	maxPendingAuth      = 256
	maxClientName       = 64
)

var appSchemes = map[string]string{"cursor": "Cursor", "vscode": "VS Code"}

var hostedCallbacks = map[string]string{"https://antigravity.google/oauth-callback": "Antigravity"}

type authRequest struct {
	client    OAuthClient
	redirect  string
	state     string
	challenge string
	resource  string
	expires   time.Time
}

type authCode struct {
	clientID  string
	redirect  string
	challenge string
	resource  string
	agent     string
	expires   time.Time
}

func loopbackName(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// RedirectAllowed reports whether an OAuth redirect URI may be registered: loopback http on any port (RFC 8252 section 7.3), a known app's scheme or a reverse-DNS private-use scheme with a dot (RFC 8252 section 7.1), or one exactly listed app callback page; any other scheme, such as a single-word OS or file handler, is refused.
func RedirectAllowed(raw string) bool {
	if _, ok := hostedCallbacks[raw]; ok {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Scheme == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" {
		return loopbackName(u.Hostname())
	}
	_, known := appSchemes[scheme]
	return known || strings.Contains(scheme, ".")
}

func redirectMatches(registered, got string) bool {
	if registered == got {
		return true
	}
	r, err1 := url.Parse(registered)
	g, err2 := url.Parse(got)
	if err1 != nil || err2 != nil || r.Scheme != "http" || g.Scheme != "http" || !loopbackName(r.Hostname()) {
		return false
	}
	return r.Hostname() == g.Hostname() && r.EscapedPath() == g.EscapedPath() && r.RawQuery == g.RawQuery && g.User == nil && g.Fragment == ""
}

func (o OAuthClient) allows(redirect string) bool {
	return slices.ContainsFunc(o.RedirectURIs, func(r string) bool { return redirectMatches(r, redirect) })
}

func clientName(raw string) string {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw))
	if runes := []rune(name); len(runes) > maxClientName {
		name = string(runes[:maxClientName])
	}
	if Slug(name) == "" {
		return "MCP client"
	}
	return name
}

func (c *Console) origin(r *http.Request) string {
	return c.base(r.Host)
}

func (c *Console) base(host string) string {
	scheme := "http"
	if u, err := url.Parse(c.MCPURL); err == nil && u.Scheme != "" {
		scheme = u.Scheme
	}
	return scheme + "://" + host
}

func (c *Console) resourceOK(r *http.Request, resource string) bool {
	if resource == "" {
		return true
	}
	resource = strings.TrimRight(resource, "/")
	for _, h := range append([]string{r.Host}, c.Guard.Hosts...) {
		if base := c.base(h); strings.EqualFold(resource, base+"/mcp") || strings.EqualFold(resource, base) {
			return true
		}
	}
	return false
}

func (c *Console) challenge(w http.ResponseWriter, r *http.Request, extra string) {
	w.Header().Set("WWW-Authenticate", c.challengeValue(c.origin(r), extra))
	writeError(w, http.StatusUnauthorized, "connect this app from the console: open "+c.origin(r)+"/access")
}

func (c *Console) challengeValue(origin, extra string) string {
	v := `Bearer resource_metadata="` + origin + `/.well-known/oauth-protected-resource/mcp"`
	if extra != "" {
		v += ", " + extra
	}
	return v
}

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, MCP-Protocol-Version")
	h.Set("Access-Control-Max-Age", "600")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
}

type authServer struct {
	mu         sync.Mutex
	pending    map[string]authRequest
	codes      map[string]authCode
	registered minuteWindow
	refreshMu  sync.Mutex
}

type appRoute struct {
	method, path string
	serve        func(*Console, http.ResponseWriter, *http.Request)
}

var appRoutes = []appRoute{
	{http.MethodGet, "/.well-known/oauth-protected-resource", (*Console).protectedResource},
	{http.MethodGet, "/.well-known/oauth-protected-resource/mcp", (*Console).protectedResource},
	{http.MethodGet, "/.well-known/oauth-authorization-server", (*Console).authServerMetadata},
	{http.MethodPost, "/oauth/register", (*Console).register},
	{http.MethodPost, "/oauth/token", (*Console).token},
}

func isAppPath(p string) bool {
	return slices.ContainsFunc(appRoutes, func(r appRoute) bool { return r.path == p })
}

func (c *Console) appRoutes(mux *router) {
	for _, rt := range appRoutes {
		mux.handle(rt.method+" "+rt.path, func(w http.ResponseWriter, r *http.Request) {
			cors(w)
			rt.serve(c, w, r)
		})
		mux.handle("OPTIONS "+rt.path, func(w http.ResponseWriter, _ *http.Request) {
			cors(w)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func (c *Console) protectedResource(w http.ResponseWriter, r *http.Request) {
	o := c.origin(r)
	writeJSON(w, http.StatusOK, map[string]any{"resource": o + "/mcp", "authorization_servers": []string{o}, "bearer_methods_supported": []string{"header"}, "resource_name": "connector-engine"})
}

func (c *Console) authServerMetadata(w http.ResponseWriter, r *http.Request) {
	o := c.origin(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         o,
		"authorization_endpoint":                         o + "/oauth/authorize",
		"token_endpoint":                                 o + "/oauth/token",
		"registration_endpoint":                          o + "/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"authorization_response_iss_parameter_supported": true,
	})
}

func oauthError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": detail})
}

func (c *Console) register(w http.ResponseWriter, r *http.Request) {
	c.as.mu.Lock()
	now := time.Now()
	limited := c.as.registered.full(now, registrationsPerMin)
	if !limited {
		c.as.registered.hit(now)
	}
	c.as.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "60")
		oauthError(w, http.StatusTooManyRequests, "slow_down", "too many registrations, try again in a minute")
		return
	}
	var in struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIBody)).Decode(&in); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "the body is not valid JSON")
		return
	}
	var kept []string
	for _, u := range in.RedirectURIs {
		if RedirectAllowed(u) && !slices.Contains(kept, u) {
			kept = append(kept, u)
		}
	}
	if len(kept) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "only loopback http redirect URIs and app schemes are accepted")
		return
	}
	cl := OAuthClient{ID: randomID(), Name: clientName(in.ClientName), RedirectURIs: kept, LastUsed: now.UTC()}
	if err := c.State.PutOAuthClient(cl); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "cannot store the client")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": cl.ID, "client_id_issued_at": now.Unix(), "client_name": cl.Name, "redirect_uris": cl.RedirectURIs,
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

func (c *Console) lookupClient(id string) (OAuthClient, error) {
	if cl, ok := c.State.OAuthClient(id); ok {
		return cl, nil
	}
	if strings.HasPrefix(id, "https://") {
		return OAuthClient{}, errors.New("client_id metadata documents are not supported: register the app at /oauth/register first")
	}
	return OAuthClient{}, errors.New("unknown client_id: register the app first")
}

func withParams(redirect string, params map[string]string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return redirect
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Console) authorize(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	cl, err := c.lookupClient(qv.Get("client_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	redirect := qv.Get("redirect_uri")
	if redirect == "" && len(cl.RedirectURIs) == 1 {
		redirect = cl.RedirectURIs[0]
	}
	if !cl.allows(redirect) || !RedirectAllowed(redirect) {
		http.Error(w, "this redirect_uri is not registered for the client", http.StatusBadRequest)
		return
	}
	fail := func(code, detail string) {
		http.Redirect(w, r, withParams(redirect, map[string]string{"error": code, "error_description": detail, "state": qv.Get("state"), "iss": c.origin(r)}), http.StatusSeeOther)
	}
	switch {
	case qv.Get("response_type") != "code":
		fail("unsupported_response_type", "only the code flow is supported")
		return
	case qv.Get("code_challenge") == "" || qv.Get("code_challenge_method") != "S256":
		fail("invalid_request", "PKCE with S256 is required")
		return
	case !c.resourceOK(r, qv.Get("resource")):
		fail("invalid_target", "this server only issues tokens for its /mcp endpoint")
		return
	}
	id := randomID()
	now := time.Now()
	c.as.mu.Lock()
	expire(c.as.pending, now, func(v authRequest) time.Time { return v.expires })
	for k := range c.as.pending {
		if len(c.as.pending) < maxPendingAuth {
			break
		}
		delete(c.as.pending, k)
	}
	put(&c.as.pending, id, authRequest{client: cl, redirect: redirect, state: qv.Get("state"), challenge: qv.Get("code_challenge"), resource: qv.Get("resource"), expires: now.Add(authRequestTTL)})
	c.as.mu.Unlock()
	http.Redirect(w, r, "/connect?request="+id, http.StatusSeeOther)
}

func (c *Console) pendingRequest(id string, take bool) (authRequest, bool) {
	c.as.mu.Lock()
	defer c.as.mu.Unlock()
	req, ok := c.as.pending[id]
	if take {
		delete(c.as.pending, id)
	}
	return req, ok && time.Now().Before(req.expires)
}

func redirectTarget(redirect string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return redirect
	}
	if u.Host == "" {
		return u.Scheme + ":"
	}
	return u.Scheme + "://" + u.Host
}

// Destination is where a consent sends the browser back, in plain words: an app on this computer at a port, an app known by its scheme, or any app that opens a scheme.
type Destination struct {
	Kind   string `json:"kind"`
	Port   string `json:"port,omitempty"`
	App    string `json:"app,omitempty"`
	Scheme string `json:"scheme,omitempty"`
}

// DestinationOf names where a registered redirect URI leads: kind loopback with its port, app with a known app's name, or scheme.
func DestinationOf(redirect string) Destination {
	u, err := url.Parse(redirect)
	if err != nil {
		return Destination{Kind: "scheme"}
	}
	if app, ok := hostedCallbacks[redirect]; ok {
		return Destination{Kind: "app", App: app}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" && loopbackName(u.Hostname()) {
		return Destination{Kind: "loopback", Port: u.Port()}
	}
	if app, ok := appSchemes[scheme]; ok {
		return Destination{Kind: "app", App: app}
	}
	return Destination{Kind: "scheme", Scheme: scheme}
}

func (c *Console) consentInfo(w http.ResponseWriter, r *http.Request) {
	req, ok := c.pendingRequest(r.PathValue("id"), false)
	if !ok {
		writeError(w, http.StatusNotFound, "this request expired or was already answered")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"client": req.client.Name, "redirect": redirectTarget(req.redirect), "destination": DestinationOf(req.redirect)})
}

func (c *Console) userConfirms(w http.ResponseWriter, r *http.Request, password string) bool {
	if password == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "type your password to allow the app", Code: "password_required"})
		return false
	}
	u, _ := signedInUser(r.Context())
	ip := clientIP(r)
	if c.passwordFails.locked(time.Now(), ip, u.Username) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, apiError{Error: "too many wrong passwords, try again in a minute", Code: "slow_down"})
		return false
	}
	_, ok, err := c.State.CheckLogin(u.Username, password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read the account")
		return false
	}
	if !ok {
		c.passwordFails.fail(time.Now(), ip, u.Username)
		time.Sleep(wrongPasswordDelay)
		writeJSON(w, http.StatusForbidden, apiError{Error: "wrong password", Code: "wrong_password"})
		return false
	}
	return true
}

func (c *Console) agentFor(cl OAuthClient, person string) string {
	name := Slug(cl.Name)
	a, taken := c.State.Agent(name)
	if !taken || a.Owner == person && (a.ClientID == cl.ID || a.ClientID == "" && a.ViaBrowser && c.State.SignedIn(name, cl.ID)) {
		return name
	}
	for _, candidate := range []string{name + "-" + cl.ID[:8], name + "-" + cl.ID} {
		if c.freeFor(candidate, cl.ID, person) {
			return candidate
		}
	}
	for n := 2; ; n++ {
		if candidate := fmt.Sprintf("%s-%s-%d", name, cl.ID[:8], n); c.freeFor(candidate, cl.ID, person) {
			return candidate
		}
	}
}

func (c *Console) freeFor(name, clientID, person string) bool {
	a, taken := c.State.Agent(name)
	return !taken || a.ClientID == clientID && a.Owner == person
}

func (c *Console) consent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve  bool   `json:"approve"`
		Password string `json:"password"`
		Slot     string `json:"slot"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if _, ok := c.pendingRequest(id, false); !ok {
		writeError(w, http.StatusNotFound, "this request expired or was already answered")
		return
	}
	if in.Approve && !c.userConfirms(w, r, in.Password) {
		return
	}
	if _, ok := c.State.Slot(in.Slot); in.Approve && in.Slot != "" && !ok {
		c.fail(w, noSlot(in.Slot))
		return
	}
	req, ok := c.pendingRequest(id, true)
	if !ok {
		writeError(w, http.StatusNotFound, "this request expired or was already answered")
		return
	}
	params := map[string]string{"state": req.state, "iss": c.origin(r)}
	if !in.Approve {
		params["error"] = "access_denied"
		writeJSON(w, http.StatusOK, map[string]string{"redirect": withParams(req.redirect, params)})
		return
	}
	a, err := c.BindAgent(r.Context(), c.agentFor(req.client, actorOf(r.Context())), c.MaxAgentTTL, in.Slot)
	if err != nil {
		c.fail(w, err)
		return
	}
	if err := c.State.MarkBrowserConnected(a.Name, req.client.ID, time.Now()); err != nil {
		c.fail(w, err)
		return
	}
	req.client.LastUsed = time.Now().UTC()
	_ = c.State.PutOAuthClient(req.client)
	code := randomID()
	c.as.mu.Lock()
	now := time.Now()
	expire(c.as.codes, now, func(v authCode) time.Time { return v.expires })
	put(&c.as.codes, code, authCode{clientID: req.client.ID, redirect: req.redirect, challenge: req.challenge, resource: req.resource, agent: a.Name, expires: now.Add(authCodeTTL)})
	c.as.mu.Unlock()
	params["code"] = code
	writeJSON(w, http.StatusOK, map[string]string{"redirect": withParams(req.redirect, params)})
}

func pkceOK(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}

func (c *Console) token(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Pragma", "no-cache")
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIBody)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "the body is a form")
		return
	}
	f := r.PostForm
	if !c.resourceOK(r, f.Get("resource")) {
		oauthError(w, http.StatusBadRequest, "invalid_target", "this server only issues tokens for its /mcp endpoint")
		return
	}
	switch f.Get("grant_type") {
	case "authorization_code":
		c.exchangeCode(w, r, f)
	case "refresh_token":
		c.refresh(w, r, f)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "authorization_code or refresh_token")
	}
}

func (c *Console) exchangeCode(w http.ResponseWriter, r *http.Request, f url.Values) {
	c.as.mu.Lock()
	code, ok := c.as.codes[f.Get("code")]
	delete(c.as.codes, f.Get("code"))
	c.as.mu.Unlock()
	switch {
	case !ok || time.Now().After(code.expires):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is unknown, expired or already used")
	case code.clientID != f.Get("client_id"):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code was issued to another client")
	case f.Has("redirect_uri") && f.Get("redirect_uri") != code.redirect:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri differs from the authorization request")
	case !pkceOK(f.Get("code_verifier"), code.challenge):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code_verifier does not match")
	case f.Get("resource") != "" && code.resource != "" && strings.TrimRight(f.Get("resource"), "/") != strings.TrimRight(code.resource, "/"):
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource differs from the authorization request")
	default:
		c.issue(w, r, "", RefreshGrant{ClientID: code.clientID, Agent: code.agent, Resource: firstOf(code.resource, f.Get("resource")), Family: randomID()})
	}
}

func refreshKey(token string) string {
	sum := sha256.Sum256([]byte("refresh:" + token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *Console) refresh(w http.ResponseWriter, r *http.Request, f url.Values) {
	c.as.refreshMu.Lock()
	defer c.as.refreshMu.Unlock()
	key := refreshKey(f.Get("refresh_token"))
	g, ok := c.State.RefreshGrant(key)
	a, known := c.State.Agent(g.Agent)
	switch {
	case !ok:
		if reused, _ := c.State.RevokeReusedRefresh(key); reused {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "this refresh token was already used, so every token of its sign-in was ended; connect the app again")
			return
		}
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is unknown, expired or of another client")
		return
	case g.ClientID != f.Get("client_id"):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is unknown, expired or of another client")
		return
	case !known || c.State.Revoked(g.Agent, g.Issued.Unix()):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the access of this app was revoked; connect it again")
		return
	}
	if time.Until(a.ExpiresAt) < c.MaxAgentTTL/2 {
		if _, err := c.BindAgent(WithActor(r.Context(), a.Owner), a.Name, c.MaxAgentTTL, ""); err != nil {
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "the gateway is not reachable")
			return
		}
	}
	c.issue(w, r, key, RefreshGrant{ClientID: g.ClientID, Agent: g.Agent, Resource: g.Resource, Family: firstOf(g.Family, key)})
}

func (c *Console) issue(w http.ResponseWriter, r *http.Request, oldRefresh string, g RefreshGrant) {
	tok, err := c.agentToken(r.Context(), g.Agent, accessTokenTTL)
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "the gateway is not reachable")
		return
	}
	refresh := randomID()
	now := time.Now().UTC()
	g.Issued, g.Expires = now, now.Add(refreshTokenTTL)
	if err := c.State.RotateRefresh(oldRefresh, refreshKey(refresh), g); err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", "cannot store the refresh token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": int(accessTokenTTL.Seconds()), "refresh_token": refresh})
}
