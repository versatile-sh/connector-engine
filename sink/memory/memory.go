// Package memory is an EventSink fake that records what it was given and fails on demand.
package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/versatile-sh/connector-engine/port"
)

// Sink records delivered events; Fail, when set, decides each delivery.
type Sink struct {
	mu   sync.Mutex
	got  []port.Event
	fail func([]port.Event) error
}

var _ port.EventSink = (*Sink)(nil)

// New returns a sink that accepts everything.
func New() *Sink { return &Sink{} }

// FailWith makes every later Deliver return fn's answer without recording; a nil fn accepts again.
func (s *Sink) FailWith(fn func([]port.Event) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fn
}

// Deliver records the events unless the failure hook refuses them.
func (s *Sink) Deliver(_ context.Context, events []port.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		if err := s.fail(events); err != nil {
			return err
		}
	}
	s.got = append(s.got, events...)
	return nil
}

// Events returns a copy of what was delivered, in order.
func (s *Sink) Events() []port.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.got)
}
