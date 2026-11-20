package console

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	sessionCookie = "cn_console"
	sessionTTL    = 30 * 24 * time.Hour
	csrfHeader    = "X-CSRF-Token"
)

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

func randomID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sessionKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// Sessions is the owner's sessions as the state file keeps them, keyed by the SHA-256 of their cookie, so a restart does not sign the owner out.
type Sessions struct {
	state *State
}

// NewSessions returns the sessions kept in state.
func NewSessions(state *State) *Sessions { return &Sessions{state: state} }

// Start opens a session, sets its HttpOnly SameSite=Strict cookie and returns its CSRF token.
func (s *Sessions) Start(w http.ResponseWriter) (string, error) {
	id, csrf := randomID(), randomID()
	if err := s.state.putSession(sessionKey(id), StoredSession{CSRF: csrf, Expires: time.Now().Add(sessionTTL)}); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
	return csrf, nil
}

// Get returns the CSRF token of the request's live session.
func (s *Sessions) Get(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	v, ok := s.state.session(sessionKey(c.Value))
	return v.CSRF, ok
}

// Key returns the state key of the request's session cookie, or "" without one.
func (s *Sessions) Key(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return sessionKey(c.Value)
	}
	return ""
}

// End closes the request's session and clears its cookie.
func (s *Sessions) End(w http.ResponseWriter, r *http.Request) error {
	if k := s.Key(r); k != "" {
		if err := s.state.endSession(k); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	return nil
}

// Guard refuses a request whose Host is not one of the console's own addresses (DNS rebinding) and a mutating request whose Origin is not the console (CSRF), and sets the security headers.
type Guard struct {
	Hosts []string
}

// LoopbackHosts is the Host allowlist of a console reached at public: its own host and 127.0.0.1, localhost and [::1], all at public's port.
func LoopbackHosts(public *url.URL) []string {
	port := public.Port()
	hosts := []string{strings.ToLower(public.Host)}
	for _, h := range []string{"127.0.0.1", "localhost", "[::1]"} {
		if h+":"+port != hosts[0] {
			hosts = append(hosts, h+":"+port)
		}
	}
	return hosts
}

func (g Guard) hostOK(host string) bool {
	return slices.Contains(g.Hosts, strings.ToLower(host))
}

func (g Guard) originOK(origin string) bool {
	host, ok := strings.CutPrefix(origin, "http://")
	if !ok {
		host, ok = strings.CutPrefix(origin, "https://")
	}
	return ok && g.hostOK(host)
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// Wrap applies the guard before next.
func (g Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if !g.hostOK(r.Host) {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		origin := r.Header.Get("Origin")
		if isAppPath(r.URL.Path) {
			if origin != "" && !appOrigin(origin) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if origin != "" && !g.originOK(origin) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		if !safeMethod(r.Method) && origin == "" && !isAgentPath(r.URL.Path) {
			http.Error(w, "a mutating request carries its Origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAgentPath(p string) bool {
	return p == "/mcp"
}

func appOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" {
		return false
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		return loopbackName(u.Hostname())
	}
	return true
}

func csrfOK(want, got string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
