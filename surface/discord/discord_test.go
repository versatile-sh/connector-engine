package discord

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

	"github.com/agents-control/connector-engine/surface"
)

const token = "BOT.SECRET-discord-token"

type request struct {
	method, path, auth string
	body               map[string]any
}

type fake struct {
	t        *testing.T
	mu       sync.Mutex
	events   []string
	requests []request
	frames   chan any
	identify chan map[string]any
	opcode   int
	interval int
	srv      *httptest.Server
}

func (f *fake) note(e string) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

func (f *fake) snapshot() ([]string, []request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...), append([]request(nil), f.requests...)
}

func (f *fake) find(method, prefix string) []request {
	_, rs := f.snapshot()
	var out []request
	for _, r := range rs {
		if r.method == method && strings.HasPrefix(r.path, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func (f *fake) rest(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.requests = append(f.requests, request{r.Method, r.URL.Path, r.Header.Get("Authorization"), body})
	f.mu.Unlock()
	switch {
	case r.URL.Path == "/api/users/@me/channels":
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "dm1"})
	case strings.HasPrefix(r.URL.Path, "/api/channels/") && r.Method == http.MethodPost:
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "m1", "channel_id": "dm1"})
	case r.URL.Path == "/api/gateway/bot":
		_ = json.NewEncoder(w).Encode(map[string]string{"url": "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/gw"})
	case strings.HasSuffix(r.URL.Path, "/callback"):
		f.note("callback:" + jsonString(body["type"]))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPatch:
		f.note("edit")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "m1"})
	default:
		f.note("other:" + r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "x"})
	}
}

func jsonString(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(b.String())
}

func (f *fake) gateway(ws *websocket.Conn) {
	if !strings.Contains(ws.Request().URL.RawQuery, "v=10") || !strings.Contains(ws.Request().URL.RawQuery, "encoding=json") {
		f.t.Errorf("gateway query = %q", ws.Request().URL.RawQuery)
	}
	_ = websocket.JSON.Send(ws, map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": f.interval}})
	var id map[string]any
	if websocket.JSON.Receive(ws, &id) != nil {
		return
	}
	f.identify <- id
	if f.opcode != 0 {
		_ = websocket.JSON.Send(ws, map[string]any{"op": f.opcode, "d": false})
	}
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
		if in["op"] == float64(1) {
			f.note("heartbeat")
			_ = websocket.JSON.Send(ws, map[string]any{"op": 11})
		}
	}
}

func setup(t *testing.T) (*Channel, *fake) {
	t.Helper()
	f := &fake{t: t, interval: 60000, frames: make(chan any, 4), identify: make(chan map[string]any, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", f.rest)
	mux.Handle("/gw", websocket.Handler(f.gateway))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(func() { close(f.frames); f.srv.Close() })
	c, err := New(Config{Token: token, OwnerUserID: "42", APIBase: f.srv.URL + "/api"})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func prompt(choices ...surface.Choice) surface.Prompt {
	return surface.Prompt{Key: "k1", Approval: "ap-1", Connector: "Notes", Function: "create_note", FunctionTitle: "Create note",
		Kind: "write", App: "claude-code", ExpiresAt: time.Unix(1800000000, 0), Choices: choices}
}

func TestNew(t *testing.T) {
	for _, cfg := range []Config{{OwnerUserID: "1"}, {Token: "t", OwnerUserID: "x"}, {Token: "t", OwnerUserID: "1", GatewayURL: "http://x"}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded", cfg)
		}
	}
	c, err := New(Config{Token: "t", OwnerUserID: "1"})
	if err != nil || c.base != defaultAPIBase || c.Kind() != "discord" {
		t.Fatalf("defaults: %v %+v", err, c)
	}
}

func TestNotifyRendersEmbedAndButtons(t *testing.T) {
	c, f := setup(t)
	p := prompt(surface.ChoiceOnce, surface.ChoiceSession, surface.ChoiceAlways, surface.ChoiceDeny)
	ref, err := c.Notify(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Notify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if ref != "dm1/m1" {
		t.Fatalf("ref = %q", ref)
	}
	if dms := f.find(http.MethodPost, "/api/users/@me/channels"); len(dms) != 1 || dms[0].body["recipient_id"] != "42" || dms[0].auth != "Bot "+token {
		t.Fatalf("dm opens = %+v", dms)
	}
	msg := f.find(http.MethodPost, "/api/channels/dm1/messages")[0].body
	raw := jsonString(msg)
	for _, want := range []string{"Allow Notes: Create note?", "claude-code", "<t:1800000000:R>", "versatile.sh"} {
		if !strings.Contains(raw, want) {
			t.Errorf("message lacks %q: %s", want, raw)
		}
	}
	row := msg["components"].([]any)[0].(map[string]any)["components"].([]any)
	var styles, ids []string
	for _, b := range row {
		m := b.(map[string]any)
		styles = append(styles, jsonString(m["style"]))
		ids = append(ids, m["custom_id"].(string))
	}
	if strings.Join(styles, ",") != "3,3,1,4" || strings.Join(ids, ",") != "o:k1,s:k1,a:k1,d:k1" {
		t.Fatalf("buttons styles=%v ids=%v", styles, ids)
	}
}

func TestNotifyWithoutChoicesHasNoButtons(t *testing.T) {
	c, f := setup(t)
	if _, err := c.Notify(context.Background(), prompt()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.find(http.MethodPost, "/api/channels/")[0].body["components"]; ok {
		t.Fatal("components sent without choices")
	}
}

func TestSettleRecoloursAndRemovesButtons(t *testing.T) {
	c, f := setup(t)
	o := surface.Outcome{State: surface.StateDenied, By: "discord:42", Channel: "discord"}
	if err := c.Settle(context.Background(), "dm1/m1", prompt(surface.ChoiceOnce), o); err != nil {
		t.Fatal(err)
	}
	e := f.find(http.MethodPatch, "/api/channels/dm1/messages/m1")[0].body
	if comps := e["components"].([]any); len(comps) != 0 {
		t.Fatalf("components = %v", comps)
	}
	em := e["embeds"].([]any)[0].(map[string]any)
	if em["color"] != float64(colorDenied) || !strings.Contains(jsonString(em["fields"]), surface.Sentence(o)) {
		t.Fatalf("embed = %v", em)
	}
}

func listen(t *testing.T, c *Channel, f *fake, click surface.ClickFunc) (context.CancelFunc, chan error) {
	t.Helper()
	c.gateway = "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/gw"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Listen(ctx, click) }()
	select {
	case id := <-f.identify:
		d := id["d"].(map[string]any)
		if id["op"] != float64(2) || d["token"] != token || d["intents"] != float64(0) {
			t.Fatalf("identify = %v", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no IDENTIFY")
	}
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

func interactionFrame(user, customID string) map[string]any {
	return map[string]any{"op": 0, "s": 3, "t": "INTERACTION_CREATE", "d": map[string]any{
		"id": "i1", "application_id": "app1", "type": 3, "token": "INTERACTION-SECRET", "channel_id": "dm1",
		"user": map[string]any{"id": user}, "data": map[string]any{"custom_id": customID, "component_type": 2},
		"message": map[string]any{"id": "m1", "channel_id": "dm1", "embeds": []any{map[string]any{"title": "Allow Notes: Create note?", "color": colorPending}}},
	}}
}

func has(events []string, e string) bool {
	for _, x := range events {
		if x == e {
			return true
		}
	}
	return false
}

func TestOwnerClickAcksThenDecidesThenEdits(t *testing.T) {
	c, f := setup(t)
	var got surface.Click
	o := surface.Outcome{State: surface.StateApproved, By: "discord:42", Channel: "discord"}
	cancel, done := listen(t, c, f, func(_ context.Context, cl surface.Click) (surface.Outcome, error) {
		got = cl
		f.note("click")
		return o, nil
	})
	f.frames <- interactionFrame("42", "o:k1")
	waitFor(t, func() bool { ev, _ := f.snapshot(); return has(ev, "edit") })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Listen = %v", err)
	}
	ev, _ := f.snapshot()
	var order []string
	for _, e := range ev {
		if e != "heartbeat" {
			order = append(order, e)
		}
	}
	if strings.Join(order, ",") != "callback:6,click,edit" {
		t.Fatalf("order = %v", order)
	}
	if got != (surface.Click{Key: "k1", Choice: surface.ChoiceOnce, User: "42"}) {
		t.Fatalf("click = %+v", got)
	}
	cb := f.find(http.MethodPost, "/api/interactions/i1/INTERACTION-SECRET/callback")
	if len(cb) != 1 {
		t.Fatalf("callbacks = %+v", cb)
	}
	e := f.find(http.MethodPatch, "/api/channels/dm1/messages/m1")[0].body
	if !strings.Contains(jsonString(e), surface.Sentence(o)) || len(e["components"].([]any)) != 0 {
		t.Fatalf("edit = %v", e)
	}
}

func TestHeartbeatsAreSent(t *testing.T) {
	c, f := setup(t)
	f.interval = 10
	cancel, done := listen(t, c, f, nil)
	waitFor(t, func() bool { ev, _ := f.snapshot(); return has(ev, "heartbeat") })
	cancel()
	<-done
}

func TestMemberUserIsTheClicker(t *testing.T) {
	c, f := setup(t)
	var got surface.Click
	cancel, done := listen(t, c, f, func(_ context.Context, cl surface.Click) (surface.Outcome, error) {
		got = cl
		return surface.Outcome{State: surface.StateDenied}, nil
	})
	fr := interactionFrame("99", "d:k1")
	d := fr["d"].(map[string]any)
	delete(d, "user")
	d["member"] = map[string]any{"user": map[string]any{"id": "42"}}
	f.frames <- fr
	waitFor(t, func() bool { ev, _ := f.snapshot(); return has(ev, "edit") })
	cancel()
	<-done
	if got.User != "42" || got.Choice != surface.ChoiceDeny {
		t.Fatalf("click = %+v", got)
	}
}

func TestNonOwnerClickGetsEphemeralRefusal(t *testing.T) {
	c, f := setup(t)
	clicked := false
	cancel, done := listen(t, c, f, func(context.Context, surface.Click) (surface.Outcome, error) {
		clicked = true
		return surface.Outcome{}, nil
	})
	f.frames <- interactionFrame("99", "o:k1")
	waitFor(t, func() bool { return len(f.find(http.MethodPost, "/api/interactions/")) == 1 })
	cancel()
	<-done
	if clicked {
		t.Fatal("ClickFunc called for a non-owner")
	}
	b := f.find(http.MethodPost, "/api/interactions/")[0].body
	data := b["data"].(map[string]any)
	if b["type"] != float64(4) || data["flags"] != float64(64) || data["content"] != surface.ErrNotOwner.Error() {
		t.Fatalf("refusal = %v", b)
	}
	if len(f.find(http.MethodPatch, "/")) != 0 {
		t.Fatal("message edited for a non-owner")
	}
}

func TestExpiredClickSendsEphemeralFollowup(t *testing.T) {
	c, f := setup(t)
	cancel, done := listen(t, c, f, func(context.Context, surface.Click) (surface.Outcome, error) {
		return surface.Outcome{}, surface.ErrExpired
	})
	f.frames <- interactionFrame("42", "o:gone")
	waitFor(t, func() bool { return len(f.find(http.MethodPost, "/api/webhooks/app1/INTERACTION-SECRET")) == 1 })
	cancel()
	<-done
	b := f.find(http.MethodPost, "/api/webhooks/app1/INTERACTION-SECRET")[0].body
	if b["content"] != surface.ErrExpired.Error() || b["flags"] != float64(64) {
		t.Fatalf("followup = %v", b)
	}
	if len(f.find(http.MethodPatch, "/")) != 0 {
		t.Fatal("message edited on an expired click")
	}
}

func TestReconnectOpcodeReturnsError(t *testing.T) {
	for _, op := range []int{7, 9} {
		c, f := setup(t)
		f.opcode = op
		cancel, done := listen(t, c, f, nil)
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("op %d: Listen returned nil", op)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("op %d: Listen did not return", op)
		}
		cancel()
	}
}

func TestListenUsesGatewayBotAndReturnsNilOnCancel(t *testing.T) {
	c, f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Listen(ctx, nil) }()
	<-f.identify
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Listen = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Listen did not return")
	}
	if len(f.find(http.MethodGet, "/api/gateway/bot")) != 1 {
		t.Fatal("gateway/bot not used")
	}
}

func TestErrorsNeverCarryToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c, err := New(Config{Token: token, OwnerUserID: "42", APIBase: srv.URL, GatewayURL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/gw?token=" + token})
	if err != nil {
		t.Fatal(err)
	}
	_, nerr := c.Notify(context.Background(), prompt(surface.ChoiceOnce))
	serr := c.Settle(context.Background(), "dm1/m1", prompt(), surface.Outcome{State: surface.StateDenied})
	lerr := c.Listen(context.Background(), nil)
	for _, e := range []error{nerr, serr, lerr} {
		if e == nil {
			t.Fatal("expected an error from a closed server")
		}
		if strings.Contains(e.Error(), "SECRET") {
			t.Fatalf("token leaked: %v", e)
		}
	}
}

func TestAnUpstreamNameCannotPlantMarkdownInTheCard(t *testing.T) {
	p := prompt()
	p.Function, p.FunctionTitle, p.App = "x` [Approve here](https://evil.example) `", "[Approve](https://evil.example)", "@everyone"
	e := card(p)
	if e.Description != "`x' [Approve here](https://evil.example) '`" {
		t.Errorf("the function name left its code span: %q", e.Description)
	}
	if strings.Contains(e.Title, "[Approve]") || !strings.Contains(e.Title, `\[Approve\]\(https://evil.example\)`) {
		t.Errorf("the title renders a link: %q", e.Title)
	}
	if e.Fields[0].Value != `\@everyone` {
		t.Errorf("the app name mentions: %q", e.Fields[0].Value)
	}
}
