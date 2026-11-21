package console_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

func TestAConsoleBehindATLSProxyAnswersOnlyItsPublicHostAndMarksTheCookieSecure(t *testing.T) {
	public, _ := url.Parse("https://console.example.com")
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: &console.Engine{}, State: state, Guard: console.Guard{Hosts: console.LoopbackHosts(public)}, Secrets: &sealed.Box{}}
	c.Sessions = console.NewSessions(state)
	c.Sessions.Secure = true
	h := c.Handler()
	send := func(host, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{"username":"me","password":"correct horse battery"}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if got := send("evil.example", "https://evil.example").Code; got != http.StatusMisdirectedRequest {
		t.Errorf("a foreign Host: %d", got)
	}
	if got := send("console.example.com", "https://evil.example").Code; got != http.StatusForbidden {
		t.Errorf("a foreign Origin: %d", got)
	}
	rec := send("console.example.com", "https://console.example.com")
	if rec.Code != http.StatusCreated {
		t.Fatalf("the public Host and Origin: %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Errorf("the session cookie is not Secure and HttpOnly: %v", cookies)
	}
}
