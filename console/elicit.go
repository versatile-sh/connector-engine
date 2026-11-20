package console

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
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

func (c *Console) elicitProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			c.surf.el.forget(r.Header.Get(sessionHeader))
		}
		sub, ok := bearerClaims(r.Header.Get("Authorization"))
		agent, session := sub.Agent, sub.Session
		enabled := ok && c.State.Elicits(agent)
		peekInit := r.Header.Get(sessionHeader) == "" && r.Header.Get("MCP-Protocol-Version") < newEra
		if r.Method != http.MethodPost || !ok || !enabled && !peekInit || r.ContentLength > maxMCPBody {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxMCPBody+1))
		rest := r.Body
		reset := func() {
			r.Body = struct {
				io.Reader
				io.Closer
			}{io.MultiReader(bytes.NewReader(body), rest), rest}
		}
		reset()
		if err != nil || len(body) > maxMCPBody {
			next.ServeHTTP(w, r)
			return
		}
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
