package egress_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/internal/egress"
)

func TestForbidden(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "10.1.2.3": true, "172.16.0.1": true, "192.168.1.1": true, "169.254.169.254": true,
		"fd00:ec2::254": true, "100.64.0.1": true, "0.0.0.0": true, "::ffff:127.0.0.1": true, "8.8.8.8": false, "2606:4700::1111": false,
	} {
		if got := egress.Forbidden(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Forbidden(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestGuardRefusesLoopbackUnlessAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()
	host, _, _ := net.SplitHostPort(srv.Listener.Addr().String())
	cases := []struct {
		name  string
		guard egress.Guard
		extra []string
		ok    bool
	}{
		{"refused by default", egress.Guard{}, nil, false},
		{"allowed by host list", egress.Guard{Hosts: []string{host}}, nil, true},
		{"manifest-allowed private host refused", egress.Guard{}, []string{host}, false},
		{"operator-listed host passes", egress.Guard{Hosts: []string{host}}, []string{host}, true},
		{"host outside the narrowed list refused", egress.Guard{AllowPrivate: true}, []string{"example.com"}, false},
		{"allowed by flag", egress.Guard{AllowPrivate: true}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := c.guard.Client(c.extra...).Get(srv.URL)
			if c.ok != (err == nil) {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			if err == nil {
				res.Body.Close()
				return
			}
			if !strings.Contains(err.Error(), "refused") {
				t.Fatalf("error does not say refused: %v", err)
			}
		})
	}
}

func TestTheGuardedTransportIgnoresAnEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:3128")
	if (egress.Guard{}).Transport().Proxy != nil {
		t.Fatal("an environment proxy would carry every connection past the guard")
	}
}

func TestALoopbackHostIsDialledAsTheLoopbackHostAndKeepsItsHostHeader(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	typed := "http://[::1]:" + port + "/mcp"
	if res, err := (egress.Guard{AllowPrivate: true}).Client().Get(typed); err == nil {
		res.Body.Close()
		t.Skip("something answers on [::1] at the test server's port")
	}
	res, err := egress.Guard{AllowPrivate: true, LoopbackHost: "127.0.0.1"}.Client().Get(typed)
	if err != nil {
		t.Fatalf("the mapped dial failed: %v", err)
	}
	res.Body.Close()
	if gotHost != "[::1]:"+port {
		t.Fatalf("Host = %q, want the URL as typed", gotHost)
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "LOCALHOST": true, "127.0.0.1": true, "127.1.2.3": true, "::1": true, "[::1]": true,
		"host.docker.internal": false, "10.0.0.1": false, "example.com": false, "": false} {
		if got := egress.IsLoopback(host); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}
