package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

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
	take := func(msg []byte) {
		var m rpcMessage
		if json.Unmarshal(msg, &m) == nil && m.Method == "" && bytes.Equal(m.ID, id) {
			a.final, a.raw = m, msg
		}
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		_ = ReadEvents(bytes.NewReader(raw), take)
	} else {
		take(raw)
	}
	return a
}

func (c *Console) passAnswer(w http.ResponseWriter, r *http.Request, a engineAnswer) {
	if a.err != nil {
		c.engineDown(w)
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
	captured(r, a.status, a.header.Get("Content-Type"), a.body)
}

func captured(r *http.Request, status int, kind string, body []byte) {
	calls, ok := r.Context().Value(captureKey{}).(*inflight)
	if !ok {
		return
	}
	resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {kind}}, Body: io.NopCloser(bytes.NewReader(body))}
	calls.wrap(resp)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func metaString(meta map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(meta[key], &s)
	return s
}

func calledFunction(p callParams) string {
	if p.Name == port.ToolSkillLoad {
		return port.SkillFunction
	}
	if p.Name != port.ToolCall {
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
		c.passAnswer(w, r, ans)
		return true
	}
	v, ok := c.elicitable(ctx, id, agent, session)
	if !ok {
		if !done {
			ans = <-first
		}
		c.passAnswer(w, r, ans)
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
	pending, err := c.Engine.pendingApprovals(ctx, agent, session)
	if err != nil {
		return ""
	}
	sep := c.separator(ctx, agent)
	var found []string
	for _, a := range pending {
		if (function == a.Function || strings.HasSuffix(function, sep+a.Function)) && !a.CreatedAt.Before(start.Add(-bindSkew)) && !c.surf.el.isClaimed(string(a.ID)) {
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
	if err != nil || v.State != string(port.ApprovalPending) || v.App != agent || v.Session != session || !v.known || v.Kind == port.ClassDestructive {
		return v, false
	}
	return v, c.surf.el.claim(id)
}

func elicitRequest(v ApprovalView) map[string]any {
	enum, names := []string{string(surface.ChoiceOnce)}, []string{surface.Label(surface.ChoiceOnce, v.SessionUntil)}
	if slices.Contains(v.Choices, surface.ChoiceSession) {
		enum, names = append(enum, string(surface.ChoiceSession)), append(names, surface.Label(surface.ChoiceSession, v.SessionUntil))
	}
	enum, names = append(enum, string(surface.ChoiceDeny)), append(names, surface.Label(surface.ChoiceDeny, v.SessionUntil))
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
	if _, _, err := c.DecideChoice(WithActor(ctx, c.State.KeyOwner(agent)), v.ID, choice, elicitChannel, elicitChannel+":"+agent); err != nil {
		c.logger().Warn("elicitation decision", "approval", v.ID, "error", err)
	}
}

func (c *Console) stillPending(ctx context.Context, id string) bool {
	a, err := c.record(ctx, id)
	return err == nil && a.State == port.ApprovalPending
}

func (c *Console) settled(ctx context.Context, r *http.Request, body []byte, id json.RawMessage, approval string, a engineAnswer, deadline time.Time) engineAnswer {
	for a.err == nil && a.pendingID() == approval && time.Now().Before(deadline) && !c.stillPending(ctx, approval) {
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
	captured(r, http.StatusOK, "application/json", ans.raw)
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, key string, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, key: v})
}

func (c *Console) askStateless(w http.ResponseWriter, m rpcMessage, p callParams, v ApprovalView, agent, session string) {
	state, err := c.sign(stateKind, v.ExpiresAt, v.ID, agent, session, callDigest(p))
	if err != nil {
		message := "cannot sign the request state"
		if errors.Is(err, sealed.ErrLocked) {
			message = gatewayLocked
		}
		writeRPC(w, m.ID, "error", map[string]any{"code": -32603, "message": message})
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
	c.passAnswer(w, r, c.settled(ctx, r, body, m.ID, f[0], a, time.Now().Add(runGrace)))
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
