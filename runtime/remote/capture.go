package remote

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

type recorded struct {
	id          json.RawMessage
	contentType string
	mu          sync.Mutex
	body        bytes.Buffer
}

func (r *recorded) write(p []byte) {
	r.mu.Lock()
	r.body.Write(p)
	r.mu.Unlock()
}

type response struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
}

func (r *recorded) result() (json.RawMessage, bool) {
	r.mu.Lock()
	data := bytes.Clone(r.body.Bytes())
	r.mu.Unlock()
	payloads := [][]byte{data}
	if strings.HasPrefix(r.contentType, "text/event-stream") {
		payloads = nil
		for line := range bytes.SplitSeq(data, []byte("\n")) {
			if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				payloads = append(payloads, bytes.TrimSpace(rest))
			}
		}
	}
	for _, p := range payloads {
		var m response
		if json.Unmarshal(p, &m) == nil && len(m.Result) > 0 && sameID(m.ID, r.id) {
			return m.Result, true
		}
	}
	return nil, false
}

func sameID(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	return json.Compact(&x, a) == nil && json.Compact(&y, b) == nil && bytes.Equal(x.Bytes(), y.Bytes())
}

type tee struct {
	io.ReadCloser
	rec *recorded
}

func (t tee) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	t.rec.write(p[:n])
	return n, err
}

type capture struct {
	next http.RoundTripper
	mu   sync.Mutex
	list []*recorded
}

func (c *capture) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || req.Body == nil {
		return c.next.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.Body = io.NopCloser(bytes.NewReader(body))
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	watched := json.Unmarshal(body, &m) == nil && m.Method == "tools/list"
	resp, err := c.next.RoundTrip(req)
	if err != nil || !watched {
		return resp, err
	}
	rec := &recorded{id: m.ID, contentType: resp.Header.Get("Content-Type")}
	resp.Body = tee{ReadCloser: resp.Body, rec: rec}
	c.mu.Lock()
	c.list = append(c.list, rec)
	c.mu.Unlock()
	return resp, nil
}

func (c *capture) take() (raw json.RawMessage, sent bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sent = len(c.list) > 0
	for len(c.list) > 0 {
		rec := c.list[0]
		c.list = c.list[1:]
		if raw, ok := rec.result(); ok {
			return raw, true
		}
	}
	return nil, sent
}

func (c *capture) reset() {
	c.mu.Lock()
	c.list = nil
	c.mu.Unlock()
}
