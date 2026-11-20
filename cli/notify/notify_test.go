package notify_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/cli/notify"
	"github.com/agents-control/connector-engine/console"
	"github.com/agents-control/connector-engine/surface"
)

type shown struct {
	title, body, link string
	ctx               context.Context
}

type shower struct{ got chan shown }

func (s shower) Show(ctx context.Context, title, body, link string) error {
	s.got <- shown{title, body, link, ctx}
	<-ctx.Done()
	return nil
}

func TestTheHelperShowsPromptsAndClosesThemWhenSettled(t *testing.T) {
	var mu sync.Mutex
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.URL.Path != "/api/notify/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		p := surface.Prompt{Approval: "a1", Connector: "Notes", Function: "create", FunctionTitle: "Create note", App: "claude-code", ExpiresAt: time.Now().Add(time.Hour)}
		for _, ev := range []console.HubEvent{{Type: "prompt", Approval: "a1", Prompt: &p, Link: "http://127.0.0.1:7465/approve/a1?t=x"}, {Type: "settled", Approval: "a1", Outcome: &surface.Outcome{State: surface.StateApproved}}} {
			raw, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, raw)
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	s := shower{got: make(chan shown, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go (&notify.Helper{Console: srv.URL, Key: "cnk_k", Show: s, Retry: 10 * time.Millisecond}).Run(ctx)
	got := <-s.got
	if got.title != "Allow Notes: Create note?" || !strings.Contains(got.body, "claude-code wants to run create") || got.link != "http://127.0.0.1:7465/approve/a1?t=x" {
		t.Fatalf("shown %+v", got)
	}
	select {
	case <-got.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the settled prompt's notification stayed open")
	}
	mu.Lock()
	defer mu.Unlock()
	if auth[0] != "Bearer cnk_k" {
		t.Fatalf("authorization %q", auth[0])
	}
}

func TestARefusedKeyStopsTheHelper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	err := (&notify.Helper{Console: srv.URL, Key: "old", Show: shower{}}).Run(context.Background())
	if err != notify.ErrUnpaired {
		t.Fatalf("err %v", err)
	}
}
