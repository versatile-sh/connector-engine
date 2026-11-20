package console

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/surface"
)

const (
	elicitPrefix  = "ce-elicit-"
	elicitChannel = "mcp-elicitation"
	inputKey      = "approval"
	sessionHeader = "Mcp-Session-Id"
	newEra        = "2026-07-28"
	metaProtocol  = "io.modelcontextprotocol/protocolVersion"
	metaCaps      = "io.modelcontextprotocol/clientCapabilities"
	maxMCPBody    = 4 << 20
	maxSessions   = 1024
	bindPoll      = 250 * time.Millisecond
	bindSkew      = 2 * time.Second
	settlePoll    = time.Second
	runGrace      = 2 * time.Minute
)

var mcpClient = &http.Client{}

type elicitSession struct {
	agent string
	seen  time.Time
}

type elicitAnswer struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

type elicitWait struct {
	session, agent string
	answer         chan elicitAnswer
}

type elicitState struct {
	mu       sync.Mutex
	sessions map[string]elicitSession
	waits    map[string]elicitWait
	claimed  map[string]bool
}

func (e *elicitState) remember(session, agent string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sessions == nil {
		e.sessions = map[string]elicitSession{}
	}
	if len(e.sessions) >= maxSessions {
		oldest := ""
		for k, v := range e.sessions {
			if oldest == "" || v.seen.Before(e.sessions[oldest].seen) {
				oldest = k
			}
		}
		delete(e.sessions, oldest)
	}
	e.sessions[session] = elicitSession{agent: agent, seen: time.Now()}
}

func (e *elicitState) forget(session string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.sessions, session)
}

func (e *elicitState) elicits(session, agent string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sessions[session]
	if ok {
		s.seen = time.Now()
		e.sessions[session] = s
	}
	return ok && session != "" && s.agent == agent
}

func (e *elicitState) claim(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.claimed == nil {
		e.claimed = map[string]bool{}
	}
	if e.claimed[id] {
		return false
	}
	e.claimed[id] = true
	return true
}

func (e *elicitState) isClaimed(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.claimed[id]
}

func (e *elicitState) release(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.claimed, id)
}

func (e *elicitState) wait(id, session, agent string) chan elicitAnswer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.waits == nil {
		e.waits = map[string]elicitWait{}
	}
	ch := make(chan elicitAnswer, 1)
	e.waits[id] = elicitWait{session: session, agent: agent, answer: ch}
	return ch
}

func (e *elicitState) unwait(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.waits, id)
}

func (e *elicitState) deliver(id, session, agent string, a elicitAnswer) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	w, ok := e.waits[id]
	if !ok || w.session != session || w.agent != agent {
		return false
	}
	select {
	case w.answer <- a:
	default:
	}
	return true
}

type callParams struct {
	Name           string                     `json:"name"`
	Arguments      json.RawMessage            `json:"arguments,omitempty"`
	Meta           map[string]json.RawMessage `json:"_meta,omitempty"`
	InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   string                     `json:"requestState,omitempty"`
}

func agentSubject(header string) (agent, session string, ok bool) {
	scheme, raw, found := strings.Cut(header, " ")
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if !found || !strings.EqualFold(scheme, "Bearer") || len(parts) != 3 {
		return "", "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false
	}
	var claims struct {
		Agent   string `json:"agent"`
		Session string `json:"session"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Agent == "" {
		return "", "", false
	}
	return claims.Agent, claims.Session, true
}

func (c *Console) elicitProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			c.surf.el.forget(r.Header.Get(sessionHeader))
		}
		agent, session, ok := agentSubject(r.Header.Get("Authorization"))
		enabled := ok && c.State.Elicits(agent)
		if r.Method != http.MethodPost || !ok || !enabled && r.Header.Get(sessionHeader) != "" {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMCPBody))
		if err != nil {
			http.Error(w, "the request is too large", http.StatusRequestEntityTooLarge)
			return
		}
		reset := func() { r.Body = io.NopCloser(bytes.NewReader(body)) }
		reset()
		var m rpcMessage
		if json.Unmarshal(body, &m) != nil {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case m.Method == "initialize":
			c.captureInit(w, r, next, agent, m)
		case !enabled:
			next.ServeHTTP(w, r)
		case m.Method == "" && strings.HasPrefix(strings.Trim(string(m.ID), `"`), elicitPrefix):
			var a elicitAnswer
			_ = json.Unmarshal(m.Result, &a)
			if m.Error != nil {
				a.Action = "cancel"
			}
			if !c.surf.el.deliver(strings.Trim(string(m.ID), `"`), r.Header.Get(sessionHeader), agent, a) {
				http.Error(w, "unknown elicitation", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case m.Method == "tools/call" && c.elicitCall(w, r, body, m, agent, session):
		default:
			reset()
			next.ServeHTTP(w, r)
		}
	})
}

type sessionCapture struct {
	http.ResponseWriter
	elicits func(session string)
	done    bool
}

func (s *sessionCapture) WriteHeader(code int) {
	if !s.done {
		s.done = true
		if id := s.Header().Get(sessionHeader); id != "" && code < 300 {
			s.elicits(id)
		}
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *sessionCapture) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func elicitCapable(raw json.RawMessage) bool {
	var caps struct {
		Elicitation *struct {
			Form *struct{} `json:"form"`
			URL  *struct{} `json:"url"`
		} `json:"elicitation"`
	}
	if json.Unmarshal(raw, &caps) != nil || caps.Elicitation == nil {
		return false
	}
	return caps.Elicitation.Form != nil || caps.Elicitation.URL == nil
}

func (c *Console) captureInit(w http.ResponseWriter, r *http.Request, next http.Handler, agent string, m rpcMessage) {
	var p struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	_ = json.Unmarshal(m.Params, &p)
	if !elicitCapable(p.Capabilities) {
		next.ServeHTTP(w, r)
		return
	}
	next.ServeHTTP(&sessionCapture{ResponseWriter: w, elicits: func(id string) { c.surf.el.remember(id, agent) }}, r)
}

type engineAnswer struct {
	status int
	header http.Header
	body   []byte
	final  rpcMessage
	raw    []byte
	err    error
}

func (a engineAnswer) pendingID() string {
	var res struct {
		Structured struct {
			Status     string `json:"status"`
			ApprovalID string `json:"approval_id"`
		} `json:"structuredContent"`
	}
	if a.err != nil || json.Unmarshal(a.final.Result, &res) != nil || res.Structured.Status != "pending_approval" {
		return ""
	}
	return res.Structured.ApprovalID
}

func (c *Console) toEngine(ctx context.Context, r *http.Request, body []byte, id json.RawMessage) engineAnswer {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Engine.PublicURL, "/")+"/mcp", bytes.NewReader(body))
	if err != nil {
		return engineAnswer{err: err}
	}
	for k, v := range r.Header {
		if k == "Authorization" || k == "Content-Type" || k == "Accept" || strings.HasPrefix(strings.ToLower(k), "mcp-") {
			req.Header[k] = slices.Clone(v)
		}
	}
	req.Header.Set("X-Forwarded-Host", r.Host)
	resp, err := mcpClient.Do(req)
	if err != nil {
		return engineAnswer{err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	a := engineAnswer{status: resp.StatusCode, header: resp.Header, body: raw, err: err}
	if err != nil {
		return a
	}
	msgs := [][]byte{raw}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		msgs = sseData(raw)
	}
	for _, msg := range msgs {
		var m rpcMessage
		if json.Unmarshal(msg, &m) == nil && m.Method == "" && bytes.Equal(m.ID, id) {
			a.final, a.raw = m, msg
		}
	}
	return a
}

func sseData(raw []byte) [][]byte {
	var out [][]byte
	var cur bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if v, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			cur.Write(bytes.TrimSpace(v))
			continue
		}
		if len(line) == 0 && cur.Len() > 0 {
			out = append(out, slices.Clone(cur.Bytes()))
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.Bytes())
	}
	return out
}

func (c *Console) relay(w http.ResponseWriter, r *http.Request, a engineAnswer) {
	if a.err != nil {
		http.Error(w, "the engine is unreachable", http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Content-Type", sessionHeader, "Cache-Control"} {
		if v := a.header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if a.status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", c.challengeValue(c.base(r.Host), `error="invalid_token"`))
	}
	w.WriteHeader(a.status)
	_, _ = w.Write(a.body)
}

func metaString(meta map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(meta[key], &s)
	return s
}

func calledFunction(p callParams) string {
	if p.Name == "skill_load" {
		return "skill"
	}
	if p.Name != "call" {
		return p.Name
	}
	var inner struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(p.Arguments, &inner)
	return inner.Name
}

func callDigest(p callParams) string {
	var args bytes.Buffer
	_ = json.Compact(&args, p.Arguments)
	sum := sha256.Sum256(append([]byte(p.Name+"\x00"), args.Bytes()...))
	return hex.EncodeToString(sum[:])
}

func (c *Console) elicitCall(w http.ResponseWriter, r *http.Request, body []byte, m rpcMessage, agent, session string) bool {
	var p callParams
	if json.Unmarshal(m.Params, &p) != nil {
		return false
	}
	stateless := metaString(p.Meta, metaProtocol) >= newEra
	if stateless && !elicitCapable(p.Meta[metaCaps]) || !stateless && !c.surf.el.elicits(r.Header.Get(sessionHeader), agent) {
		return false
	}
	if stateless && p.RequestState != "" {
		c.answered(w, r, body, m, p, agent, session)
		return true
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	first := make(chan engineAnswer, 1)
	go func() { first <- c.toEngine(ctx, r, body, m.ID) }()
	id, ans, done := c.bindCall(ctx, first, agent, session, calledFunction(p), time.Now())
	if id == "" {
		c.relay(w, r, ans)
		return true
	}
	v, ok := c.elicitable(ctx, id, agent, session)
	if !ok {
		if !done {
			ans = <-first
		}
		c.relay(w, r, ans)
		return true
	}
	defer c.surf.el.release(id)
	if stateless {
		cancel()
		if !done {
			<-first
		}
		c.askStateless(w, m, p, v, agent, session)
		return true
	}
	c.askOnStream(w, r, body, m, v, agent, first, ans, done)
	return true
}

func (c *Console) bindCall(ctx context.Context, first chan engineAnswer, agent, session, function string, start time.Time) (string, engineAnswer, bool) {
	tick := time.NewTicker(bindPoll)
	defer tick.Stop()
	for {
		select {
		case a := <-first:
			return a.pendingID(), a, true
		case <-tick.C:
			if id := c.correlate(ctx, agent, session, function, start); id != "" {
				return id, engineAnswer{}, false
			}
		case <-ctx.Done():
			return "", engineAnswer{err: ctx.Err()}, true
		}
	}
}

func (c *Console) correlate(ctx context.Context, agent, session, function string, start time.Time) string {
	var list items[port.Approval]
	if err := c.Engine.Call(ctx, http.MethodGet, "/v1/approvals"+q("state", string(port.ApprovalPending), "agent", agent, "session", session), nil, &list); err != nil {
		return ""
	}
	var found []string
	for _, a := range list.Items {
		if (function == a.Function || strings.HasSuffix(function, "__"+a.Function)) && !a.CreatedAt.Before(start.Add(-bindSkew)) && !c.surf.el.isClaimed(string(a.ID)) {
			found = append(found, string(a.ID))
		}
	}
	if len(found) != 1 {
		return ""
	}
	return found[0]
}

func (c *Console) elicitable(ctx context.Context, id, agent, session string) (ApprovalView, bool) {
	v, err := c.Approval(ctx, id)
	if err != nil || v.State != string(port.ApprovalPending) || v.App != agent || v.Session != session || v.Kind == port.ClassDestructive {
		return v, false
	}
	return v, c.surf.el.claim(id)
}

func elicitRequest(v ApprovalView) map[string]any {
	enum, names := []string{string(surface.ChoiceOnce)}, []string{surface.Label(surface.ChoiceOnce)}
	if slices.Contains(v.Choices, surface.ChoiceSession) {
		enum, names = append(enum, string(surface.ChoiceSession)), append(names, surface.Label(surface.ChoiceSession))
	}
	enum, names = append(enum, string(surface.ChoiceDeny)), append(names, surface.Label(surface.ChoiceDeny))
	name := firstOf(v.FunctionTitle, v.Function)
	msg := fmt.Sprintf("Allow %s: %s?\n%s wants to run %s. The request expires at %s.", v.Connector.Title, name, v.App, v.Function, v.ExpiresAt.Local().Format("15:04"))
	schema := map[string]any{"type": "object", "required": []string{"decision"}, "properties": map[string]any{
		"decision": map[string]any{"type": "string", "title": "Answer", "enum": enum, "enumNames": names, "default": string(surface.ChoiceOnce)}}}
	return map[string]any{"mode": "form", "message": msg, "requestedSchema": schema}
}

func (c *Console) apply(ctx context.Context, v ApprovalView, a elicitAnswer, agent string) {
	choice := surface.ChoiceDeny
	switch a.Action {
	case "accept":
		s, _ := a.Content["decision"].(string)
		choice = surface.Choice(s)
		if choice == surface.ChoiceAlways || !slices.Contains(v.Choices, choice) {
			return
		}
	case "decline":
	default:
		return
	}
	if _, _, err := c.DecideChoice(ctx, v.ID, choice, elicitChannel, elicitChannel+":"+agent); err != nil {
		c.logger().Warn("elicitation decision", "approval", v.ID, "error", err)
	}
}

func (c *Console) stillPending(ctx context.Context, id string) bool {
	v, err := c.Approval(ctx, id)
	return err == nil && v.State == string(port.ApprovalPending)
}

func (c *Console) settled(ctx context.Context, r *http.Request, body []byte, id json.RawMessage, approval string, a engineAnswer, deadline time.Time) engineAnswer {
	for a.err == nil && a.pendingID() != "" && time.Now().Before(deadline) && !c.stillPending(ctx, approval) {
		a = c.toEngine(ctx, r, body, id)
	}
	return a
}

func (c *Console) askOnStream(w http.ResponseWriter, r *http.Request, body []byte, m rpcMessage, v ApprovalView, agent string, first chan engineAnswer, ans engineAnswer, done bool) {
	ctx := r.Context()
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	send := func(v any) {
		raw, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", raw)
		_ = rc.Flush()
	}
	eid := elicitPrefix + randomID()[:20]
	answer := c.surf.el.wait(eid, r.Header.Get(sessionHeader), agent)
	defer c.surf.el.unwait(eid)
	send(map[string]any{"jsonrpc": "2.0", "id": eid, "method": "elicitation/create", "params": elicitRequest(v)})
	poll := time.NewTicker(settlePoll)
	defer poll.Stop()
	expiry := time.NewTimer(time.Until(v.ExpiresAt))
	defer expiry.Stop()
wait:
	for {
		select {
		case a := <-answer:
			c.apply(ctx, v, a, agent)
			break wait
		case a := <-first:
			ans, done, first = a, true, nil
			if a.pendingID() == "" {
				send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": eid, "reason": "answered"}})
				break wait
			}
		case <-poll.C:
			if !c.stillPending(ctx, v.ID) {
				send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": eid, "reason": "decided elsewhere"}})
				break wait
			}
		case <-expiry.C:
			break wait
		case <-ctx.Done():
			return
		}
	}
	if !done {
		ans = <-first
	}
	ans = c.settled(ctx, r, body, m.ID, v.ID, ans, v.ExpiresAt.Add(runGrace))
	if ans.err != nil || ans.raw == nil {
		send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32603, "message": "the engine did not answer"}})
		return
	}
	_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", ans.raw)
	_ = rc.Flush()
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, key string, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, key: v})
}

func (c *Console) askStateless(w http.ResponseWriter, m rpcMessage, p callParams, v ApprovalView, agent, session string) {
	state, err := c.sign(stateKind, v.ExpiresAt, v.ID, agent, session, callDigest(p))
	if err != nil {
		writeRPC(w, m.ID, "error", map[string]any{"code": -32603, "message": "cannot sign the request state"})
		return
	}
	writeRPC(w, m.ID, "result", map[string]any{"resultType": "input_required", "requestState": state,
		"inputRequests": map[string]any{inputKey: map[string]any{"method": "elicitation/create", "params": elicitRequest(v)}}})
}

func (c *Console) answered(w http.ResponseWriter, r *http.Request, body []byte, m rpcMessage, p callParams, agent, session string) {
	_, f, err := c.open(stateKind, p.RequestState)
	if err != nil || len(f) != 4 || f[1] != agent || f[2] != session || f[3] != callDigest(p) {
		writeRPC(w, m.ID, "error", map[string]any{"code": -32602, "message": "invalid requestState"})
		return
	}
	ctx := r.Context()
	if v, err := c.Approval(ctx, f[0]); err == nil && v.State == string(port.ApprovalPending) && v.App == agent && v.Session == session {
		var a elicitAnswer
		_ = json.Unmarshal(p.InputResponses[inputKey], &a)
		c.apply(ctx, v, a, agent)
	}
	body = withoutAnswers(body, m)
	a := c.toEngine(ctx, r, body, m.ID)
	c.relay(w, r, c.settled(ctx, r, body, m.ID, f[0], a, time.Now().Add(runGrace)))
}

func withoutAnswers(body []byte, m rpcMessage) []byte {
	var params, msg map[string]json.RawMessage
	_ = json.Unmarshal(m.Params, &params)
	delete(params, "inputResponses")
	delete(params, "requestState")
	_ = json.Unmarshal(body, &msg)
	msg["params"], _ = json.Marshal(params)
	out, _ := json.Marshal(msg)
	return out
}
