package console_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/console"
	"github.com/agents-control/connector-engine/port"
)

type fakeEngine struct {
	mu        sync.Mutex
	mcp       http.HandlerFunc
	calls     []port.Call
	approvals []port.Approval
	separator string
}

func (f *fakeEngine) addCall(c port.Call) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

func (f *fakeEngine) setApproval(a port.Approval) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.approvals {
		if f.approvals[i].ID == a.ID {
			f.approvals[i] = a
			return
		}
	}
	f.approvals = append(f.approvals, a)
}

func (f *fakeEngine) handler() http.Handler {
	items := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": v})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /local/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "person"})
	})
	mux.HandleFunc("GET /v1/connectors", func(w http.ResponseWriter, _ *http.Request) {
		items(w, []map[string]any{{"connector": port.Connector{ID: "c-notes", Alias: "notes"}}, {"connector": port.Connector{ID: "c-mail", Alias: "mail"}},
			{"connector": port.Connector{ID: "c-pdf", Alias: "pdf", Listing: port.VersionRef{Listing: port.ListingRef{Name: "pdf"}, Version: "1"}}}})
	})
	mux.HandleFunc("GET /v1/listings/version", func(w http.ResponseWriter, r *http.Request) {
		m := port.Manifest{Name: r.URL.Query().Get("name")}
		if m.Name == "pdf" {
			m.Kind = port.KindSkill
		}
		_ = json.NewEncoder(w).Encode(port.ListingVersion{Manifest: m})
	})
	mux.HandleFunc("GET /v1/agents/{agent}/functions", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		sep := f.separator
		f.mu.Unlock()
		if sep == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"functions": []map[string]string{{"name": "notes" + sep + "search", "connector": "c-notes", "function": "search"}}})
	})
	mux.HandleFunc("GET /v1/calls", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		since, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
		items(w, slices.DeleteFunc(slices.Clone(f.calls), func(c port.Call) bool { return c.StartedAt.Before(since) }))
	})
	mux.HandleFunc("GET /v1/calls/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, c := range f.calls {
			if string(c.ID) == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(c)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "detail": "no call"})
	})
	mux.HandleFunc("GET /v1/approvals", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		since, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
		items(w, slices.DeleteFunc(slices.Clone(f.approvals), func(a port.Approval) bool { return a.CreatedAt.Before(since) }))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		h := f.mcp
		f.mu.Unlock()
		h(w, r)
	})
	return mux
}

type activityRig struct {
	t        *testing.T
	base     string
	engine   *fakeEngine
	activity *console.Activity
	dir      string
	b        *browser
	token    string
}

func agentToken(agent, session string) string {
	enc := base64.RawURLEncoding.EncodeToString
	payload, _ := json.Marshal(map[string]any{"agent": agent, "session": session, "iat": time.Now().Unix()})
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

func newActivityRig(t *testing.T) *activityRig {
	t.Helper()
	fe := &fakeEngine{}
	up := httptest.NewServer(fe.handler())
	t.Cleanup(up.Close)
	dir := t.TempDir()
	state, err := console.OpenState(dir + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	act, err := console.OpenActivity(filepath.Join(dir, "activity"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(act.Close)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	c := &console.Console{Engine: &console.Engine{PublicURL: up.URL, OperatorURL: up.URL, Operator: "op", Tenant: "local", Workspace: "default", Person: "owner"},
		State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}}, MCPURL: base + "/mcp", Activity: act}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	return &activityRig{t: t, base: base, engine: fe, activity: act, dir: filepath.Join(dir, "activity"), b: b, token: agentToken("claude-code", "claude-code-s1")}
}

func (r *activityRig) answer(h http.HandlerFunc) {
	r.engine.mu.Lock()
	defer r.engine.mu.Unlock()
	r.engine.mcp = h
}

func (r *activityRig) post(body string) (*http.Response, []byte) {
	r.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, r.base+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Mcp-Session-Id", "mcp-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		r.t.Fatal(err)
	}
	return resp, raw
}

func (r *activityRig) page(query string) console.ActivityPage {
	r.t.Helper()
	var p console.ActivityPage
	r.b.ok("GET", "/api/activity"+query, nil, &p)
	return p
}

func (r *activityRig) until(query string, ok func(console.ActivityPage) bool) console.ActivityPage {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p := r.page(query)
		if ok(p) || time.Now().After(deadline) {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (r *activityRig) rows(query string, n int) console.ActivityPage {
	r.t.Helper()
	return r.until(query, func(p console.ActivityPage) bool {
		return len(p.Calls) == n && !slices.ContainsFunc(p.Calls, func(c console.ActivityCall) bool { return !c.Captured })
	})
}

func (r *activityRig) detail(id string) console.ActivityDetail {
	r.t.Helper()
	var d console.ActivityDetail
	r.b.ok("GET", "/api/activity/"+id, nil, &d)
	return d
}

func toolCall(id int, name, args string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, args)
}

func jsonAnswer(raw string, onCall func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if onCall != nil {
			onCall()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, raw)
	}
}

func (r *activityRig) engineCall(id, function string, state port.CallState, isError bool) func() {
	return func() {
		now := time.Now().UTC()
		r.engine.addCall(port.Call{ID: port.CallID(id), Agent: "claude-code", Session: "claude-code-s1", Connector: "c-notes", Function: function, Class: port.ClassRead,
			State: state, StartedAt: now, FinishedAt: now.Add(3 * time.Millisecond), IsError: isError, ResultBytes: 42})
	}
}

func only(t *testing.T, p console.ActivityPage) console.ActivityCall {
	t.Helper()
	if len(p.Calls) != 1 {
		t.Fatalf("calls: %+v", p.Calls)
	}
	return p.Calls[0]
}

func TestAJSONAnswerIsCapturedJoinedAndPassedOnByteForByte(t *testing.T) {
	r := newActivityRig(t)
	const answer = `{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"3 notes"}],"isError":false}}` + "\n"
	r.answer(jsonAnswer(answer, r.engineCall("k1", "search", port.CallFinished, false)))
	resp, got := r.post(toolCall(7, "notes__search", `{"q":"groceries","api_key":"plain-key","nested":{"password":"hunter22"},"note":"Bearer abcdefghijklmnop","max_tokens":5}`))
	if resp.StatusCode != http.StatusOK || string(got) != answer {
		t.Fatalf("the client got %d %q, want the engine's bytes %q", resp.StatusCode, got, answer)
	}
	row := only(t, r.rows("", 1))
	if row.Function != "search" || row.Connector != "c-notes" || row.Alias != "notes" || row.Agent != "claude-code" || row.Outcome != console.OutcomeOK || row.Class != port.ClassRead || !row.Captured {
		t.Fatalf("row: %+v", row)
	}
	if row.DurationMS == nil || *row.DurationMS != 3 {
		t.Fatalf("the duration is the engine's: %v", row.DurationMS)
	}
	d := r.detail(row.ID)
	if d.EngineCall != "k1" || d.Session != "claude-code-s1" || d.MCPSession != "mcp-1" || string(d.RPCID) != "7" || d.Tool != "notes__search" || d.ResultBytes != 42 {
		t.Fatalf("detail: %+v", d)
	}
	for _, secret := range []string{"plain-key", "hunter22", "abcdefghijklmnop"} {
		if strings.Contains(d.Arguments, secret) {
			t.Fatalf("a secret was kept: %s", d.Arguments)
		}
	}
	if !d.KeptArguments || !strings.Contains(d.Arguments, `"q":"groceries"`) || !strings.Contains(d.Arguments, `"max_tokens":5`) {
		t.Fatalf("arguments: %s", d.Arguments)
	}
	if d.KeptResult || d.Result != "" {
		t.Fatalf("results are off by default: %+v", d)
	}
}

func TestAnSSEAnswerReachesTheClientBeforeTheStreamEnds(t *testing.T) {
	r := newActivityRig(t)
	r.b.ok("PUT", "/api/activity/settings", map[string]bool{"record_arguments": true, "record_results": true}, nil)
	const first = "event: message\r\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progress\":1}}\r\n\r\n"
	const second = "id: 2\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"call-1\",\n" +
		"data: \"result\":{\"content\":[{\"type\":\"text\",\"text\":\"sent with Bearer zyxwvutsrqponm\"}],\"structuredContent\":{\"to\":\"ana@example.com\"}}}\n\n"
	release := make(chan struct{})
	r.answer(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, first)
		w.(http.Flusher).Flush()
		<-release
		r.engineCall("k2", "send", port.CallFinished, false)()
		_, _ = io.WriteString(w, second)
	})
	req, _ := http.NewRequest(http.MethodPost, r.base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"call","arguments":{"name":"notes__send","arguments":{"to":"ana@example.com"}}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := make([]byte, len(first))
	read := make(chan error, 1)
	go func() { _, err := io.ReadFull(resp.Body, got); read <- err }()
	select {
	case err := <-read:
		if err != nil || string(got) != first {
			t.Fatalf("first event: %q %v", got, err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the first event was held back by the console")
	}
	close(release)
	rest, err := io.ReadAll(bufio.NewReader(resp.Body))
	if err != nil || string(rest) != second {
		t.Fatalf("the rest: %q %v", rest, err)
	}
	row := only(t, r.rows("", 1))
	if row.Function != "send" || row.Connector != "c-notes" || row.Outcome != console.OutcomeOK {
		t.Fatalf("the call meta-tool is unwrapped: %+v", row)
	}
	d := r.detail(row.ID)
	if d.EngineCall != "k2" || !d.KeptResult || !strings.Contains(d.Result, "ana@example.com") || strings.Contains(d.Result, "zyxwvutsrqponm") || !strings.Contains(d.Result, "[redacted]") {
		t.Fatalf("result excerpt: %+v", d)
	}
	if d.Arguments != `{"to":"ana@example.com"}` {
		t.Fatalf("the inner arguments: %s", d.Arguments)
	}
}

func TestDeniedHeldAndFailedCallsShowWhatTheyCameTo(t *testing.T) {
	r := newActivityRig(t)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"unknown tool \"notes__purge\"","data":{"_meta":{"connector-engine/error":"unknown_tool"}}}}`, nil))
	r.post(toolCall(1, "notes__purge", `{}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"waits"}],"structuredContent":{"status":"pending_approval","approval_id":"a1","poll_after_seconds":5}}}`, nil))
	r.engine.setApproval(port.Approval{ID: "a1", Agent: "claude-code", Connector: "c-notes", Function: "create", Class: port.ClassWrite, State: port.ApprovalPending, CreatedAt: time.Now()})
	r.post(toolCall(2, "notes__create", `{"title":"x"}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"upstream broke"}],"isError":true}}`, r.engineCall("k3", "search", port.CallFinished, true)))
	r.post(toolCall(3, "notes__search", `{}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":4,"result":{"tools":[]}}`, nil))
	r.post(toolCall(4, "find", `{"query":"notes"}`))

	byFn := map[string]console.ActivityCall{}
	for _, c := range r.rows("", 3).Calls {
		byFn[c.Function] = c
	}
	if len(byFn) != 3 {
		t.Fatalf("find is not a call of a connector: %+v", byFn)
	}
	if c := byFn["purge"]; c.Outcome != console.OutcomeDenied || c.Code != "unknown_tool" {
		t.Fatalf("denied: %+v", c)
	}
	if c := byFn["create"]; c.Outcome != console.OutcomePending || c.Approval != "a1" || c.ApprovalState != "pending" || c.Class != port.ClassWrite || c.DurationMS != nil {
		t.Fatalf("held: %+v", c)
	}
	if c := byFn["search"]; c.Outcome != console.OutcomeError || c.Class != port.ClassRead {
		t.Fatalf("failed: %+v", c)
	}
	if p := r.page("?outcome=pending"); len(p.Calls) != 1 || p.Calls[0].Function != "create" || p.Total != 1 {
		t.Fatalf("the outcome filter: %+v", p.Calls)
	}
	if p := r.page("?search=SEAR"); len(p.Calls) != 1 || p.Calls[0].Function != "search" || p.Total != 1 || len(p.Connectors) != 1 || p.Connectors[0].Total != 1 {
		t.Fatalf("the search narrows the calls and the totals to the functions it names: %+v %+v", p.Calls, p.Connectors)
	}

	r.engine.setApproval(port.Approval{ID: "a1", Agent: "claude-code", Connector: "c-notes", Function: "create", Class: port.ClassWrite, State: port.ApprovalExecuted, CreatedAt: time.Now()})
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":5,"result":{"content":[{"type":"text","text":"created"}]}}`, func() {
		now := time.Now().UTC()
		r.engine.addCall(port.Call{ID: "k4", Agent: "claude-code", Session: "claude-code-s1", Connector: "c-notes", Function: "create", Class: port.ClassWrite, Approval: "a1", State: port.CallFinished, StartedAt: now, FinishedAt: now})
	}))
	r.post(toolCall(5, "approval_status", `{"approval_id":"a1"}`))
	byFn = map[string]console.ActivityCall{}
	settled := r.until("", func(p console.ActivityPage) bool {
		return slices.ContainsFunc(p.Calls, func(c console.ActivityCall) bool { return c.Function == "create" && c.Outcome == console.OutcomeOK })
	})
	for _, c := range settled.Calls {
		byFn[c.Function] = c
	}
	if c := byFn["create"]; len(byFn) != 3 || c.Outcome != console.OutcomeOK || c.ApprovalState != "executed" {
		t.Fatalf("the approved call is one row, now ok: %+v", byFn)
	}
	if d := r.detail(byFn["create"].ID); d.EngineCall != "k4" || d.Arguments != `{"title":"x"}` {
		t.Fatalf("approved detail: %+v", d)
	}
}

func TestTwoCallsOfOneFunctionKeepTheirOwnArguments(t *testing.T) {
	r := newActivityRig(t)
	for i, q := range []string{"first", "second"} {
		r.answer(jsonAnswer(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"content":[]}}`, i), r.engineCall(fmt.Sprintf("k%d", i), "search", port.CallFinished, false)))
		r.post(toolCall(i, "notes__search", fmt.Sprintf(`{"q":%q}`, q)))
	}
	p := r.rows("", 2)
	if len(p.Calls) != 2 {
		t.Fatalf("calls: %+v", p.Calls)
	}
	for _, c := range p.Calls {
		d := r.detail(c.ID)
		want := map[string]string{"k0": `{"q":"first"}`, "k1": `{"q":"second"}`}[d.EngineCall]
		if want == "" || d.Arguments != want {
			t.Fatalf("engine call %s holds arguments %s", d.EngineCall, d.Arguments)
		}
	}
}

func TestEngineRowsWithoutACaptureAreListedToo(t *testing.T) {
	r := newActivityRig(t)
	now := time.Now().UTC()
	r.engine.addCall(port.Call{ID: "k9", Agent: "cursor", Session: "s9", Connector: "c-mail", Function: "list", Class: port.ClassRead, State: port.CallStarted, StartedAt: now.Add(-2 * time.Hour)})
	r.engine.addCall(port.Call{ID: "k8", Agent: "cursor", Session: "s9", Connector: "c-mail", Function: "old", State: port.CallFinished, StartedAt: now.Add(-48 * time.Hour)})
	p := r.page("")
	row := only(t, p)
	if row.ID != "e-k9" || row.Captured || row.Outcome != console.OutcomeRunning || row.Alias != "mail" {
		t.Fatalf("engine row: %+v", row)
	}
	if len(p.Connectors) != 1 || p.Connectors[0].Connector != "c-mail" || p.Connectors[0].Total != 1 || len(p.Agents) != 1 || p.Agents[0] != "cursor" {
		t.Fatalf("aggregates: %+v %+v", p.Connectors, p.Agents)
	}
	hours := 0
	for _, h := range p.Hours {
		hours += h.Running
	}
	if hours != 1 || len(p.Hours) < 24 || len(p.Hours) > 26 {
		t.Fatalf("hours: %d buckets, %d running", len(p.Hours), hours)
	}
	if d := r.detail("e-k9"); d.EngineCall != "k9" || d.KeptArguments || d.Function != "list" {
		t.Fatalf("engine detail: %+v", d)
	}
	if p := r.page("?from=" + now.Add(-72*time.Hour).Format(time.RFC3339)); len(p.Calls) != 2 {
		t.Fatalf("a wider range: %+v", p.Calls)
	}
	if p := r.page("?connector=c-notes"); len(p.Calls) != 0 || len(p.Connectors) != 1 {
		t.Fatalf("the connector filter keeps the per-connector totals: %+v", p)
	}
	for _, bad := range []string{"?from=yesterday", "?outcome=maybe", "?from=" + now.Format(time.RFC3339) + "&to=" + now.Add(-time.Hour).Format(time.RFC3339)} {
		if status, _ := r.b.send("GET", "/api/activity"+bad, nil, nil); status != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, status)
		}
	}
}

func TestTheSettingsDecideWhatIsKeptAndSurviveARestart(t *testing.T) {
	r := newActivityRig(t)
	if status, _ := r.b.send("PUT", "/api/activity/settings", map[string]bool{"record_arguments": false, "record_results": true}, map[string]string{"X-CSRF-Token": "nope"}); status != http.StatusForbidden {
		t.Fatalf("a settings change without CSRF: %d", status)
	}
	if status, _ := r.b.send("PUT", "/api/activity/settings", map[string]bool{"record_results": true}, nil); status != http.StatusBadRequest {
		t.Fatalf("half a settings change: %d", status)
	}
	var s map[string]any
	r.b.ok("GET", "/api/activity/settings", nil, &s)
	if s["record_arguments"] != true || s["record_results"] != false || s["retention_days"] != float64(30) {
		t.Fatalf("defaults: %v", s)
	}
	r.b.ok("PUT", "/api/activity/settings", map[string]bool{"record_arguments": false, "record_results": true}, &s)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Dear Ana, the password=swordfish1 is"}]}}`, nil))
	r.post(toolCall(1, "mail__read", `{"id":"m1"}`))
	row := only(t, r.rows("", 1))
	d := r.detail(row.ID)
	if d.KeptArguments || d.Arguments != "" || !d.KeptResult || !strings.Contains(d.Result, "Dear Ana") || strings.Contains(d.Result, "swordfish1") {
		t.Fatalf("arguments off, results on: %+v", d)
	}

	r.activity.Close()
	again, err := console.OpenActivity(r.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.Settings(); got.RecordArguments || !got.RecordResults {
		t.Fatalf("settings after a restart: %+v", got)
	}
	list, err := again.Captures(time.Now().Add(-time.Hour))
	if err != nil || len(list) != 1 || list[0].ID != row.ID {
		t.Fatalf("captures after a restart: %+v %v", list, err)
	}
}

func TestRetentionPrunesOldDaysAndDeleteAllClearsEverything(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02")+".jsonl")
	recent := filepath.Join(dir, time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")+".jsonl")
	line, _ := json.Marshal(console.Capture{ID: "x-1", At: time.Now().UTC().AddDate(0, 0, -3), Agent: "a", Tool: "notes__search", Outcome: console.OutcomeOK})
	for _, p := range []string{old, recent} {
		if err := os.WriteFile(p, append(line, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	act, err := console.OpenActivity(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer act.Close()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("a day past retention was kept: %v", err)
	}
	if list, err := act.Captures(time.Now().AddDate(0, 0, -60)); err != nil || len(list) != 1 {
		t.Fatalf("recent captures: %+v %v", list, err)
	}
	if f := act.Floor(); time.Since(f) < 29*24*time.Hour || time.Since(f) > 31*24*time.Hour {
		t.Fatalf("the floor is retention: %v", f)
	}
	if err := act.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recent); !os.IsNotExist(err) {
		t.Fatalf("delete all kept a day: %v", err)
	}
	if time.Since(act.Floor()) > time.Minute {
		t.Fatalf("the floor moves to the clearing: %v", act.Floor())
	}
	act.Record(console.Capture{ID: "x-2", At: time.Now().Add(-time.Minute), Agent: "a", Tool: "notes__search"})
	act.Flush()
	if list, _ := act.Captures(time.Time{}); len(list) != 0 {
		t.Fatalf("a call that started before the clearing came back: %+v", list)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Contains(raw, []byte("cleared_at")) {
		t.Fatalf("the clearing is kept: %s %v", raw, err)
	}
}

func TestDeleteAllHidesEngineRowsFromBeforeIt(t *testing.T) {
	r := newActivityRig(t)
	r.engine.addCall(port.Call{ID: "k1", Agent: "cursor", Connector: "c-mail", Function: "list", State: port.CallFinished, StartedAt: time.Now().UTC().Add(-time.Minute)})
	if len(r.page("").Calls) != 1 {
		t.Fatal("the engine row is listed first")
	}
	if status, _ := r.b.send("DELETE", "/api/activity", nil, map[string]string{"X-CSRF-Token": "nope"}); status != http.StatusForbidden {
		t.Fatalf("delete without CSRF: %d", status)
	}
	if status, _ := r.b.send("DELETE", "/api/activity", nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if p := r.page(""); len(p.Calls) != 0 {
		t.Fatalf("an engine row from before the clearing: %+v", p.Calls)
	}
	if status, _ := r.b.send("GET", "/api/activity/e-k1", nil, nil); status != http.StatusNotFound {
		t.Fatalf("its detail: %d", status)
	}
}

func TestTheActivityNeedsASignedInOwner(t *testing.T) {
	r := newActivityRig(t)
	anon := &browser{t: t, base: r.base, client: &http.Client{}}
	for _, path := range []string{"/api/activity", "/api/activity/settings", "/api/activity/x"} {
		if status, _ := anon.send("GET", path, nil, nil); status != http.StatusUnauthorized {
			t.Fatalf("%s without a session: %d", path, status)
		}
	}
}

func TestAnUnauthorizedAnswerIsNotRecorded(t *testing.T) {
	r := newActivityRig(t)
	r.answer(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		w.WriteHeader(http.StatusUnauthorized)
	})
	r.post(toolCall(1, "notes__search", `{}`))
	if p := r.page(""); len(p.Calls) != 0 {
		t.Fatalf("a refused token left a row: %+v", p.Calls)
	}
}

type realRig struct {
	*activityRig
	cs   *mcp.ClientSession
	conn console.ConnectorSummary
}

func newRealRig(t *testing.T, tweaks ...func(*config.Config)) *realRig {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, tweaks...)
	up := newUpstream(t)
	dir := t.TempDir()
	state, err := console.OpenState(dir + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	act, err := console.OpenActivity(dir+"/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(act.Close)
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Activity: act}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF
	b.ok("POST", "/api/import", map[string]any{"mcp": up.URL + "/mcp", "local": true}, nil)
	var list struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &list)
	var tok console.AgentToken
	b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &realRig{activityRig: &activityRig{t: t, b: b, activity: act}, cs: cs, conn: list.Connectors[0]}
}

func TestARealAgentCallIsJoinedWithTheEngineRecord(t *testing.T) {
	rr := newRealRig(t)
	cs, conn, r := rr.cs, rr.conn, rr.activityRig
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: conn.Alias + "__search", Arguments: map[string]any{"q": "milk", "token": "t0ps3cret"}})
	if err != nil || res.IsError || textOf(res) != "ran search" {
		t.Fatalf("search through the capturing proxy: %v %v", res, err)
	}
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: conn.Alias + "__purge", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	byFn := map[string]console.ActivityCall{}
	for _, row := range r.rows("", 2).Calls {
		byFn[row.Function] = row
	}
	search, purge := byFn["search"], byFn["purge"]
	if search.Outcome != console.OutcomeOK || search.Class != port.ClassRead || search.Connector != conn.ID || search.Agent != "claude-code" {
		t.Fatalf("search row: %+v", search)
	}
	if purge.Outcome != console.OutcomePending || purge.Approval == "" || purge.ApprovalState != "pending" {
		t.Fatalf("purge row: %+v", purge)
	}
	d := r.detail(search.ID)
	if d.EngineCall == "" || d.CallState != "finished" || d.Arguments != `{"q":"milk","token":"[redacted]"}` {
		t.Fatalf("search detail: %+v", d)
	}
}

func TestALaterPollDoesNotOverwriteTheFirstAnswerOfAnApproval(t *testing.T) {
	r := newActivityRig(t)
	r.b.ok("PUT", "/api/activity/settings", map[string]bool{"record_arguments": true, "record_results": true}, nil)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[],"structuredContent":{"status":"pending_approval","approval_id":"a2"}}}`, nil))
	r.post(toolCall(1, "notes__create", `{"title":"y"}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"created y"}]}}`, nil))
	r.post(toolCall(2, "approval_status", `{"approval_id":"a2"}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"result_not_kept: gone"}],"isError":true,"_meta":{"connector-engine/error":"result_not_kept"},"structuredContent":{"code":"result_not_kept","detail":"gone"}}}`, nil))
	r.post(toolCall(3, "approval_status", `{"approval_id":"a2"}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":4,"error":{"code":-32602,"message":"unknown tool \"search\"","data":{"_meta":{"connector-engine/error":"unknown_tool"}}}}`, nil))
	r.post(toolCall(4, "search", `{}`))
	p := r.rows("", 2)
	byFn := map[string]console.ActivityCall{}
	for _, c := range p.Calls {
		byFn[c.Function] = c
	}
	if c := byFn["search"]; c.Alias != "" || c.Connector != "" || c.Outcome != console.OutcomeDenied {
		t.Fatalf("a name without a prefix: %+v", c)
	}
	created := r.until("", func(p console.ActivityPage) bool {
		return slices.ContainsFunc(p.Calls, func(c console.ActivityCall) bool { return c.Function == "create" && c.Outcome == console.OutcomeOK })
	})
	for _, c := range created.Calls {
		if c.Function == "create" {
			if d := r.detail(c.ID); c.Outcome != console.OutcomeOK || !strings.Contains(d.Result, "created y") {
				t.Fatalf("the first answer of the approval: %+v %+v", c, d)
			}
		}
	}
}

func TestEachDetailTakesTheEngineRowThatStartedAfterItsRequest(t *testing.T) {
	r := newActivityRig(t)
	for i, failed := range []bool{true, false} {
		answer := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"content":[]}}`, i)
		r.answer(jsonAnswer(answer, func() {
			time.Sleep(30 * time.Millisecond)
			r.engineCall(fmt.Sprintf("k%d", i), "search", port.CallFinished, failed)()
		}))
		r.post(toolCall(i, "notes__search", fmt.Sprintf(`{"n":%d}`, i)))
	}
	for _, c := range r.rows("", 2).Calls {
		d := r.detail(c.ID)
		want := map[string]console.CallOutcome{`{"n":0}`: console.OutcomeError, `{"n":1}`: console.OutcomeOK}[d.Arguments]
		if d.Outcome != want || c.Outcome != want || d.EngineCall != "k"+d.Arguments[5:6] {
			t.Fatalf("arguments %s: list %s, detail %s from %s", d.Arguments, c.Outcome, d.Outcome, d.EngineCall)
		}
	}
}

func TestAnUpstreamErrorKeepsNoTextWhenExcerptsAreOff(t *testing.T) {
	r := newActivityRig(t)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"mailbox says no"}],"isError":true,"structuredContent":{"code":"Dear Ana, your invoice","detail":"is attached"}}}`, nil))
	r.post(toolCall(1, "mail__send", `{}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"credential_required: no usable credential for this call"}],"isError":true,"_meta":{"connector-engine/error":"credential_required"},"structuredContent":{"code":"credential_required","detail":"no usable credential for this call"}}}`, nil))
	r.post(toolCall(2, "mail__read", `{}`))
	byFn := map[string]console.ActivityCall{}
	for _, c := range r.rows("", 2).Calls {
		byFn[c.Function] = c
	}
	send := r.detail(byFn["send"].ID)
	if send.Outcome != console.OutcomeError || send.Code != "" || send.Detail != "" || send.Result != "" {
		t.Fatalf("the upstream's own error text was kept: %+v", send)
	}
	read := r.detail(byFn["read"].ID)
	if read.Code != "credential_required" || read.Detail != "no usable credential for this call" {
		t.Fatalf("the gateway's error: %+v", read)
	}
}

func TestARequestOverTheCaptureLimitPassesUnchangedAndUnrecorded(t *testing.T) {
	r := newActivityRig(t)
	var got []byte
	r.answer(func(w http.ResponseWriter, req *http.Request) {
		got, _ = io.ReadAll(req.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[]}}`)
	})
	body := toolCall(1, "notes__create", fmt.Sprintf(`{"text":%q}`, strings.Repeat("x", 1<<20+10)))
	resp, _ := r.post(body)
	if resp.StatusCode != http.StatusOK || string(got) != body {
		t.Fatalf("the engine got %d bytes of %d", len(got), len(body))
	}
	if p := r.page(""); len(p.Calls) != 0 {
		t.Fatalf("an over-limit request was recorded: %+v", p.Calls)
	}
}

func TestASkillLoadIsShownByTheSkillName(t *testing.T) {
	r := newActivityRig(t)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[],"structuredContent":{"skills":[]}}}`, nil))
	r.post(toolCall(1, "skill_list", `{}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"# fields"}]}}`, func() {
		now := time.Now().UTC()
		r.engine.addCall(port.Call{ID: "s1", Agent: "claude-code", Session: "claude-code-s1", Connector: "c-pdf", Function: "skill", Class: port.ClassRead, State: port.CallFinished, StartedAt: now, FinishedAt: now})
	}))
	r.post(toolCall(2, "skill_load", `{"skill":"pdf","path":"reference/fields.md"}`))
	r.engine.addCall(port.Call{ID: "s2", Agent: "cursor", Session: "s9", Connector: "c-pdf", Function: "skill", State: port.CallFinished, StartedAt: time.Now().UTC()})
	p := r.until("", func(p console.ActivityPage) bool {
		return len(p.Calls) == 2 && slices.ContainsFunc(p.Calls, func(c console.ActivityCall) bool { return c.Captured })
	})
	byID := map[string]console.ActivityCall{}
	for _, c := range p.Calls {
		byID[c.ID] = c
	}
	engineOnly := byID["e-s2"]
	if engineOnly.Skill != "pdf" || engineOnly.Connector != "c-pdf" {
		t.Fatalf("an engine row of a skill: %+v", p.Calls)
	}
	delete(byID, "e-s2")
	for _, c := range byID {
		if c.Skill != "pdf" || c.Path != "reference/fields.md" || c.Connector != "c-pdf" || c.Outcome != console.OutcomeOK || !c.Captured {
			t.Fatalf("the skill load: %+v", c)
		}
		if d := r.detail(c.ID); d.EngineCall != "s1" || d.Tool != "skill_load" {
			t.Fatalf("its detail: %+v", d)
		}
	}
	if len(p.Connectors) != 1 || !p.Connectors[0].Skill || p.Connectors[0].Total != 2 {
		t.Fatalf("per connector: %+v", p.Connectors)
	}
}

func TestACallApprovedInsideTheHoldWindowIsOneRow(t *testing.T) {
	rr := newRealRig(t, func(c *config.Config) { c.Approval.HoldWindow = 20 * time.Second })
	done := make(chan error, 1)
	go func() {
		res, err := rr.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: rr.conn.Alias + "__purge", Arguments: map[string]any{"all": true}})
		if err == nil && (res.IsError || textOf(res) != "ran purge") {
			err = fmt.Errorf("purge answered %v", res)
		}
		done <- err
	}()
	var waiting struct{ Approvals []struct{ ID string } }
	for deadline := time.Now().Add(10 * time.Second); len(waiting.Approvals) == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the held call never reached the approvals")
		}
		rr.b.ok("GET", "/api/approvals", nil, &waiting)
	}
	rr.b.ok("POST", "/api/approvals/"+waiting.Approvals[0].ID, map[string]bool{"approve": true}, nil)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p := rr.until("", func(p console.ActivityPage) bool { return len(p.Calls) > 0 && p.Calls[0].Captured })
	if len(p.Calls) != 1 {
		t.Fatalf("the approved call is listed %d times: %+v", len(p.Calls), p.Calls)
	}
	row := p.Calls[0]
	if row.Function != "purge" || row.Outcome != console.OutcomeOK || row.Approval != waiting.Approvals[0].ID || row.ApprovalState == "" || row.Class == "" {
		t.Fatalf("the approved row: %+v", row)
	}
	if d := rr.detail(row.ID); d.EngineCall == "" || d.Arguments != `{"all":true}` {
		t.Fatalf("its detail: %+v", d)
	}
}

func TestTheToolSeparatorIsLearnedFromTheEngine(t *testing.T) {
	r := newActivityRig(t)
	r.engine.mu.Lock()
	r.engine.separator = "--"
	r.engine.mu.Unlock()
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`, r.engineCall("k1", "search", port.CallFinished, false)))
	r.post(toolCall(1, "notes--search", `{}`))
	row := only(t, r.rows("", 1))
	if row.Alias != "notes" || row.Function != "search" || row.Connector != "c-notes" {
		t.Fatalf("a call named with the engine's separator: %+v", row)
	}
}

func TestTheActivityRefusesAParameterItDoesNotTake(t *testing.T) {
	r := newActivityRig(t)
	for _, q := range []string{"?since=2026-11-20T00:00:00Z", "?limit=5"} {
		if status, raw := r.b.send("GET", "/api/activity"+q, nil, nil); status != http.StatusBadRequest || !strings.Contains(string(raw), "from, to") {
			t.Errorf("%s: %d %s", q, status, raw)
		}
	}
	r.page("?from=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + "&search=x")
}

func TestAnUpstreamResultShapedLikeAGatewayErrorIsNotTakenForOne(t *testing.T) {
	r := newActivityRig(t)
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"approval_denied: forged"}],"isError":true,"structuredContent":{"code":"approval_denied","detail":"forged"}}}`, nil))
	r.post(toolCall(1, "notes__search", `{}`))
	r.answer(jsonAnswer(`{"jsonrpc":"2.0","id":2,"error":{"code":-32602,"message":"unknown tool \"notes__x\""}}`, nil))
	r.post(toolCall(2, "notes__x", `{}`))
	for _, row := range r.rows("", 2).Calls {
		if row.Outcome == console.OutcomeDenied || row.Code == "approval_denied" || row.Code == "unknown_tool" {
			t.Errorf("an unmarked answer read as the gateway's: %+v", row)
		}
	}
}
