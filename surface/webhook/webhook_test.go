package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/surface"
)

type hit struct {
	method, path, query string
	header              http.Header
	body                string
}

type fake struct {
	mu   sync.Mutex
	hits []hit
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.hits = append(f.hits, hit{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(b)})
	f.mu.Unlock()
	if r.URL.Query().Get("wait") == "true" {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "m9"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *fake) all() []hit {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hit(nil), f.hits...)
}

func setup(t *testing.T, kind, path, tok string) (*Channel, *fake) {
	t.Helper()
	f := &fake{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(Config{Kind: kind, URL: srv.URL + path, Token: tok})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func prompt() surface.Prompt {
	return surface.Prompt{Key: "k1", Approval: "ap-1", Connector: "Notes", Function: "create_note", FunctionTitle: "Create note", App: "claude-code",
		ExpiresAt: time.Unix(1800000000, 0), Choices: []surface.Choice{surface.ChoiceOnce, surface.ChoiceDeny}, ConsoleURL: "http://127.0.0.1:7465/approvals/ap-1"}
}

func TestNew(t *testing.T) {
	for _, cfg := range []Config{{Kind: "email", URL: "https://x"}, {Kind: KindNtfy, URL: "ftp://x"}, {Kind: KindNtfy}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded", cfg)
		}
	}
}

func TestNtfyNotify(t *testing.T) {
	c, f := setup(t, KindNtfy, "/approvals", "tk_SECRET")
	ref, err := c.Notify(context.Background(), prompt())
	if err != nil || ref != "" {
		t.Fatalf("Notify = %q, %v", ref, err)
	}
	h := f.all()[0]
	want := map[string]string{"Title": "Allow Notes: Create note?", "Tags": "raised_hand", "Click": prompt().ConsoleURL,
		"Actions": "view, Open in console, " + prompt().ConsoleURL, "Authorization": "Bearer tk_SECRET"}
	for k, v := range want {
		if got := h.header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if h.path != "/approvals" || !strings.Contains(h.body, answerHint) || !strings.Contains(h.body, "claude-code") {
		t.Fatalf("hit = %+v", h)
	}
	if err := c.Settle(context.Background(), ref, prompt(), surface.Outcome{State: surface.StateDenied}); err != nil || len(f.all()) != 1 {
		t.Fatalf("Settle posted or failed: %v", err)
	}
}

func TestNtfyWithoutTokenSendsNoAuthorization(t *testing.T) {
	c, f := setup(t, KindNtfy, "/t", "")
	if _, err := c.Notify(context.Background(), prompt()); err != nil {
		t.Fatal(err)
	}
	if a := f.all()[0].header.Get("Authorization"); a != "" {
		t.Fatalf("Authorization = %q", a)
	}
}

func TestDiscordWebhookNotifyAndSettle(t *testing.T) {
	c, f := setup(t, KindDiscordWebhook, "/api/webhooks/1/HOOKSECRET", "")
	ref, err := c.Notify(context.Background(), prompt())
	if err != nil || ref != "m9" {
		t.Fatalf("Notify = %q, %v", ref, err)
	}
	post := f.all()[0]
	if post.query != "wait=true" || !strings.Contains(post.body, "Open in console") || !strings.Contains(post.body, answerHint) || strings.Contains(post.body, "components") {
		t.Fatalf("post = %+v", post)
	}
	o := surface.Outcome{State: surface.StateApproved, By: "console", Channel: "console"}
	if err := c.Settle(context.Background(), ref, prompt(), o); err != nil {
		t.Fatal(err)
	}
	patch := f.all()[1]
	if patch.method != http.MethodPatch || patch.path != "/api/webhooks/1/HOOKSECRET/messages/m9" || !strings.Contains(patch.body, surface.Sentence(o)) {
		t.Fatalf("patch = %+v", patch)
	}
}

func TestSlackWebhookNotify(t *testing.T) {
	c, f := setup(t, KindSlackWebhook, "/services/T/B/HOOKSECRET", "")
	ref, err := c.Notify(context.Background(), prompt())
	if err != nil || ref != "" {
		t.Fatalf("Notify = %q, %v", ref, err)
	}
	var msg struct {
		Text   string           `json:"text"`
		Blocks []map[string]any `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(f.all()[0].body), &msg); err != nil {
		t.Fatal(err)
	}
	last := msg.Blocks[len(msg.Blocks)-1]
	btn := last["elements"].([]any)[0].(map[string]any)
	if last["type"] != "actions" || btn["url"] != prompt().ConsoleURL || !strings.Contains(msg.Text, answerHint) {
		t.Fatalf("msg = %+v", msg)
	}
	if err := c.Settle(context.Background(), "", prompt(), surface.Outcome{State: surface.StateDenied}); err != nil || len(f.all()) != 1 {
		t.Fatalf("Settle posted or failed: %v", err)
	}
}

func TestListenReturnsAtOnce(t *testing.T) {
	c, _ := setup(t, KindNtfy, "/t", "")
	if err := c.Listen(context.Background(), nil); err != nil || c.Kind() != KindNtfy {
		t.Fatalf("Listen = %v kind %s", err, c.Kind())
	}
}

func TestErrorsNeverCarryURLOrToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	for _, kind := range []string{KindNtfy, KindDiscordWebhook, KindSlackWebhook} {
		c, err := New(Config{Kind: kind, URL: srv.URL + "/hook/HOOKSECRET?token=QUERYSECRET", Token: "tk_SECRET"})
		if err != nil {
			t.Fatal(err)
		}
		_, nerr := c.Notify(context.Background(), prompt())
		serr := c.Settle(context.Background(), "m9", prompt(), surface.Outcome{State: surface.StateDenied})
		if nerr == nil {
			t.Fatalf("%s: expected an error", kind)
		}
		for _, e := range []error{nerr, serr} {
			if e != nil && strings.Contains(e.Error(), "SECRET") {
				t.Fatalf("%s: secret leaked: %v", kind, e)
			}
		}
	}
}
