// Package handles keeps the started instances of a runtime adapter under opaque handles.
package handles

import (
	"fmt"
	"sync"

	"github.com/agents-control/connector-engine/port"
)

// Registry maps handles to the instances an adapter started.
type Registry[T any] struct {
	mu     sync.Mutex
	prefix string
	next   int
	items  map[port.Handle]T
}

// New returns an empty registry whose handles start with prefix.
func New[T any](prefix string) *Registry[T] {
	return &Registry[T]{prefix: prefix, items: map[port.Handle]T{}}
}

// Add stores v and returns its handle.
func (r *Registry[T]) Add(v T) port.Handle {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	h := port.Handle(fmt.Sprintf("%s-%d", r.prefix, r.next))
	r.items[h] = v
	return h
}

// Get returns the instance of h, or a not_found error.
func (r *Registry[T]) Get(h port.Handle) (T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.items[h]
	if !ok {
		return v, port.Errorf(port.CodeNotFound, "no started instance %q", h)
	}
	return v, nil
}

// Remove deletes h and returns its instance when it existed.
func (r *Registry[T]) Remove(h port.Handle) (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.items[h]
	delete(r.items, h)
	return v, ok
}
