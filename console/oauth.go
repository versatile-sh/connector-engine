package console

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
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

var browserSchemes = []string{"https", "javascript", "data", "file", "blob", "about", "ftp", "ws", "wss", "mailto", "tel", "sms", "view-source", "chrome", "chrome-extension", "edge", "moz-extension", "intent", "filesystem", "jar", "vbscript", "resource"}

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

// RedirectAllowed reports whether an OAuth redirect URI may be registered: loopback http on any port (RFC 8252 §7.3) or an app's private-use scheme (§7.1), never https or a scheme a browser handles itself.
func RedirectAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Scheme == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" {
		return loopbackName(u.Hostname())
	}
	return !slices.Contains(browserSchemes, scheme)
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
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "connect this app from the console: open " + c.origin(r) + "/access"})
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

func (c *Console) preflight(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) protectedResource(w http.ResponseWriter, r *http.Request) {
	cors(w)
	o := c.origin(r)
	writeJSON(w, http.StatusOK, map[string]any{"resource": o + "/mcp", "authorization_servers": []string{o}, "bearer_methods_supported": []string{"header"}, "resource_name": "connector-engine"})
}

func (c *Console) authServerMetadata(w http.ResponseWriter, r *http.Request) {
	cors(w)
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
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
	})
}

func oauthError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": detail})
}

func (c *Console) register(w http.ResponseWriter, r *http.Request) {
	cors(w)
	c.asMu.Lock()
	now := time.Now()
	c.registered = slices.DeleteFunc(c.registered, func(t time.Time) bool { return now.Sub(t) > time.Minute })
	limited := len(c.registered) >= registrationsPerMin
	if !limited {
		c.registered = append(c.registered, now)
	}
	c.asMu.Unlock()
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
	if !cl.allows(redirect) {
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
	c.asMu.Lock()
	for k, v := range c.pending {
		if now.After(v.expires) || len(c.pending) >= maxPendingAuth {
			delete(c.pending, k)
		}
	}
	c.pending[id] = authRequest{client: cl, redirect: redirect, state: qv.Get("state"), challenge: qv.Get("code_challenge"), resource: qv.Get("resource"), expires: now.Add(authRequestTTL)}
	c.asMu.Unlock()
	http.Redirect(w, r, "/connect?request="+id, http.StatusSeeOther)
}

func (c *Console) pendingRequest(id string, take bool) (authRequest, bool) {
	c.asMu.Lock()
	defer c.asMu.Unlock()
	req, ok := c.pending[id]
	if take {
		delete(c.pending, id)
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

func (c *Console) consentInfo(w http.ResponseWriter, r *http.Request) {
	req, ok := c.pendingRequest(r.PathValue("id"), false)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "this request expired or was already answered"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"client": req.client.Name, "redirect": redirectTarget(req.redirect)})
}

func (c *Console) consent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	req, ok := c.pendingRequest(r.PathValue("id"), true)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "this request expired or was already answered"})
		return
	}
	params := map[string]string{"state": req.state, "iss": c.origin(r)}
	if !in.Approve {
		params["error"] = "access_denied"
		writeJSON(w, http.StatusOK, map[string]string{"redirect": withParams(req.redirect, params)})
		return
	}
	a, err := c.BindAgent(r.Context(), req.client.Name, c.MaxAgentTTL)
	if err != nil {
		c.fail(w, err)
		return
	}
	req.client.LastUsed = time.Now().UTC()
	_ = c.State.PutOAuthClient(req.client)
	code := randomID()
	c.asMu.Lock()
	now := time.Now()
	for k, v := range c.codes {
		if now.After(v.expires) {
			delete(c.codes, k)
		}
	}
	c.codes[code] = authCode{clientID: req.client.ID, redirect: req.redirect, challenge: req.challenge, resource: req.resource, agent: a.Name, expires: now.Add(authCodeTTL)}
	c.asMu.Unlock()
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
	cors(w)
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
	c.asMu.Lock()
	code, ok := c.codes[f.Get("code")]
	delete(c.codes, f.Get("code"))
	c.asMu.Unlock()
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
		c.issue(w, r, "", RefreshGrant{ClientID: code.clientID, Agent: code.agent, Resource: firstOf(code.resource, f.Get("resource"))})
	}
}

func refreshKey(token string) string {
	sum := sha256.Sum256([]byte("refresh:" + token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *Console) refresh(w http.ResponseWriter, r *http.Request, f url.Values) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	key := refreshKey(f.Get("refresh_token"))
	g, ok := c.State.RefreshGrant(key)
	a, known := c.State.Agent(g.Agent)
	switch {
	case !ok || g.ClientID != f.Get("client_id"):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is unknown, expired or of another client")
		return
	case !known || c.State.Revoked(g.Agent, g.Issued.Unix()):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the access of this app was revoked; connect it again")
		return
	}
	if time.Until(a.ExpiresAt) < c.MaxAgentTTL/2 {
		if _, err := c.BindAgent(r.Context(), a.Name, c.MaxAgentTTL); err != nil {
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "the gateway is not reachable")
			return
		}
	}
	c.issue(w, r, key, RefreshGrant{ClientID: g.ClientID, Agent: g.Agent, Resource: g.Resource})
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
