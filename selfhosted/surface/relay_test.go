package surface_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

type feed struct{ events chan surface.Event }

func (f feed) Watch(ctx context.Context, fn func(surface.Event)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-f.events:
			fn(ev)
		}
	}
}

type desk struct {
	mu      sync.Mutex
	decided []string
}

func (d *desk) Prompt(_ context.Context, id string) (surface.Prompt, error) {
	return surface.Prompt{Approval: id, Connector: "Notes", Function: "create", App: "claude-code", Choices: []surface.Choice{surface.ChoiceOnce, surface.ChoiceDeny}}, nil
}

func (d *desk) Decide(_ context.Context, id string, c surface.Choice, channel, by string) (surface.Outcome, error) {
	d.mu.Lock()
	d.decided = append(d.decided, id+"/"+string(c)+"/"+channel+"/"+by)
	d.mu.Unlock()
	return surface.Outcome{State: surface.StateApproved, By: by, Channel: channel}, nil
}

type channel struct {
	mu       sync.Mutex
	notified chan surface.Prompt
	settled  chan surface.Outcome
	click    surface.ClickFunc
	ready    chan struct{}
}

func newChannel() *channel {
	return &channel{notified: make(chan surface.Prompt, 4), settled: make(chan surface.Outcome, 4), ready: make(chan struct{})}
}

func (c *channel) Kind() string { return "fake" }

func (c *channel) Notify(_ context.Context, p surface.Prompt) (surface.Ref, error) {
	c.notified <- p
	return surface.Ref("msg-" + p.Approval), nil
}

func (c *channel) Settle(_ context.Context, ref surface.Ref, _ surface.Prompt, o surface.Outcome) error {
	if ref == "" {
		return errors.New("no ref")
	}
	c.settled <- o
	return nil
}

func (c *channel) Listen(ctx context.Context, click surface.ClickFunc) error {
	c.mu.Lock()
	c.click = click
	c.mu.Unlock()
	close(c.ready)
	<-ctx.Done()
	return nil
}

func recv[T any](t *testing.T, ch chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	var zero T
	return zero
}

func TestTheRelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f, d, ch := feed{events: make(chan surface.Event, 4)}, &desk{}, newChannel()
	r := &surface.Relay{Feed: f, Desk: d, Channels: []surface.Channel{ch}}
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-ch.ready

	t.Run("a requested approval is notified once, with a fresh key", func(t *testing.T) {
		f.events <- surface.Event{Kind: surface.EventRequested, Approval: "a1"}
		f.events <- surface.Event{Kind: surface.EventRequested, Approval: "a1"}
		p := recv(t, ch.notified)
		if p.Approval != "a1" || len(p.Key) != 13 {
			t.Fatalf("prompt %+v", p)
		}
		select {
		case again := <-ch.notified:
			t.Fatalf("notified twice: %+v", again)
		case <-time.After(100 * time.Millisecond):
		}

		t.Run("a click on its key decides through the desk with the channel and user", func(t *testing.T) {
			o, err := ch.click(ctx, surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "42"})
			if err != nil || o.State != surface.StateApproved {
				t.Fatalf("click: %+v %v", o, err)
			}
			d.mu.Lock()
			got := d.decided
			d.mu.Unlock()
			if len(got) != 1 || got[0] != "a1/once/fake/fake:42" {
				t.Fatalf("decided %v", got)
			}
		})

		t.Run("settling edits the post, then a late click on the key expires", func(t *testing.T) {
			f.events <- surface.Event{Kind: surface.EventSettled, Approval: "a1", Outcome: surface.Outcome{State: surface.StateDenied, By: "owner"}}
			if o := recv(t, ch.settled); o.State != surface.StateDenied {
				t.Fatalf("settled %+v", o)
			}
			if _, err := ch.click(ctx, surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "42"}); !errors.Is(err, surface.ErrExpired) {
				t.Fatalf("late click: %v", err)
			}
		})
	})

	t.Run("an unknown key expires", func(t *testing.T) {
		if _, err := ch.click(ctx, surface.Click{Key: "nope", Choice: surface.ChoiceOnce}); !errors.Is(err, surface.ErrExpired) {
			t.Fatalf("unknown key: %v", err)
		}
	})

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestButtonIDsRoundTripAndFitTelegram(t *testing.T) {
	key := surface.NewKey()
	for _, c := range []surface.Choice{surface.ChoiceOnce, surface.ChoiceSession, surface.ChoiceAlways, surface.ChoiceDeny} {
		id := surface.ButtonID(c, key)
		got, k, ok := surface.ParseButton(id)
		if !ok || got != c || k != key || len(id) > 64 {
			t.Fatalf("%s: %q %q %v", c, got, k, ok)
		}
	}
	if _, _, ok := surface.ParseButton("x:abc"); ok {
		t.Fatal("an unknown letter parsed")
	}
}
