package console

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

// ChannelFactory builds an approval channel of a kind from its stored settings.
type ChannelFactory func(kind string, settings map[string]string) (surface.Channel, error)

type channelStatus struct {
	State string
	Error string
}

type surfaces struct {
	mu      sync.Mutex
	always  map[string]alwaysMark
	shown   map[string]presented
	status  map[string]channelStatus
	changed chan struct{}
	relay   *surface.Relay
	links   spentSet
	hub     deviceHub
	el      elicitState
}

type alwaysMark struct {
	at     time.Time
	failed string
}

func (s *surfaces) markAlways(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.always == nil {
		s.always = map[string]alwaysMark{}
	}
	now := time.Now()
	expire(s.always, now, func(v alwaysMark) time.Time { return v.at.Add(time.Hour) })
	s.always[id] = alwaysMark{at: now}
}

func (s *surfaces) failAlways(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.always[id]; ok {
		m.failed = err.Error()
		s.always[id] = m
	}
}

func (s *surfaces) alwaysFailed(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.always[id].failed
}

const presentTTL = 5 * time.Minute

func (s *surfaces) presented(key string) (presented, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr, ok := s.shown[key]
	return pr, ok && time.Since(pr.at) < presentTTL
}

func (s *surfaces) present(key string, pr presented) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shown == nil || len(s.shown) > 256 {
		s.shown = map[string]presented{}
	}
	s.shown[key] = pr
}

func (s *surfaces) wasAlways(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.always[id]
	return ok && m.failed == ""
}

func (s *surfaces) spent(nonce string) bool { return s.links.has(nonce) }

func (s *surfaces) wakeChan() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changed == nil {
		s.changed = make(chan struct{}, 1)
	}
	return s.changed
}

func (s *surfaces) wake() {
	select {
	case s.wakeChan() <- struct{}{}:
	default:
	}
}

func (s *surfaces) setStatus(id, state, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == nil {
		s.status = map[string]channelStatus{}
	}
	s.status[id] = channelStatus{State: state, Error: msg}
}

func (s *surfaces) statusOf(id string) channelStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status[id]
}

func (s *surfaces) setRelay(r *surface.Relay) {
	s.mu.Lock()
	s.relay = r
	s.mu.Unlock()
}

func (s *surfaces) pending() []surface.Prompt {
	s.mu.Lock()
	r := s.relay
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return r.Pending()
}

func (s *surfaces) holds(approval string) bool {
	s.mu.Lock()
	r := s.relay
	s.mu.Unlock()
	if r == nil {
		return false
	}
	_, ok := r.Held(approval)
	return ok
}

var interactiveKinds = map[string]bool{"discord": true, "slack": true, "telegram": true}

type tracked struct {
	surface.Channel
	id      string
	secrets []string
	surf    *surfaces
}

func (t tracked) scrub(err error) string {
	msg := err.Error()
	for _, s := range t.secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	return msg
}

func (t tracked) Notify(ctx context.Context, p surface.Prompt) (surface.Ref, error) {
	ref, err := t.Channel.Notify(ctx, p)
	if err != nil {
		t.surf.setStatus(t.id, "error", t.scrub(err))
		return ref, errors.New(t.scrub(err))
	}
	return ref, nil
}

func (t tracked) Listen(ctx context.Context, click surface.ClickFunc) error {
	if interactiveKinds[t.Kind()] {
		t.surf.setStatus(t.id, "connected", "")
	}
	err := t.Channel.Listen(ctx, click)
	if err != nil && ctx.Err() == nil {
		t.surf.setStatus(t.id, "error", t.scrub(err))
		return errors.New(t.scrub(err))
	}
	return nil
}

func (c *Console) publicBase() string { return strings.TrimSuffix(c.MCPURL, "/mcp") }

func (c *Console) factory() ChannelFactory {
	if c.NewChannel != nil {
		return c.NewChannel
	}
	return DefaultChannel
}

func (c *Console) buildChannels(ctx context.Context) []surface.Channel {
	var out []surface.Channel
	for _, sc := range c.State.Channels() {
		settings, err := c.settingsOf(ctx, sc)
		t := tracked{id: sc.ID, secrets: secretValues(sc.Kind, settings), surf: &c.surf}
		var ch surface.Channel
		if err == nil {
			ch, err = c.factory()(sc.Kind, settings)
		}
		if err != nil {
			c.surf.setStatus(sc.ID, "error", t.scrub(err))
			continue
		}
		t.Channel = ch
		if interactiveKinds[sc.Kind] {
			c.surf.setStatus(sc.ID, "connecting", "")
		} else {
			c.surf.setStatus(sc.ID, "notify_only", "")
		}
		out = append(out, t)
	}
	if len(c.State.Devices()) > 0 {
		out = append(out, hubChannel{c: c})
	}
	return out
}

// RunNotifications runs the approval relay while a channel or a paired device is configured and rebuilds it when the owner changes either or the console's secrets unlock.
func (c *Console) RunNotifications(ctx context.Context) {
	changed := c.surf.wakeChan()
	var unlocked <-chan struct{}
	if c.Secrets.Locked() {
		unlocked = c.Secrets.Ready()
	}
	for ctx.Err() == nil {
		if !c.Secrets.Locked() {
			if _, err := c.linkKey(); err != nil {
				c.logger().Warn("approval link key", "error", err)
			}
		}
		chans := c.buildChannels(ctx)
		rctx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		if len(chans) > 0 {
			r := &surface.Relay{Feed: &engineFeed{c: c}, Desk: desk{c: c}, Channels: chans, Log: c.logger()}
			c.surf.setRelay(r)
			go func() {
				defer close(done)
				if err := r.Run(rctx); err != nil {
					c.logger().Warn("approval relay stopped", "error", err)
				}
			}()
		} else {
			c.surf.setRelay(nil)
			close(done)
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-unlocked:
			unlocked = nil
		}
		cancel()
		<-done
	}
}

type desk struct{ c *Console }

func (d desk) Prompt(ctx context.Context, id string) (surface.Prompt, error) {
	v, err := d.c.Approval(ctx, id)
	if err != nil {
		return surface.Prompt{}, err
	}
	if v.State != string(port.ApprovalPending) {
		return surface.Prompt{}, surface.ErrSettled
	}
	return surface.Prompt{Approval: v.ID, Connector: v.Connector.Title, ConnectorIcon: v.Connector.Icon, Function: v.Function, FunctionTitle: v.FunctionTitle,
		Kind: string(v.Kind), App: v.App, ExpiresAt: v.ExpiresAt, Choices: v.Choices, SessionUntil: v.SessionUntil, ConsoleURL: d.c.publicBase() + "/approve/" + v.ID}, nil
}

func (d desk) Decide(ctx context.Context, id string, choice surface.Choice, channel, by string) (surface.Outcome, error) {
	ctx, err := d.c.asKeyOwner(ctx, id)
	if err != nil {
		return surface.Outcome{}, err
	}
	v, _, err := d.c.DecideChoice(ctx, id, choice, channel, by)
	if err != nil {
		return surface.Outcome{}, err
	}
	return d.c.Outcome(v), nil
}
