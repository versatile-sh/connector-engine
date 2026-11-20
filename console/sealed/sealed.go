// Package sealed keeps the console's own secrets, such as notifier tokens, sealed by a port.SecretStore so the state file never holds them in plaintext.
package sealed

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/hostkey"
	"github.com/versatile-sh/connector-engine/secrets/local"
)

// Tenant is the key scope of every console secret in its SecretStore.
const Tenant port.TenantID = "console"

// ErrLocked is returned before the box has a SecretStore, while its key waits to be delivered.
var ErrLocked = errors.New("the console's secrets are locked until the host delivers the key")

// Box seals and opens console secrets; it is locked until Unlock gives it a store, and a nil Box stays locked.
type Box struct {
	store atomic.Pointer[port.SecretStore]
	mu    sync.Mutex
	ready chan struct{}
	once  sync.Once
}

// Unlock makes the box use store from now on and closes Ready.
func (b *Box) Unlock(store port.SecretStore) {
	b.store.Store(&store)
	b.once.Do(func() { close(b.readyChan()) })
}

// Ready returns a channel closed once the box has a store, so whatever needs a secret at start can wait for the host to deliver the key.
func (b *Box) Ready() <-chan struct{} {
	if b == nil {
		return nil
	}
	return b.readyChan()
}

func (b *Box) readyChan() chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ready == nil {
		b.ready = make(chan struct{})
	}
	return b.ready
}

// Locked reports whether the box has no store yet.
func (b *Box) Locked() bool { return b.load() == nil }

func (b *Box) load() *port.SecretStore {
	if b == nil {
		return nil
	}
	return b.store.Load()
}

// Protection reports how the console's root key rests: locked, plaintext, wrapped, or empty for a store that keeps no key file.
func (b *Box) Protection() string {
	s := b.load()
	if s == nil {
		return hostkey.Locked
	}
	if f, ok := (*s).(interface{ Form() local.Form }); ok {
		return string(f.Form())
	}
	return ""
}

// Seal returns plaintext sealed under the console's key.
func (b *Box) Seal(ctx context.Context, plaintext string) (port.Sealed, error) {
	s := b.load()
	if s == nil {
		return port.Sealed{}, ErrLocked
	}
	return (*s).Seal(ctx, Tenant, []byte(plaintext))
}

// Open returns the plaintext of a value Seal made.
func (b *Box) Open(ctx context.Context, v port.Sealed) (string, error) {
	s := b.load()
	if s == nil {
		return "", ErrLocked
	}
	plaintext, err := (*s).Open(ctx, Tenant, v)
	return string(plaintext), err
}
