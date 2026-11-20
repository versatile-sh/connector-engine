package console

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

const (
	captureBodyMax = 1 << 20
	argumentsMax   = 16 << 10
	excerptMax     = 4 << 10
	detailMax      = 512
	codeMax        = 64
)

type captureKey struct{}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type toolCall struct {
	Name      string          `json:"name"`
	URI       string          `json:"uri"`
	Arguments json.RawMessage `json:"arguments"`
}

var capturedMethods = map[string]string{"tools/call": "", "prompts/get": port.ItemPrompt, "resources/read": port.ItemResource}

type inflight struct {
	activity *Activity
	results  bool
	mu       sync.Mutex
	byID     map[string]*Capture
	answers  map[string]rpcMessage
	open     int
}

func decodeMessages(raw []byte) ([]rpcMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var batch []rpcMessage
		return batch, json.Unmarshal(raw, &batch) == nil
	}
	var one rpcMessage
	err := json.Unmarshal(raw, &one)
	return []rpcMessage{one}, err == nil && (len(one.ID) > 0 || one.Method != "")
}

func rpcKey(id json.RawMessage) string { return string(bytes.TrimSpace(id)) }

func (c *Console) capture(next http.Handler) http.Handler {
	if c.Activity == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		head, err := io.ReadAll(io.LimitReader(r.Body, captureBodyMax+1))
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
		if err == nil && len(head) <= captureBodyMax {
			if calls := c.callsIn(r, head); calls != nil {
				r = r.WithContext(context.WithValue(r.Context(), captureKey{}, calls))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Console) callsIn(r *http.Request, body []byte) *inflight {
	msgs, ok := decodeMessages(body)
	if !ok {
		return nil
	}
	claims, ok := bearerClaims(r.Header.Get("Authorization"))
	if !ok {
		return nil
	}
	settings := c.Activity.Settings()
	now := time.Now().UTC()
	in := &inflight{activity: c.Activity, results: settings.RecordResults, byID: map[string]*Capture{}, answers: map[string]rpcMessage{}}
	for _, m := range msgs {
		var p toolCall
		item, captured := capturedMethods[m.Method]
		if !captured || len(m.ID) == 0 || json.Unmarshal(m.Params, &p) != nil {
			continue
		}
		cp := &Capture{ID: newCaptureID(now), At: now, Agent: claims.Agent, Tool: firstOf(p.Name, p.URI), Item: item, CallPayload: CallPayload{Session: claims.Session, MCPSession: r.Header.Get("Mcp-Session-Id"), RPCID: m.ID}}
		switch {
		case item != "":
		case p.Name == port.ToolFind, p.Name == port.ToolApprovalCancel, p.Name == port.ToolSkillList:
			continue
		case p.Name == port.ToolSkillLoad:
			var s port.SkillLoad
			if json.Unmarshal(p.Arguments, &s) != nil || s.Skill == "" {
				continue
			}
			cp.Skill, cp.Path = s.Skill, s.Path
		case p.Name == port.ToolCall:
			var inner toolCall
			if json.Unmarshal(p.Arguments, &inner) != nil || inner.Name == "" {
				continue
			}
			cp.Tool, p.Arguments = inner.Name, inner.Arguments
		case p.Name == port.ToolApprovalStatus:
			var a struct {
				ApprovalID string `json:"approval_id"`
			}
			if json.Unmarshal(p.Arguments, &a) != nil || a.ApprovalID == "" {
				continue
			}
			cp.Late, cp.Approval = true, a.ApprovalID
			p.Arguments = nil
		}
		if settings.RecordArguments && !cp.Late {
			cp.KeptArguments = true
			if len(p.Arguments) > 0 {
				args, ok := Redact(p.Arguments)
				if !ok {
					args = redactedValue
				}
				cp.Arguments, cp.ArgumentsCut = cut(args, argumentsMax)
			}
		}
		in.byID[rpcKey(m.ID)] = cp
		in.open++
	}
	if in.open == 0 {
		return nil
	}
	return in
}

func (in *inflight) wrap(resp *http.Response) {
	if resp.StatusCode != http.StatusOK || !strings.EqualFold(firstOf(resp.Header.Get("Content-Encoding"), "identity"), "identity") {
		return
	}
	kind := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.HasPrefix(kind, "text/event-stream"):
		resp.Body = &tap{body: resp.Body, calls: in, sse: true}
	case strings.HasPrefix(kind, "application/json"):
		resp.Body = &tap{body: resp.Body, calls: in}
	}
}

type tap struct {
	body     io.ReadCloser
	calls    *inflight
	sse      bool
	buf      []byte
	over     bool
	data     []byte
	dataOver bool
	size     int
	cr       bool
	done     bool
	once     sync.Once
}

// Read passes the engine's bytes on unchanged and watches them, through bounded buffers, for the answers.
func (t *tap) Read(p []byte) (int, error) {
	n, err := t.body.Read(p)
	if n > 0 && !t.done {
		t.size += n
		switch {
		case t.sse:
			t.scan(p[:n])
		case !t.over && len(t.buf)+n <= captureBodyMax:
			t.buf = append(t.buf, p[:n]...)
		default:
			t.over, t.buf = true, nil
		}
	}
	return n, err
}

// Close records the captures off the response path and closes the engine's body.
func (t *tap) Close() error {
	t.once.Do(func() { go t.finish() })
	return t.body.Close()
}

func (t *tap) scan(chunk []byte) {
	for _, b := range chunk {
		if t.cr && b == '\n' {
			t.cr = false
			continue
		}
		t.cr = b == '\r'
		if b != '\n' && b != '\r' {
			if len(t.buf) < captureBodyMax {
				t.buf = append(t.buf, b)
			} else {
				t.over = true
			}
			continue
		}
		t.line()
		if t.done {
			return
		}
	}
}

func (t *tap) line() {
	line, over := t.buf, t.over
	t.buf, t.over = t.buf[:0], false
	if len(line) == 0 {
		t.event()
		return
	}
	value, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return
	}
	value = bytes.TrimPrefix(value, []byte(" "))
	switch {
	case over || t.dataOver || len(t.data)+len(value)+1 > captureBodyMax:
		t.data, t.dataOver = t.data[:0], true
	case len(t.data) > 0:
		t.data = append(append(t.data, '\n'), value...)
	default:
		t.data = append(t.data, value...)
	}
}

func (t *tap) event() {
	if len(t.data) > 0 && !t.dataOver {
		if msgs, ok := decodeMessages(t.data); ok {
			for _, m := range msgs {
				if t.calls.take(m) == 0 {
					t.done = true
				}
			}
		}
	}
	t.data, t.dataOver = t.data[:0], false
}

func (t *tap) finish() {
	if !t.sse && !t.over {
		if msgs, ok := decodeMessages(t.buf); ok {
			for _, m := range msgs {
				t.calls.take(m)
			}
		}
	}
	t.calls.settle(t.over && !t.sse, t.size)
}

func (in *inflight) take(m rpcMessage) int {
	in.mu.Lock()
	defer in.mu.Unlock()
	k := rpcKey(m.ID)
	cp := in.byID[k]
	if cp == nil || !cp.Done.IsZero() || (len(m.Result) == 0 && len(m.Error) == 0) {
		return in.open
	}
	cp.Done = time.Now().UTC()
	in.answers[k] = m
	in.open--
	return in.open
}

func (in *inflight) describe(cp *Capture, m rpcMessage) {
	if len(m.Error) > 0 {
		var e struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Meta map[string]json.RawMessage `json:"_meta"`
			} `json:"data"`
		}
		_ = json.Unmarshal(m.Error, &e)
		cp.Outcome, cp.Code = OutcomeError, strconv.Itoa(e.Code)
		if own := metaString(e.Data.Meta, port.ErrorMeta); own == string(port.CodeUnknownTool) {
			cp.Outcome, cp.Code = OutcomeDenied, own
		} else if own != "" && len(own) <= codeMax {
			cp.Code = own
		}
		cp.Detail, _ = cut(redactText(e.Message), detailMax)
		return
	}
	var r struct {
		IsError    bool                       `json:"isError"`
		Meta       map[string]json.RawMessage `json:"_meta"`
		Structured struct {
			Status   string `json:"status"`
			Approval string `json:"approval_id"`
			Code     string `json:"code"`
			Detail   string `json:"detail"`
		} `json:"structuredContent"`
	}
	_ = json.Unmarshal(m.Result, &r)
	sc := r.Structured
	if !r.IsError || metaString(r.Meta, port.ErrorMeta) != sc.Code || len(sc.Code) > codeMax {
		sc.Code, sc.Detail = "", ""
	}
	cp.ResultBytes = len(m.Result)
	switch {
	case sc.Status == "pending_approval" && sc.Approval != "":
		cp.Outcome, cp.Approval = OutcomePending, sc.Approval
		return
	case r.IsError && sc.Code == "approval_denied":
		cp.Outcome, cp.Code = OutcomeDenied, sc.Code
	case r.IsError:
		cp.Outcome, cp.Code = OutcomeError, sc.Code
	default:
		cp.Outcome = OutcomeOK
	}
	if sc.Detail != "" {
		cp.Detail, _ = cut(redactText(sc.Detail), detailMax)
	}
	if cp.Approval == "" {
		cp.Approval = sc.Approval
	}
	if in.results {
		cp.KeptResult = true
		if excerpt, ok := Redact(m.Result); ok {
			cp.Result, cp.ResultCut = cut(excerpt, excerptMax)
		}
	}
}

func (in *inflight) settle(oversize bool, size int) {
	in.mu.Lock()
	defer in.mu.Unlock()
	for k, cp := range in.byID {
		if m, ok := in.answers[k]; ok {
			in.describe(cp, m)
		} else {
			cp.Done = time.Now().UTC()
			cp.Outcome, cp.Code = OutcomeError, "no_answer"
			if oversize {
				cp.Outcome, cp.Code, cp.ResultBytes = OutcomeOK, "", size
			}
		}
		if cp.Late && (cp.Outcome == OutcomePending || cp.Code == "not_found" || cp.Code == "no_answer") {
			continue
		}
		in.activity.Record(*cp)
	}
}
