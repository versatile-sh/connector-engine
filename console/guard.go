package console

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "cn_console"
	sessionTTL    = 12 * time.Hour
	csrfHeader    = "X-CSRF-Token"
)

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

func randomID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type session struct {
	csrf    string
	expires time.Time
}

// Sessions holds the owner's sessions in memory; a restart of the console signs the owner out.
type Sessions struct {
	mu   sync.Mutex
	byID map[string]session
}

// NewSessions returns an empty session table.
func NewSessions() *Sessions { return &Sessions{byID: map[string]session{}} }

// Start opens a session, sets its HttpOnly SameSite=Strict cookie and returns its CSRF token.
func (s *Sessions) Start(w http.ResponseWriter) string {
	id, csrf := randomID(), randomID()
	now := time.Now()
	s.mu.Lock()
	for k, v := range s.byID {
		if now.After(v.expires) {
			delete(s.byID, k)
		}
	}
	s.byID[id] = session{csrf: csrf, expires: now.Add(sessionTTL)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
	return csrf
}

// Get returns the CSRF token of the request's live session.
func (s *Sessions) Get(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.byID[c.Value]
	if !ok || time.Now().After(v.expires) {
		return "", false
	}
	return v.csrf, true
}

// End closes the request's session and clears its cookie.
func (s *Sessions) End(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.byID, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
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
	return p == "/mcp" || p == "/.well-known/oauth-protected-resource" || p == "/.well-known/oauth-protected-resource/mcp"
}

func csrfOK(want, got string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
