package remote_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/port"
)

type jump struct {
	*httptest.Server
	on     atomic.Bool
	same   bool
	moved  atomic.Int32
	landed atomic.Int32
}

func newJump(t *testing.T, up *upstream, same bool) *jump {
	t.Helper()
	j := &jump{same: same}
	landed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { j.landed.Add(1) }))
	j.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/again" {
			j.landed.Add(1)
		}
		if j.on.Load() {
			if r.Method == http.MethodPost {
				j.moved.Add(1)
			}
			to := landed.URL
			if j.same {
				to = j.URL + "/again"
			}
			http.Redirect(w, r, to, http.StatusTemporaryRedirect)
			return
		}
		up.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(j.Close)
	t.Cleanup(landed.Close)
	return j
}

func eachTarget(t *testing.T, run func(t *testing.T, same bool)) {
	for name, same := range map[string]bool{"same host": true, "other host": false} {
		t.Run(name, func(t *testing.T) { run(t, same) })
	}
}

func TestRedirectedCallIsAnErrorAndNotFollowed(t *testing.T) {
	eachTarget(t, redirectedCall)
}

func redirectedCall(t *testing.T, same bool) {
	up := newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true})
	j := newJump(t, up, same)
	rt, m := runtimeFor(j.URL)
	ctx := context.Background()
	h, err := rt.Start(ctx, port.Connector{}, m, port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx, h)

	j.on.Store(true)
	if _, err := rt.Call(ctx, h, "plain", nil, time.Now().Add(5*time.Second)); err == nil {
		t.Fatal("a redirected call must be an error")
	}
	if j.moved.Load() != 1 || j.landed.Load() != 0 {
		t.Fatalf("requests: redirected=%d followed=%d, want 1 and 0", j.moved.Load(), j.landed.Load())
	}
}

func TestRedirectedListIsAnErrorAndNotFollowed(t *testing.T) {
	eachTarget(t, redirectedList)
}

func redirectedList(t *testing.T, same bool) {
	up := newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true})
	j := newJump(t, up, same)
	rt, m := runtimeFor(j.URL)
	ctx := context.Background()
	h, err := rt.Start(ctx, port.Connector{}, m, port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx, h)

	j.on.Store(true)
	if _, _, err := rt.ListFunctions(ctx, h); err == nil {
		t.Fatal("a redirected list must be an error")
	}
	if j.moved.Load() != 1 || j.landed.Load() != 0 {
		t.Fatalf("requests: redirected=%d followed=%d, want 1 and 0", j.moved.Load(), j.landed.Load())
	}
}

func TestRedirectedStartIsAnErrorAndNotFollowed(t *testing.T) {
	up := newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true})
	j := newJump(t, up, true)
	j.on.Store(true)
	rt, m := runtimeFor(j.URL)
	if _, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("start: %v", err)
	}
	if j.landed.Load() != 0 {
		t.Fatalf("the redirect was followed %d times", j.landed.Load())
	}
}
