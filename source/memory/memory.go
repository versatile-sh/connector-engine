// Package memory is a ListingSource over a list held in memory, serving manifests exactly as given, for tests of the layers above it.
package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

// Source serves the manifests last set.
type Source struct {
	*importer.Static
	mu        sync.Mutex
	manifests []port.Manifest
}

// New returns a source named name with the given manifests and pages of pageSize (100 when zero).
func New(name string, pageSize int, manifests ...port.Manifest) *Source {
	s := &Source{manifests: manifests}
	s.Static = &importer.Static{Registry: name, PageSize: pageSize, Keep: true, Load: func(context.Context) ([]port.Manifest, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return slices.Clone(s.manifests), nil
	}}
	return s
}

// Set replaces the manifests the source serves.
func (s *Source) Set(manifests ...port.Manifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifests = manifests
}
