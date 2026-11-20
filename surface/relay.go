package surface

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	keyBytes     = 8
	callTimeout  = 15 * time.Second
	listenBackof = 5 * time.Second
	maxBackoff   = 2 * time.Minute
)

type held struct {
	prompt Prompt
	refs   map[int]Ref
	done   bool
}

// Relay fans each requested approval of the feed out to every channel, edits every post when it settles, and decides the clicks through the desk.
type Relay struct {
	Feed     Feed
	Desk     Desk
	Channels []Channel
	Log      *slog.Logger

	mu         sync.Mutex
	byKey      map[string]*held
	byApproval map[string]*held
}

// NewKey returns a fresh prompt key of 13 lowercase base32 characters.
func NewKey() string {
	b := make([]byte, keyBytes)
	rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// Run listens on every channel and follows the feed until ctx ends.
func (r *Relay) Run(ctx context.Context) error {
	if r.Log == nil {
		r.Log = slog.New(slog.DiscardHandler)
	}
	r.mu.Lock()
	r.byKey, r.byApproval = map[string]*held{}, map[string]*held{}
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, ch := range r.Channels {
		wg.Go(func() { r.listen(ctx, ch) })
	}
	err := r.Feed.Watch(ctx, func(ev Event) { r.handle(ctx, ev) })
	wg.Wait()
	return err
}

func (r *Relay) listen(ctx context.Context, ch Channel) {
	wait := listenBackof
	for ctx.Err() == nil {
		started := time.Now()
		err := ch.Listen(ctx, r.clickOn(ch))
		if err == nil || ctx.Err() != nil {
			return
		}
		if time.Since(started) > maxBackoff {
			wait = listenBackof
		}
		r.Log.Warn("channel disconnected", "channel", ch.Kind(), "error", err, "retry_in", wait)
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		wait = min(wait*2, maxBackoff)
	}
}

func (r *Relay) clickOn(ch Channel) ClickFunc {
	return func(ctx context.Context, c Click) (Outcome, error) {
		r.mu.Lock()
		h := r.byKey[c.Key]
		live := h != nil && !h.done
		var approval string
		if live {
			approval = h.prompt.Approval
		}
		r.mu.Unlock()
		if !live {
			return Outcome{}, ErrExpired
		}
		return r.Desk.Decide(ctx, approval, c.Choice, ch.Kind(), ch.Kind()+":"+c.User)
	}
}

// Held reports the prompt the relay holds for an approval.
func (r *Relay) Held(approval string) (Prompt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.byApproval[approval]
	if h == nil {
		return Prompt{}, false
	}
	return h.prompt, true
}

// Pending returns the prompts the relay holds, for a surface that connects late.
func (r *Relay) Pending() []Prompt {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Prompt, 0, len(r.byKey))
	for _, h := range r.byKey {
		out = append(out, h.prompt)
	}
	return out
}

func (r *Relay) handle(ctx context.Context, ev Event) {
	switch ev.Kind {
	case EventRequested:
		r.request(ctx, ev.Approval)
	case EventSettled:
		r.settle(ctx, ev.Approval, ev.Outcome)
	}
}

func (r *Relay) request(ctx context.Context, approval string) {
	r.mu.Lock()
	if r.byApproval[approval] != nil {
		r.mu.Unlock()
		return
	}
	h := &held{refs: map[int]Ref{}}
	r.byApproval[approval] = h
	r.mu.Unlock()
	p, err := r.Desk.Prompt(ctx, approval)
	if err != nil {
		if !errors.Is(err, ErrSettled) {
			r.Log.Warn("approval prompt", "approval", approval, "error", err)
		}
		r.mu.Lock()
		delete(r.byApproval, approval)
		r.mu.Unlock()
		return
	}
	p.Key = NewKey()
	r.mu.Lock()
	h.prompt = p
	r.byKey[p.Key] = h
	r.mu.Unlock()
	refs := r.each(ctx, func(cctx context.Context, i int, ch Channel) (Ref, error) { return ch.Notify(cctx, p) })
	r.mu.Lock()
	h.refs = refs
	r.mu.Unlock()
}

func (r *Relay) settle(ctx context.Context, approval string, o Outcome) {
	r.mu.Lock()
	h := r.byApproval[approval]
	if h == nil || h.done {
		r.mu.Unlock()
		return
	}
	h.done = true
	refs, p := h.refs, h.prompt
	delete(r.byApproval, approval)
	delete(r.byKey, p.Key)
	r.mu.Unlock()
	r.each(ctx, func(cctx context.Context, i int, ch Channel) (Ref, error) {
		ref, ok := refs[i]
		if !ok {
			return "", nil
		}
		return ref, ch.Settle(cctx, ref, p, o)
	})
}

func (r *Relay) each(ctx context.Context, fn func(context.Context, int, Channel) (Ref, error)) map[int]Ref {
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := map[int]Ref{}
	for i, ch := range r.Channels {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, callTimeout)
			defer cancel()
			ref, err := fn(cctx, i, ch)
			if err != nil {
				r.Log.Warn("channel call failed", "channel", ch.Kind(), "error", err)
				return
			}
			if ref != "" {
				mu.Lock()
				out[i] = ref
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return out
}
