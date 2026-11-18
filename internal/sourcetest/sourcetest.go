// Package sourcetest checks the rules every ListingSource adapter shares.
package sourcetest

import (
	"context"
	"slices"
	"testing"

	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/port"
)

// Import pages through src and checks each manifest: unreviewed, provenance set, installation owner, no lint finding unless excused in incomplete; it returns them by name.
func Import(t *testing.T, src port.ListingSource, incomplete ...string) map[string]port.Manifest {
	t.Helper()
	got := map[string]port.Manifest{}
	for cursor, pages := "", 0; ; pages++ {
		if pages > 100 {
			t.Fatal("the source never ends its pages")
		}
		page, err := src.List(context.Background(), cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Manifests {
			if f := listing.Lint(m, false); len(f) > 0 && !slices.Contains(incomplete, m.Name) {
				t.Fatalf("%s %s: %+v", m.Name, m.Version, f)
			}
			if m.TrustLevel != port.TrustUnreviewed || m.Review != nil || m.Owner != port.Installation {
				t.Fatalf("%s is not an unreviewed installation import: %+v", m.Name, m)
			}
			if m.Source == nil || m.Source.Registry != src.Name() || m.Source.UpstreamName != m.Name || m.Source.UpstreamVersion != m.Version || m.Source.ImportedAt.IsZero() {
				t.Fatalf("%s provenance %+v", m.Name, m.Source)
			}
			got[m.Name] = m
		}
		if cursor = page.Next; cursor == "" {
			break
		}
	}
	for name, m := range got {
		fetched, err := src.Fetch(context.Background(), name, m.Version)
		if err != nil || fetched.Name != name || fetched.Version != m.Version {
			t.Fatalf("fetch %s %s: %v", name, m.Version, err)
		}
	}
	if _, err := src.Fetch(context.Background(), "no.such/listing", ""); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("fetching an absent listing: %v", err)
	}
	return got
}
