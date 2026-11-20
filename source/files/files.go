// Package files is the ListingSource over a directory of manifest files in YAML or JSON.
package files

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

// Registry is the name recorded as the provenance of a file import.
const Registry = "files"

// New returns a source that reads every .json, .yaml and .yml file of dir as one manifest, in name order; now may be nil.
func New(dir string, now func() time.Time) *importer.Static {
	return &importer.Static{Registry: Registry, Now: now, Load: func(context.Context) ([]port.Manifest, error) { return load(dir) }}
}

func load(dir string) ([]port.Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, port.Errorf(port.CodeNotFound, "listing directory: %v", err)
	}
	var out []port.Manifest
	for _, e := range entries {
		if e.IsDir() || !slices.Contains([]string{".json", ".yaml", ".yml"}, strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, port.Errorf(port.CodeUnavailable, "%s: %v", e.Name(), err)
		}
		var m port.Manifest
		if err := importer.Decode(data, &m); err != nil {
			return nil, port.Errorf(port.CodeInvalidManifest, "%s: %v", e.Name(), err)
		}
		out = append(out, m)
	}
	return out, nil
}
