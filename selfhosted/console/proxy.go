package console

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/versatile-sh/connector-engine/core/args"
)

type tokenClaims struct {
	Agent      string  `json:"agent"`
	Session    string  `json:"session"`
	OnBehalfOf string  `json:"on_behalf_of"`
	IssuedAt   float64 `json:"iat"`
}

// bearerClaims reads a bearer token's claims without checking its signature: only to refuse a revoked key, which the engine would reject anyway,
// and to key per-app settings. It never labels activity; a label needs a request the engine accepted.
func bearerClaims(header string) (tokenClaims, bool) {
	scheme, raw, found := strings.Cut(header, " ")
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if !found || !strings.EqualFold(scheme, "Bearer") || len(parts) != 3 {
		return tokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, false
	}
	var claims tokenClaims
	if args.Parse(payload) != nil || json.Unmarshal(payload, &claims) != nil || claims.Agent == "" || !exactNames(payload) {
		return tokenClaims{}, false
	}
	return claims, true
}

func exactNames(payload []byte) bool {
	var names map[string]json.RawMessage
	_ = json.Unmarshal(payload, &names)
	for name := range names {
		for _, claim := range []string{"agent", "session", "on_behalf_of", "iat"} {
			if name != claim && strings.EqualFold(name, claim) {
				return false
			}
		}
	}
	return true
}

func (c *Console) refuseRevoked(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := bearerClaims(r.Header.Get("Authorization")); ok && c.State.Revoked(claims.Agent, int64(claims.IssuedAt)) {
			w.Header().Set("WWW-Authenticate", c.challengeValue(c.origin(r), `error="invalid_token", error_description="the access key was revoked"`))
			writeError(w, http.StatusUnauthorized, "the access key was revoked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Console) gateMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, "/access", http.StatusSeeOther)
			return
		}
		c.challenge(w, r, "")
	})
}

func (c *Console) agentProxy() http.Handler {
	target, _ := url.Parse(c.Engine.PublicURL)
	return &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del(csrfHeader)
			pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			if resp.StatusCode == http.StatusUnauthorized {
				resp.Header.Set("WWW-Authenticate", c.challengeValue(c.base(resp.Request.Header.Get("X-Forwarded-Host")), `error="invalid_token"`))
			}
			if calls, ok := resp.Request.Context().Value(captureKey{}).(*inflight); ok {
				calls.wrap(resp)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) { c.engineDown(w) },
	}
}

func (c *Console) engineDown(w http.ResponseWriter) {
	message := "the gateway is not answering; after a restart it stays locked until you run mise run unlock on this computer, then retry"
	if c.Secrets.Locked() {
		message = "the gateway is locked: run mise run unlock on this computer, then retry"
	}
	w.Header().Set("Retry-After", "5")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32000, "message": message}})
}
