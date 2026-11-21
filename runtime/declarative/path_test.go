package declarative_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func pathServer(t *testing.T) (port.Manifest, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.RequestURI)
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	m := port.Manifest{
		Kind:   port.KindRESTDeclarative,
		Egress: &port.Egress{AllowHosts: []string{u.Host}},
		REST: &port.REST{BaseURL: srv.URL + "/v1/", Functions: []port.RESTFunction{
			{Name: "get", Method: "GET", Path: "/items/{id}/notes"},
		}},
	}
	return m, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func refusesPathValues(t *testing.T, values ...string) {
	t.Helper()
	m, seen := pathServer(t)
	rt, h := start(t, m)
	defer rt.Stop(context.Background(), h)
	for _, v := range values {
		args, _ := json.Marshal(map[string]string{"id": v})
		_, err := rt.Call(context.Background(), h, "get", args, time.Now().Add(5*time.Second))
		if port.CodeOf(err) != port.CodeInvalidArgument || !strings.Contains(err.Error(), "path parameter id") {
			t.Errorf("%q: want invalid_argument naming the parameter, got %v", v, err)
		}
	}
	if got := seen(); len(got) != 0 {
		t.Fatalf("upstream requests = %v, want none", got)
	}
}

func TestPathParameterRefusesDotSegments(t *testing.T) {
	refusesPathValues(t, ".", "..", "...")
}

func TestPathParameterRefusesSeparators(t *testing.T) {
	refusesPathValues(t, "a/b", "../x", `a\b`, "%2F", "a%2e%2e", "%", "a?b", "a#b", "a;b")
}

func TestPathParameterRefusesNonASCII(t *testing.T) {
	refusesPathValues(t, "．．", "a／b", "é", "a\x00b")
}

func TestPathParameterRefusesEmptyAndSpace(t *testing.T) {
	refusesPathValues(t, "", " ", "a b", "a\tb", "a\nb")
}

func TestPathParameterSendsOneSegment(t *testing.T) {
	m, seen := pathServer(t)
	rt, h := start(t, m)
	defer rt.Stop(context.Background(), h)
	for _, v := range []string{"abc", "A-b_c~9", "v1.2", "a..b", ".hidden", "a@b.test", "2026-01-01T00:00:00Z", "+5511999", "user:42"} {
		args, _ := json.Marshal(map[string]string{"id": v})
		if res := call(t, rt, h, "get", string(args)); res.IsError {
			t.Fatalf("%q: %s", v, res.Content)
		}
		got := seen()
		if want := "/v1/items/" + url.PathEscape(v) + "/notes"; got[len(got)-1] != want {
			t.Fatalf("request = %q, want %q", got[len(got)-1], want)
		}
	}
}

func TestNumberPathParameterStaysTokenText(t *testing.T) {
	m, seen := pathServer(t)
	rt, h := start(t, m)
	defer rt.Stop(context.Background(), h)
	for _, v := range []string{"1e5", "-3", "42", "1.50", "true"} {
		if res := call(t, rt, h, "get", `{"id":`+v+`}`); res.IsError {
			t.Fatalf("%s: %s", v, res.Content)
		}
		got := seen()
		if want := "/v1/items/" + v + "/notes"; got[len(got)-1] != want {
			t.Fatalf("request = %q, want %q", got[len(got)-1], want)
		}
	}
}
