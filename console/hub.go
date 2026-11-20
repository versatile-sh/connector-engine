package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/surface"
)

const hubKeepAlive = 25 * time.Second

// HubEvent is one message of the paired helpers' stream: a prompt with its single-use link, or the outcome of one.
type HubEvent struct {
	Type     string           `json:"type"`
	Approval string           `json:"approval"`
	Prompt   *surface.Prompt  `json:"prompt,omitempty"`
	Link     string           `json:"link,omitempty"`
	Outcome  *surface.Outcome `json:"outcome,omitempty"`
}

type hubSub struct {
	device Device
	events chan HubEvent
	gone   chan struct{}
}

type deviceHub struct {
	mu   sync.Mutex
	subs map[*hubSub]struct{}
}

func (h *deviceHub) add(d Device) *hubSub {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[*hubSub]struct{}{}
	}
	s := &hubSub{device: d, events: make(chan HubEvent, 32), gone: make(chan struct{})}
	h.subs[s] = struct{}{}
	return s
}

func (h *deviceHub) remove(s *hubSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[s]; ok {
		delete(h.subs, s)
		close(s.gone)
	}
}

func (h *deviceHub) drop(device string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.device.ID == device {
			delete(h.subs, s)
			close(s.gone)
		}
	}
}

func (h *deviceHub) each(fn func(*hubSub)) {
	h.mu.Lock()
	subs := make([]*hubSub, 0, len(h.subs))
	for s := range h.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()
	for _, s := range subs {
		fn(s)
	}
}

func (s *hubSub) send(ev HubEvent) {
	select {
	case s.events <- ev:
	default:
	}
}

type hubChannel struct{ c *Console }

func (hubChannel) Kind() string { return "desktop" }

func (h hubChannel) prompt(s *hubSub, p surface.Prompt) {
	link, err := h.c.LinkToken(p.Approval, s.device.ID, p.ExpiresAt)
	if err != nil {
		h.c.logger().Warn("link token", "error", err)
		return
	}
	s.send(HubEvent{Type: "prompt", Approval: p.Approval, Prompt: &p, Link: p.ConsoleURL + "?t=" + link})
}

func (h hubChannel) Notify(_ context.Context, p surface.Prompt) (surface.Ref, error) {
	h.c.surf.hub.each(func(s *hubSub) { h.prompt(s, p) })
	return "hub", nil
}

func (h hubChannel) Settle(_ context.Context, _ surface.Ref, p surface.Prompt, o surface.Outcome) error {
	h.c.surf.hub.each(func(s *hubSub) { s.send(HubEvent{Type: "settled", Approval: p.Approval, Outcome: &o}) })
	return nil
}

func (hubChannel) Listen(context.Context, surface.ClickFunc) error { return nil }

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (c *Console) notifyStream(w http.ResponseWriter, r *http.Request) {
	scheme, key, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	d, ok := c.State.DeviceByKey(keyHash(strings.TrimSpace(key)))
	if !strings.EqualFold(scheme, "Bearer") || !ok {
		writeError(w, http.StatusUnauthorized, "pair this computer in Notifications first")
		return
	}
	d.LastSeen = time.Now().UTC().Truncate(time.Second)
	_ = c.State.PutDevice(d)
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()
	sub := c.surf.hub.add(d)
	defer c.surf.hub.remove(sub)
	h := hubChannel{c: c}
	for _, p := range c.surf.pending() {
		h.prompt(sub, p)
	}
	tick := time.NewTicker(hubKeepAlive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.gone:
			return
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case ev := <-sub.events:
			raw, _ := json.Marshal(ev)
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, raw); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
