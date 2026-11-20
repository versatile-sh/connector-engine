package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/versatile-sh/connector-engine/surface"
)

const (
	appToken = "xapp-1-APPSECRET"
	botToken = "xoxb-BOTSECRET"
)

type request struct {
	path, auth string
	body       map[string]any
}

type fake struct {
	mu       sync.Mutex
	events   []string
	requests []request
	frames   chan any
	acks     chan string
	failWith string
	srv      *httptest.Server
}

func (f *fake) note(e string) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

func (f *fake) eventList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fake) find(path string) []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []request
	for _, r := range f.requests {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.requests = append(f.requests, request{r.URL.Path, r.Header.Get("Authorization"), body})
	f.mu.Unlock()
	if f.failWith != "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": f.failWith})
		return
	}
	switch r.URL.Path {
	case "/api/conversations.open":
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": map[string]string{"id": "D1"}})
	case "/api/chat.postMessage":
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "D1", "ts": "1700.01"})
	case "/api/chat.update":
		f.note("update")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	case "/api/apps.connections.open":
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/socket?ticket=TICKETSECRET"})
	case "/respond":
		f.note("respond")
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (f *fake) socket(ws *websocket.Conn) {
	_ = websocket.JSON.Send(ws, map[string]any{"type": "hello"})
	go func() {
		for fr := range f.frames {
			if websocket.JSON.Send(ws, fr) != nil {
				return
			}
		}
	}()
	for {
		var in map[string]any
		if websocket.JSON.Receive(ws, &in) != nil {
			return
		}
		id, _ := in["envelope_id"].(string)
		f.acks <- id
	}
}

func setup(t *testing.T) (*Channel, *fake) {
	t.Helper()
	f := &fake{frames: make(chan any, 4), acks: make(chan string, 8)}
	mux := http.NewServeMux()
	mux.HandleFunc("/", f.serve)
	mux.Handle("/socket", websocket.Handler(f.socket))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(func() { close(f.frames); f.srv.Close() })
	c, err := New(Config{AppToken: appToken, BotToken: botToken, OwnerUserID: "U1", APIBase: f.srv.URL + "/api"})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func prompt(choices ...surface.Choice) surface.Prompt {
	return surface.Prompt{Key: "k1", Approval: "ap-1", Connector: "Notes", Function: "create_note", FunctionTitle: "Create note",
		App: "claude-code", ExpiresAt: time.Unix(1800000000, 0), Choices: choices}
}

func actionsOf(body map[string]any) []any {
	for _, b := range body["blocks"].([]any) {
		if m := b.(map[string]any); m["type"] == "actions" {
			return m["elements"].([]any)
		}
	}
	return nil
}

func TestNew(t *testing.T) {
	for _, cfg := range []Config{{BotToken: botToken, OwnerUserID: "U1"}, {AppToken: appToken, OwnerUserID: "U1"}, {AppToken: appToken, BotToken: botToken}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded", cfg)
		}
	}
	c, err := New(Config{AppToken: appToken, BotToken: botToken, OwnerUserID: "U1"})
	if err != nil || c.base != defaultAPIBase || c.Kind() != "slack" {
		t.Fatalf("defaults: %v %+v", err, c)
	}
}

func TestNotifyRendersBlocksAndButtons(t *testing.T) {
	c, f := setup(t)
	p := prompt(surface.ChoiceOnce, surface.ChoiceAlways, surface.ChoiceDeny)
	ref, err := c.Notify(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Notify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if ref != "D1:1700.01" {
		t.Fatalf("ref = %q", ref)
	}
	if opens := f.find("/api/conversations.open"); len(opens) != 1 || opens[0].body["users"] != "U1" || opens[0].auth != "Bearer "+botToken {
		t.Fatalf("opens = %+v", opens)
	}
	body := f.find("/api/chat.postMessage")[0].body
	if body["channel"] != "D1" || !strings.Contains(body["text"].(string), "Allow Notes: Create note?") {
		t.Fatalf("post = %v", body)
	}
	els := actionsOf(body)
	if len(els) != 3 {
		t.Fatalf("buttons = %v", els)
	}
	var got []string
	for _, e := range els {
		m := e.(map[string]any)
		style, _ := m["style"].(string)
		got = append(got, m["action_id"].(string)+"="+m["value"].(string)+"/"+style)
	}
	if strings.Join(got, ",") != "o:k1=o:k1/primary,a:k1=a:k1/,d:k1=d:k1/danger" {
		t.Fatalf("buttons = %v", got)
	}
}

func TestNotifyWithoutChoicesHasNoActions(t *testing.T) {
	c, f := setup(t)
	if _, err := c.Notify(context.Background(), prompt()); err != nil {
		t.Fatal(err)
	}
	if actionsOf(f.find("/api/chat.postMessage")[0].body) != nil {
		t.Fatal("actions block sent without choices")
	}
}

func TestSlackOkFalseIsAnError(t *testing.T) {
	c, f := setup(t)
	f.failWith = "channel_not_found"
	if _, err := c.Notify(context.Background(), prompt()); err == nil || !strings.Contains(err.Error(), "channel_not_found") {
		t.Fatalf("Notify = %v", err)
	}
}

func TestSettleDropsActionsAndAddsSentence(t *testing.T) {
	c, f := setup(t)
	o := surface.Outcome{State: surface.StateExpired}
	if err := c.Settle(context.Background(), "D1:1700.01", prompt(surface.ChoiceOnce), o); err != nil {
		t.Fatal(err)
	}
	body := f.find("/api/chat.update")[0].body
	if body["channel"] != "D1" || body["ts"] != "1700.01" || actionsOf(body) != nil {
		t.Fatalf("update = %v", body)
	}
	raw, _ := json.Marshal(body["blocks"])
	if !strings.Contains(string(raw), surface.Sentence(o)) {
		t.Fatalf("update lacks sentence: %s", raw)
	}
}

func listen(t *testing.T, c *Channel, click surface.ClickFunc) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Listen(ctx, click) }()
	return cancel, done
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func interactive(f *fake, user, actionID string) map[string]any {
	return map[string]any{"envelope_id": "e1", "type": "interactive", "accepts_response_payload": false, "payload": map[string]any{
		"type": "block_actions", "user": map[string]any{"id": user}, "response_url": f.srv.URL + "/respond",
		"channel": map[string]any{"id": "D1"}, "container": map[string]any{"channel_id": "D1", "message_ts": "1700.01"},
		"message": map[string]any{"ts": "1700.01", "text": "Allow Notes: Create note?", "blocks": []any{
			map[string]any{"type": "header", "text": map[string]any{"type": "plain_text", "text": "Allow Notes: Create note?"}},
			map[string]any{"type": "actions", "block_id": "choices", "elements": []any{}},
		}},
		"actions": []any{map[string]any{"action_id": actionID, "value": actionID}},
	}}
}

func TestOwnerClickAcksThenDecidesThenUpdates(t *testing.T) {
	c, f := setup(t)
	var got surface.Click
	o := surface.Outcome{State: surface.StateSession, By: "slack:U1", Channel: "slack"}
	cancel, done := listen(t, c, func(_ context.Context, cl surface.Click) (surface.Outcome, error) {
		got = cl
		select {
		case id := <-f.acks:
			f.note("acked:" + id)
		case <-time.After(time.Second):
			f.note("not-acked")
		}
		f.note("click")
		return o, nil
	})
	f.frames <- interactive(f, "U1", "s:k1")
	waitFor(t, func() bool { return len(f.find("/api/chat.update")) == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Listen = %v", err)
	}
	if ev := strings.Join(f.eventList(), ","); ev != "acked:e1,click,update" {
		t.Fatalf("order = %s", ev)
	}
	if got != (surface.Click{Key: "k1", Choice: surface.ChoiceSession, User: "U1"}) {
		t.Fatalf("click = %+v", got)
	}
	body := f.find("/api/chat.update")[0].body
	raw, _ := json.Marshal(body["blocks"])
	if actionsOf(body) != nil || !strings.Contains(string(raw), surface.Sentence(o)) || !strings.Contains(string(raw), "Allow Notes") {
		t.Fatalf("update = %s", raw)
	}
}

func TestNonOwnerClickGetsEphemeralRefusal(t *testing.T) {
	c, f := setup(t)
	clicked := false
	cancel, done := listen(t, c, func(context.Context, surface.Click) (surface.Outcome, error) {
		clicked = true
		return surface.Outcome{}, nil
	})
	f.frames <- interactive(f, "U2", "o:k1")
	waitFor(t, func() bool { return len(f.find("/respond")) == 1 })
	cancel()
	<-done
	if clicked {
		t.Fatal("ClickFunc called for a non-owner")
	}
	b := f.find("/respond")[0].body
	if b["response_type"] != "ephemeral" || b["replace_original"] != false || b["text"] != surface.ErrNotOwner.Error() {
		t.Fatalf("refusal = %v", b)
	}
	if len(f.find("/api/chat.update")) != 0 {
		t.Fatal("message updated for a non-owner")
	}
}

func TestExpiredClickRespondsEphemeral(t *testing.T) {
	c, f := setup(t)
	cancel, done := listen(t, c, func(context.Context, surface.Click) (surface.Outcome, error) {
		return surface.Outcome{}, surface.ErrExpired
	})
	f.frames <- interactive(f, "U1", "o:gone")
	waitFor(t, func() bool { return len(f.find("/respond")) == 1 })
	cancel()
	<-done
	if b := f.find("/respond")[0].body; b["text"] != surface.ErrExpired.Error() {
		t.Fatalf("respond = %v", b)
	}
	if len(f.find("/api/chat.update")) != 0 {
		t.Fatal("message updated on an expired click")
	}
}

func TestDisconnectReturnsErrorAndCancelReturnsNil(t *testing.T) {
	c, f := setup(t)
	_, done := listen(t, c, nil)
	f.frames <- map[string]any{"envelope_id": "", "type": "disconnect", "reason": "refresh_requested"}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Listen returned nil on disconnect")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Listen did not return on disconnect")
	}
	if a := f.find("/api/apps.connections.open"); len(a) == 0 || a[0].auth != "Bearer "+appToken {
		t.Fatalf("connections.open = %+v", a)
	}

	cancel, done := listen(t, c, nil)
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Listen = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Listen did not return on cancel")
	}
}

func TestErrorsNeverCarryTokens(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c, err := New(Config{AppToken: appToken, BotToken: botToken, OwnerUserID: "U1", APIBase: srv.URL + "/" + botToken})
	if err != nil {
		t.Fatal(err)
	}
	_, nerr := c.Notify(context.Background(), prompt(surface.ChoiceOnce))
	serr := c.Settle(context.Background(), "D1:1", prompt(), surface.Outcome{State: surface.StateDenied})
	lerr := c.Listen(context.Background(), nil)
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": "ws" + strings.TrimPrefix(srv.URL, "http") + "/socket?ticket=TICKETSECRET"})
	}))
	defer ws.Close()
	c2, _ := New(Config{AppToken: appToken, BotToken: botToken, OwnerUserID: "U1", APIBase: ws.URL})
	derr := c2.Listen(context.Background(), nil)
	for _, e := range []error{nerr, serr, lerr, derr} {
		if e == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(e.Error(), "SECRET") {
			t.Fatalf("secret leaked: %v", e)
		}
	}
}
