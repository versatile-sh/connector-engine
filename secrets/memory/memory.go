// Package memory is a SecretStore fake that keeps a random root key and the wrapped keys in the process.
package memory

import (
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/internal/ring"
)

// New returns an empty store with a fresh root key.
func New() port.SecretStore {
	root, err := ring.NewRootKey()
	if err != nil {
		panic(err)
	}
	r, err := ring.New(root, ring.State{}, nil)
	if err != nil {
		panic(err)
	}
	return r
}
