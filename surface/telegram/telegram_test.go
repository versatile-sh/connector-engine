package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/surface"
)

const token = "123456:SECRET-bot-token"

type call struct {
	method string
	body   map[string]any
}

type fake struct {
	mu      sync.Mutex
	calls   []call
	updates chan []map[string]any
	status  int
}

func (f *fake) log(c call) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}

func (f *fake) byMethod(m string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.method == m {
			out = append(out, c)
		}
	}
	return out
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, `{"ok":false,"error_code":404,"description":"Not Found"}`, 404)
		return
	}
	method := strings.TrimPrefix(r.URL.Path, prefix)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
		return
	}
	if method != "getUpdates" {
		f.log(call{method, body})
	}
	var result any = true
	switch method {
	case "sendMessage":
		result = map[string]any{"message_id": 77, "chat": map[string]any{"id": 42}, "text": body["text"]}
	case "getUpdates":
		select {
		case ups := <-f.updates:
			result = ups
		case <-time.After(50 * time.Millisecond):
			result = []any{}
		case <-r.Context().Done():
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func setup(t *testing.T) (*Channel, *fake) {
	t.Helper()
	f := &fake{updates: make(chan []map[string]any, 4)}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(Config{Token: token, OwnerUserID: "42", APIBase: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	c.poll = 0
	return c, f
}

func prompt(choices ...surface.Choice) surface.Prompt {
	return surface.Prompt{Key: "abc123", Approval: "ap-1", Connector: "Notes", Function: "create_note", FunctionTitle: "Create note",
		App: "claude-code", ExpiresAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), Choices: choices}
}

func keyboardOf(t *testing.T, body map[string]any) [][]any {
	t.Helper()
	rm, ok := body["reply_markup"].(map[string]any)
	if !ok {
		return nil
	}
	var rows [][]any
	for _, r := range rm["inline_keyboard"].([]any) {
		rows = append(rows, r.([]any))
	}
	return rows
}

func TestNew(t *testing.T) {
	for _, cfg := range []Config{{OwnerUserID: "1"}, {Token: "t"}, {Token: "t", OwnerUserID: "abc"}, {Token: "t", OwnerUserID: "1", APIBase: "ftp://x"}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded", cfg)
		}
	}
	c, err := New(Config{Token: "t", OwnerUserID: "1"})
	if err != nil || c.base != defaultAPIBase || c.Kind() != "telegram" {
		t.Fatalf("defaults: %v %+v", err, c)
	}
}

func TestNotifyRendersButtonsAndNoArguments(t *testing.T) {
	c, f := setup(t)
	ref, err := c.Notify(context.Background(), prompt(surface.ChoiceOnce, surface.ChoiceSession, surface.ChoiceDeny))
	if err != nil {
		t.Fatal(err)
	}
	if ref != "42:77" {
		t.Fatalf("ref = %q", ref)
	}
	sent := f.byMethod("sendMessage")[0].body
	if sent["chat_id"].(float64) != 42 {
		t.Fatalf("chat_id = %v", sent["chat_id"])
	}
	text := sent["text"].(string)
	for _, want := range []string{"Notes", "Create note (create_note)", "claude-code", "2026-11-20 10:00 UTC"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
	if strings.Contains(text, "argument") {
		t.Errorf("text mentions arguments: %q", text)
	}
	rows := keyboardOf(t, sent)
	if len(rows) != 1 || len(rows[0]) != 3 {
		t.Fatalf("keyboard = %v", rows)
	}
	b := rows[0][2].(map[string]any)
	if b["text"] != "Deny" || b["callback_data"] != "d:abc123" {
		t.Fatalf("button = %v", b)
	}
}

func TestNotifyWithoutChoicesHasNoKeyboard(t *testing.T) {
	c, f := setup(t)
	if _, err := c.Notify(context.Background(), prompt()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.byMethod("sendMessage")[0].body["reply_markup"]; ok {
		t.Fatal("keyboard sent for a prompt without choices")
	}
}

func TestSettleEditsToSentenceAndClearsKeyboard(t *testing.T) {
	c, f := setup(t)
	o := surface.Outcome{State: surface.StateDenied, By: "telegram:42", Channel: "telegram"}
	if err := c.Settle(context.Background(), "42:77", prompt(surface.ChoiceOnce), o); err != nil {
		t.Fatal(err)
	}
	e := f.byMethod("editMessageText")[0].body
	if e["chat_id"] != "42" || e["message_id"] != "77" || !strings.HasSuffix(e["text"].(string), "\n\n"+surface.Sentence(o)) {
		t.Fatalf("edit = %v", e)
	}
	if rows := keyboardOf(t, e); len(rows) != 0 {
		t.Fatalf("keyboard kept: %v", rows)
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

func press(from int64, data string) []map[string]any {
	return []map[string]any{{"update_id": 9, "callback_query": map[string]any{
		"id": "cq1", "from": map[string]any{"id": from}, "data": data,
		"message": map[string]any{"message_id": 77, "chat": map[string]any{"id": 42}, "text": "Approval needed"},
	}}}
}

func TestOwnerClickDecidesAnswersAndEdits(t *testing.T) {
	c, f := setup(t)
	var got []surface.Click
	var mu sync.Mutex
	o := surface.Outcome{State: surface.StateApproved, By: "telegram:42", Channel: "telegram"}
	cancel, done := listen(t, c, func(_ context.Context, cl surface.Click) (surface.Outcome, error) {
		mu.Lock()
		got = append(got, cl)
		mu.Unlock()
		return o, nil
	})
	f.updates <- press(42, "o:abc123")
	waitFor(t, func() bool { return len(f.byMethod("editMessageText")) == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Listen = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != (surface.Click{Key: "abc123", Choice: surface.ChoiceOnce, User: "42"}) {
		t.Fatalf("clicks = %+v", got)
	}
	if a := f.byMethod("answerCallbackQuery"); len(a) != 1 || a[0].body["text"] != surface.Sentence(o) {
		t.Fatalf("answers = %+v", a)
	}
	if e := f.byMethod("editMessageText")[0].body; e["text"] != "Approval needed\n\n"+surface.Sentence(o) {
		t.Fatalf("edit = %v", e)
	}
	if c.offset != 10 {
		t.Fatalf("offset = %d", c.offset)
	}
}

func TestNonOwnerClickIsRefused(t *testing.T) {
	c, f := setup(t)
	clicked := false
	cancel, done := listen(t, c, func(context.Context, surface.Click) (surface.Outcome, error) {
		clicked = true
		return surface.Outcome{}, nil
	})
	f.updates <- press(99, "o:abc123")
	waitFor(t, func() bool { return len(f.byMethod("answerCallbackQuery")) == 1 })
	cancel()
	<-done
	if clicked {
		t.Fatal("ClickFunc called for a non-owner")
	}
	if a := f.byMethod("answerCallbackQuery")[0].body["text"]; a != surface.ErrNotOwner.Error() {
		t.Fatalf("answer = %v", a)
	}
	if len(f.byMethod("editMessageText")) != 0 {
		t.Fatal("message edited for a non-owner")
	}
}

func TestExpiredClickShowsExpired(t *testing.T) {
	c, f := setup(t)
	cancel, done := listen(t, c, func(context.Context, surface.Click) (surface.Outcome, error) {
		return surface.Outcome{}, surface.ErrExpired
	})
	f.updates <- press(42, "a:gone")
	waitFor(t, func() bool { return len(f.byMethod("answerCallbackQuery")) == 1 })
	cancel()
	<-done
	if a := f.byMethod("answerCallbackQuery")[0].body["text"]; a != surface.ErrExpired.Error() {
		t.Fatalf("answer = %v", a)
	}
	if len(f.byMethod("editMessageText")) != 0 {
		t.Fatal("message edited on an expired click")
	}
}

func TestListenReturnsNilOnCancel(t *testing.T) {
	c, _ := setup(t)
	cancel, done := listen(t, c, nil)
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Listen = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Listen did not return")
	}
}

func TestListenReturnsErrorOnUnauthorized(t *testing.T) {
	c, f := setup(t)
	f.status = http.StatusUnauthorized
	err := c.Listen(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), token) {
		t.Fatalf("Listen = %v", err)
	}
}

func TestTransportErrorsNeverCarryToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c, err := New(Config{Token: token, OwnerUserID: "42", APIBase: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, nerr := c.Notify(context.Background(), prompt(surface.ChoiceOnce))
	serr := c.Settle(context.Background(), "42:77", prompt(), surface.Outcome{State: surface.StateDenied})
	lerr := c.Listen(context.Background(), nil)
	for _, e := range []error{nerr, serr, lerr} {
		if e == nil {
			t.Fatal("expected an error from a closed server")
		}
		if strings.Contains(e.Error(), token) || strings.Contains(e.Error(), "SECRET") {
			t.Fatalf("token leaked: %v", e)
		}
	}
}
