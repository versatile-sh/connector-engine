package declarative_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/declarative"
)

func redirectManifest(base, host string) port.Manifest {
	return port.Manifest{
		Kind:   port.KindRESTDeclarative,
		Egress: &port.Egress{AllowHosts: []string{host}},
		REST: &port.REST{BaseURL: base + "/v1/", Functions: []port.RESTFunction{
			{Name: "read", Method: "GET", Path: "/moved"},
			{Name: "write", Method: "POST", Path: "/moved", InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)},
		}},
	}
}

func TestRedirectIsTheAnswer(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, fn := range []string{"read", "write"} {
			t.Run(http.StatusText(status)+" "+fn, func(t *testing.T) {
				var hits atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					http.Redirect(w, r, "/v1/elsewhere", status)
				}))
				defer srv.Close()
				u, _ := url.Parse(srv.URL)
				rt, h := start(t, redirectManifest(srv.URL, u.Host))

				res := call(t, rt, h, fn, `{"name":"a"}`)
				if !res.IsError {
					t.Fatalf("a %d answer is a tool error: %s", status, res.Content)
				}
				if n := hits.Load(); n != 1 {
					t.Fatalf("upstream requests = %d, want 1", n)
				}
			})
		}
	}
}

func TestRedirectToAnotherHostIsNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	var first atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.Add(1)
		http.Redirect(w, r, other.URL+"/x", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	rt := declarative.New(credential.Applier{}, nil, egress.Guard{AllowPrivate: true})
	h, err := rt.Start(context.Background(), port.Connector{}, redirectManifest(srv.URL, u.Host), port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := rt.Call(context.Background(), h, "write", json.RawMessage(`{"name":"a"}`), time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("the upstream answered, so the call has a result: %v", err)
	}
	if !res.IsError {
		t.Fatalf("result: %+v", res)
	}
	if first.Load() != 1 || elsewhere.Load() != 0 {
		t.Fatalf("requests: first=%d other=%d, want 1 and 0", first.Load(), elsewhere.Load())
	}
}
