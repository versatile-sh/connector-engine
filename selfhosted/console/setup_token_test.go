package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

func TestAConfiguredSetupTokenGatesTheFirstOwnerAndTheFirstRunRestore(t *testing.T) {
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: &console.Engine{}, State: state, Sessions: console.NewSessions(state), Secrets: &sealed.Box{}, SetupToken: "s3cret-token"}
	srv := httptest.NewUnstartedServer(nil)
	c.Guard = console.Guard{Hosts: []string{srv.Listener.Addr().String()}}
	srv.Config.Handler = c.Handler()
	srv.Start()
	t.Cleanup(srv.Close)
	post := func(path, token string) int {
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(`{"username":"me","password":"correct horse battery"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		if token != "" {
			req.Header.Set("X-Setup-Token", token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if got := post("/api/setup", ""); got != http.StatusForbidden {
		t.Errorf("setup without the token: %d", got)
	}
	if got := post("/api/setup", "wrong"); got != http.StatusForbidden {
		t.Errorf("setup with a wrong token: %d", got)
	}
	if got := post("/api/backup/inspect", "wrong"); got != http.StatusForbidden {
		t.Errorf("restore with a wrong token: %d", got)
	}
	if got := post("/api/setup", "s3cret-token"); got != http.StatusCreated {
		t.Errorf("setup with the token: %d", got)
	}
}
