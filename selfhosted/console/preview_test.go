package console_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func previewConsole(t *testing.T, c *console.Console) *browser {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	c.State, c.Sessions, c.Guard, c.Engine = state, console.NewSessions(state), console.Guard{Hosts: []string{lis.Addr().String()}}, &console.Engine{PublicURL: "http://127.0.0.1:1"}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: "http://" + lis.Addr().String(), client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	return b
}

func TestALocalServerAtLocalhostIsReachedThroughTheLoopbackHostAndKeepsItsURL(t *testing.T) {
	up := newUpstream(t)
	_, port, _ := net.SplitHostPort(up.Listener.Addr().String())
	typed := "http://[::1]:" + port + "/mcp"
	mapped := previewConsole(t, &console.Console{LoopbackHost: "127.0.0.1"})
	var pv console.Preview
	mapped.ok("POST", "/api/preview", map[string]any{"mcp": typed, "local": true}, &pv)
	if pv.URL != typed || pv.Host != "[::1]:"+port || !pv.HostLoopback || len(pv.Tools) != 4 {
		t.Fatalf("preview through the loopback host: url %q host %q host_loopback %v tools %d", pv.URL, pv.Host, pv.HostLoopback, len(pv.Tools))
	}
	plain := previewConsole(t, &console.Console{})
	var direct console.Preview
	plain.ok("POST", "/api/preview", map[string]any{"mcp": up.URL + "/mcp", "local": true}, &direct)
	if direct.HostLoopback || len(direct.Tools) != 4 {
		t.Fatal("a console without a loopback host says it reaches the computer's localhost")
	}
}

func TestAListingWhoseServerIsOnThisComputerIsRefused(t *testing.T) {
	raw, err := os.ReadFile("../../source/files/testdata/notes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{"http://localhost:7998/mcp", "https://HOST.docker.internal/mcp"} {
		listing := bytes.Replace(raw, []byte("https://mcp.example.com/mcp"), []byte(remote), 1)
		if bytes.Equal(listing, raw) {
			t.Fatal("the fixture's remote URL moved")
		}
		b := previewConsole(t, &console.Console{LoopbackHost: "host.docker.internal", Fetch: func(context.Context, string, int64) ([]byte, error) { return listing, nil }})
		if status, body := b.send("POST", "/api/preview", map[string]any{"listing": "https://listings.example/notes.yaml"}, nil); status != http.StatusBadRequest {
			t.Fatalf("a listing with its server at %s: %d %s", remote, status, body)
		}
	}
}
