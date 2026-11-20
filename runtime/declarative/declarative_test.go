package declarative_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/declarative"
)

type seen struct {
	Method, Path, RawQuery, Auth, Type, Body string
}

type rest struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []seen
}

func (s *rest) last() seen {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reqs[len(s.reqs)-1]
}

func newREST(t *testing.T) *rest {
	s := &rest{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.reqs = append(s.reqs, seen{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)})
		s.mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/items/") && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/v1/items/"), "ok": true})
		case r.URL.Path == "/v1/items" && r.Method == "GET":
			w.Write([]byte(`[1,2,3]`))
		case r.URL.Path == "/v1/items" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"created":true}`))
		case r.URL.Path == "/v1/plain":
			w.Write([]byte("not json"))
		case r.URL.Path == "/v1/jump":
			http.Redirect(w, r, "http://other.invalid/x", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func manifestFor(base string, allow ...string) port.Manifest {
	return port.Manifest{
		Kind:   port.KindRESTDeclarative,
		Egress: &port.Egress{AllowHosts: allow},
		REST: &port.REST{BaseURL: base + "/v1/", Functions: []port.RESTFunction{
			{Name: "get_item", Method: "GET", Path: "/items/{id}", Description: "Reads one item.", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"expand":{"type":"boolean"}},"required":["id"]}`)},
			{Name: "list_items", Method: "GET", Path: "/items"},
			{Name: "create_item", Method: "POST", Path: "/items", InputSchema: json.RawMessage(`{"type":"object","properties":{"dry_run":{"type":"boolean","x-in":"query"},"name":{"type":"string"}}}`)},
			{Name: "plain", Method: "GET", Path: "/plain"},
			{Name: "missing", Method: "DELETE", Path: "/gone/{id}"},
			{Name: "jump", Method: "GET", Path: "/jump"},
		}},
	}
}

func start(t *testing.T, m port.Manifest) (*declarative.Runtime, port.Handle) {
	t.Helper()
	rt := declarative.New(credential.Applier{
		Resolve: func(context.Context, port.SecretsRef) (port.CredentialKind, []byte, error) {
			return port.CredentialAPIKey, []byte("k"), nil
		},
		Injector: credential.Injector{},
	}, nil, egress.Guard{AllowPrivate: true})
	h, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{Credential: "c"})
	if err != nil {
		t.Fatal(err)
	}
	return rt, h
}

func call(t *testing.T, rt *declarative.Runtime, h port.Handle, fn, args string) port.Result {
	t.Helper()
	res, err := rt.Call(context.Background(), h, fn, json.RawMessage(args), time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("%s: %v", fn, err)
	}
	return res
}

func TestExecutesDeclaredFunctions(t *testing.T) {
	srv := newREST(t)
	u, _ := url.Parse(srv.URL)
	rt, h := start(t, manifestFor(srv.URL, u.Host))

	defs, _, err := rt.ListFunctions(context.Background(), h)
	if err != nil || len(defs) != 6 || defs[0].Name != "get_item" || !strings.Contains(string(defs[0].Raw), `"description":"Reads one item."`) || !strings.Contains(string(defs[0].Raw), `"http_method":"GET"`) {
		t.Fatalf("definitions: %v %s", err, defs)
	}

	res := call(t, rt, h, "get_item", `{"id":"a b/c","expand":true}`)
	if got := srv.last(); got.Path != "/v1/items/a%20b%2Fc" || got.RawQuery != "expand=true" || got.Auth != "Bearer k" {
		t.Fatalf("request %+v", got)
	}
	if res.IsError || !strings.Contains(string(res.Structured), `"ok":true`) {
		t.Fatalf("result %+v", res)
	}

	if res = call(t, rt, h, "list_items", ``); string(res.Structured) != `{"result":[1,2,3]}` {
		t.Fatalf("array response: %s", res.Structured)
	}

	res = call(t, rt, h, "create_item", `{"name":"n","dry_run":false}`)
	if got := srv.last(); got.Method != "POST" || got.RawQuery != "dry_run=false" || got.Type != "application/json" || got.Body != `{"name":"n"}` {
		t.Fatalf("request %+v", got)
	}
	if res.IsError || string(res.Structured) != `{"created":true}` {
		t.Fatalf("result %+v", res)
	}

	if res = call(t, rt, h, "plain", `{}`); res.Structured != nil || !strings.Contains(string(res.Content), "not json") {
		t.Fatalf("non-JSON response: %+v", res)
	}
	if res = call(t, rt, h, "missing", `{"id":"1"}`); !res.IsError {
		t.Fatal("a 404 is an error result")
	}
}

func TestRefusals(t *testing.T) {
	srv := newREST(t)
	u, _ := url.Parse(srv.URL)
	rt, h := start(t, manifestFor(srv.URL, u.Host))

	if _, err := rt.Call(context.Background(), h, "get_item", json.RawMessage(`{}`), time.Time{}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("missing path parameter: %v", err)
	}
	if _, err := rt.Call(context.Background(), h, "get_item", json.RawMessage(`{"id":{"a":1}}`), time.Time{}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("object path parameter: %v", err)
	}
	if _, err := rt.Call(context.Background(), h, "nope", nil, time.Time{}); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("unknown function: %v", err)
	}
	if _, err := rt.Call(context.Background(), h, "jump", nil, time.Time{}); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("a redirect off the allow-list must fail: %v", err)
	}

	other := declarative.New(credential.Applier{}, nil, egress.Guard{AllowPrivate: true})
	if _, err := other.Start(context.Background(), port.Connector{}, manifestFor(srv.URL, "elsewhere.example"), port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("base url outside egress: %v", err)
	}
	noEgress := manifestFor(srv.URL)
	noEgress.Egress = nil
	if _, err := other.Start(context.Background(), port.Connector{}, noEgress, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("no egress: %v", err)
	}
}
