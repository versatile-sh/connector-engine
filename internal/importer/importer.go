// Package importer holds what every ListingSource adapter shares: provenance, the unreviewed rule, paging and YAML decoding.
package importer

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/agents-control/connector-engine/port"
)

const defaultPageSize = 100

// Imported marks a manifest as an import of registry: provenance set, installation owner, unreviewed, no review and no declared class.
func Imported(m port.Manifest, registry string, now time.Time) port.Manifest {
	m.Source = &port.SourceInfo{Registry: registry, UpstreamName: m.Name, UpstreamVersion: m.Version, ImportedAt: now}
	m.Owner = port.Installation
	m.TrustLevel = port.TrustUnreviewed
	m.Review = nil
	m.Functions = slices.Clone(m.Functions)
	for i := range m.Functions {
		m.Functions[i].Class, m.Functions[i].ClassSource = port.ClassNone, ""
	}
	return m
}

// YAMLToJSON converts a YAML or JSON document to JSON.
func YAMLToJSON(data []byte) ([]byte, error) {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "yaml: %v", err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "yaml: %v", err)
	}
	return out, nil
}

// Decode reads a YAML or JSON document into out through its json tags.
func Decode(data []byte, out any) error {
	b, err := YAMLToJSON(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return port.Errorf(port.CodeInvalidManifest, "%v", err)
	}
	return nil
}

// Static is a ListingSource over a list that Load rebuilds on every call; the cursor is an offset.
type Static struct {
	Registry string
	Load     func(ctx context.Context) ([]port.Manifest, error)
	PageSize int
	Now      func() time.Time
	Keep     bool
}

// Name is the registry name recorded in the provenance of every import.
func (s *Static) Name() string { return s.Registry }

func (s *Static) prepare(ms []port.Manifest) []port.Manifest {
	if s.Keep {
		return ms
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	out := make([]port.Manifest, len(ms))
	for i, m := range ms {
		out[i] = Imported(m, s.Registry, now())
	}
	return out
}

// List returns one page of the loaded manifests.
func (s *Static) List(ctx context.Context, cursor string) (port.ListingPage, error) {
	all, err := s.Load(ctx)
	if err != nil {
		return port.ListingPage{}, err
	}
	start := 0
	if cursor != "" {
		if start, err = strconv.Atoi(cursor); err != nil || start < 0 {
			return port.ListingPage{}, port.Errorf(port.CodeInvalidArgument, "cursor %q is not an offset", cursor)
		}
	}
	size := s.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	start = min(start, len(all))
	end := min(start+size, len(all))
	page := port.ListingPage{Manifests: s.prepare(all[start:end])}
	if end < len(all) {
		page.Next = strconv.Itoa(end)
	}
	return page, nil
}

// Fetch returns the manifest of a name and version; an empty version or latest means the last one listed.
func (s *Static) Fetch(ctx context.Context, name, version string) (port.Manifest, error) {
	all, err := s.Load(ctx)
	if err != nil {
		return port.Manifest{}, err
	}
	for _, m := range slices.Backward(all) {
		if m.Name == name && (version == "" || version == "latest" || m.Version == version) {
			return s.prepare([]port.Manifest{m})[0], nil
		}
	}
	return port.Manifest{}, port.Errorf(port.CodeNotFound, "%s has no listing %s %s", s.Registry, name, version)
}
