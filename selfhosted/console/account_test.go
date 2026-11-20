package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

type accountRig struct {
	t   *testing.T
	c   *console.Console
	srv *httptest.Server
}

func newAccountRig(t *testing.T) *accountRig {
	t.Helper()
	state, err := console.OpenState(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: &console.Engine{}, State: state, Sessions: console.NewSessions(state), Secrets: &sealed.Box{}}
	srv := httptest.NewUnstartedServer(nil)
	c.Guard = console.Guard{Hosts: []string{srv.Listener.Addr().String()}}
	srv.Config.Handler = c.Handler()
	srv.Start()
	t.Cleanup(srv.Close)
	r := &accountRig{t: t, c: c, srv: srv}
	r.browser().ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, nil)
	return r
}

func (r *accountRig) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: r.t, base: r.srv.URL, client: &http.Client{Jar: jar}}
}

func (r *accountRig) signIn(user, pass string) (*browser, int) {
	b := r.browser()
	var out struct{ CSRF string }
	status, raw := b.send("POST", "/api/login", map[string]string{"username": user, "password": pass}, nil)
	if status == http.StatusOK {
		_ = json.Unmarshal(raw, &out)
		b.csrf = out.CSRF
	}
	return b, status
}

func isSignedIn(b *browser) bool {
	var s struct {
		SignedIn bool `json:"signed_in"`
	}
	b.ok("GET", "/api/session", nil, &s)
	return s.SignedIn
}

func TestChangingThePasswordKeepsThisSessionAndEndsTheOthers(t *testing.T) {
	r := newAccountRig(t)
	here, _ := r.signIn("me", "correct horse battery")
	other, _ := r.signIn("me", "correct horse battery")

	if status, _ := here.send("PUT", "/api/account/password", map[string]string{"current": "not the password", "password": "a new long password"}, nil); status != http.StatusUnauthorized {
		t.Fatalf("a wrong current password: %d", status)
	}
	if status, _ := here.send("PUT", "/api/account/password", map[string]string{"current": "correct horse battery", "password": "short"}, nil); status != http.StatusBadRequest {
		t.Fatalf("a short password: %d", status)
	}
	here.ok("PUT", "/api/account/password", map[string]string{"current": "correct horse battery", "password": "a new long password"}, nil)

	if !isSignedIn(here) {
		t.Error("the session that changed the password was signed out")
	}
	if isSignedIn(other) {
		t.Error("another session survived the password change")
	}
	if _, status := r.signIn("me", "correct horse battery"); status != http.StatusUnauthorized {
		t.Errorf("the old password still signs in: %d", status)
	}
	if _, status := r.signIn("me", "a new long password"); status != http.StatusOK {
		t.Errorf("the new password does not sign in: %d", status)
	}
}

func TestChangingThePasswordIsRateLimitedLikeTheLogin(t *testing.T) {
	r := newAccountRig(t)
	b, _ := r.signIn("me", "correct horse battery")
	for range 5 {
		b.send("PUT", "/api/account/password", map[string]string{"current": "wrong wrong wrong", "password": "a new long password"}, nil)
	}
	status, raw := b.send("PUT", "/api/account/password", map[string]string{"current": "correct horse battery", "password": "a new long password"}, nil)
	if status != http.StatusTooManyRequests || !strings.Contains(string(raw), "slow_down") {
		t.Fatalf("after five wrong passwords: %d %s", status, raw)
	}
}

func TestRenamingTheOwnerChangesTheSignInName(t *testing.T) {
	r := newAccountRig(t)
	b, _ := r.signIn("me", "correct horse battery")
	if status, _ := b.send("PUT", "/api/account/username", map[string]string{"username": "  "}, nil); status != http.StatusBadRequest {
		t.Fatalf("an empty username: %d", status)
	}
	b.ok("PUT", "/api/account/username", map[string]string{"username": " ana "}, nil)
	var acct struct{ Username string }
	b.ok("GET", "/api/account", nil, &acct)
	if acct.Username != "ana" {
		t.Fatalf("username %q", acct.Username)
	}
	if _, status := r.signIn("me", "correct horse battery"); status != http.StatusUnauthorized {
		t.Errorf("the old name still signs in: %d", status)
	}
	if _, status := r.signIn("ana", "correct horse battery"); status != http.StatusOK {
		t.Errorf("the new name does not sign in: %d", status)
	}
}

func TestAHostPasswordResetEndsEverySession(t *testing.T) {
	r := newAccountRig(t)
	b, _ := r.signIn("me", "correct horse battery")
	host := httptest.NewServer(r.c.HostHandler())
	t.Cleanup(host.Close)

	resp, err := http.Post(host.URL+"/password", "text/plain", strings.NewReader("short\n"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a short password: %d", resp.StatusCode)
	}
	resp, err = http.Post(host.URL+"/password", "text/plain", strings.NewReader("forgot it but now long\n"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: %d", resp.StatusCode)
	}
	if isSignedIn(b) {
		t.Error("a session survived the reset")
	}
	if _, status := r.signIn("me", "forgot it but now long"); status != http.StatusOK {
		t.Errorf("the reset password does not sign in: %d", status)
	}
}
