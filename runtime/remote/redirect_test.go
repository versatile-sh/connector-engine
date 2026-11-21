package remote_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/remote"
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

func started(t *testing.T, same bool) (*jump, *remote.Runtime, port.Handle) {
	t.Helper()
	j := newJump(t, newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true}), same)
	rt, m := runtimeFor(j.URL)
	h, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rt.Stop(context.Background(), h) })
	return j, rt, h
}

func TestRedirectedCallKeepsAnHonestOutcomeAndLaterCallsDoNotRun(t *testing.T) {
	j, rt, h := started(t, true)
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)

	j.on.Store(true)
	_, err := rt.Call(ctx, h, "plain", nil, deadline)
	if !port.IsCode(err, port.CodeOutcomeUnknown) || !strings.Contains(err.Error(), "307") || !strings.Contains(err.Error(), j.Listener.Addr().String()) {
		t.Fatalf("the redirected call was delivered and names the status and host: %v", err)
	}
	if _, err := rt.Call(ctx, h, "plain", nil, deadline); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("a later call sends nothing and did not run: %v", err)
	}
	if _, _, err := rt.ListFunctions(ctx, h); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("a later list: %v", err)
	}
	if j.moved.Load() != 1 || j.landed.Load() != 0 {
		t.Fatalf("requests: redirected=%d followed=%d, want 1 and 0", j.moved.Load(), j.landed.Load())
	}
}

func TestRedirectedListIsNotFollowed(t *testing.T) {
	j, rt, h := started(t, true)
	j.on.Store(true)
	if _, _, err := rt.ListFunctions(context.Background(), h); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("list: %v", err)
	}
	if j.moved.Load() != 1 || j.landed.Load() != 0 {
		t.Fatalf("requests: redirected=%d followed=%d, want 1 and 0", j.moved.Load(), j.landed.Load())
	}
}

func TestRedirectToAnotherHostIsNeverReachedPinsAllowList(t *testing.T) {
	j, rt, h := started(t, false)
	j.on.Store(true)
	if _, err := rt.Call(context.Background(), h, "plain", nil, time.Now().Add(5*time.Second)); err == nil {
		t.Fatal("a redirected call must be an error")
	}
	if j.moved.Load() != 1 || j.landed.Load() != 0 {
		t.Fatalf("requests: redirected=%d reached=%d, want 1 and 0", j.moved.Load(), j.landed.Load())
	}
}

func TestRedirectedStartNamesTheStatusAndHostAndIsNotFollowed(t *testing.T) {
	j := newJump(t, newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true}), true)
	j.on.Store(true)
	rt, m := runtimeFor(j.URL)
	_, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{})
	if !port.IsCode(err, port.CodeUnavailable) || !strings.Contains(err.Error(), "307") || !strings.Contains(err.Error(), j.Listener.Addr().String()) {
		t.Fatalf("start: %v", err)
	}
	if j.landed.Load() != 0 {
		t.Fatalf("the redirect was followed %d times", j.landed.Load())
	}
}
