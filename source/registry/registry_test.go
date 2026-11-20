package registry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/sourcetest"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/registry"
)

func serve(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		mu.Unlock()
		file := "testdata/page1.json"
		switch {
		case r.URL.EscapedPath() == "/v0.1/servers/io.github.acme%2Fnotes/versions/2.0.1":
			file = "testdata/version-notes.json"
		case r.URL.EscapedPath() == "/v0.1/servers/com.example%2Ffiles/versions/0.4.0":
			file = "testdata/version-files.json"
		case r.URL.Path != "/v0.1/servers":
			http.NotFound(w, r)
			return
		case r.URL.Query().Get("cursor") == "page2":
			file = "testdata/page2.json"
		}
		b, _ := os.ReadFile(file)
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

func TestImportsTheRegistryAsUnreviewed(t *testing.T) {
	srv, seen := serve(t)
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	src := registry.New(registry.Config{Name: "official", BaseURL: srv.URL, UpdatedSince: since, PageSize: 2, NamespaceVerified: true, Guard: egress.Guard{AllowPrivate: true}})
	got := sourcetest.Import(t, src, "com.example/files")

	if len(got) != 2 {
		t.Fatalf("imported %d: deleted and empty servers are left out", len(got))
	}
	notes := got["io.github.acme/notes"]
	if notes.Kind != port.KindMCPRemote || notes.Remote.URL != "https://notes.acme.example/mcp" || notes.Remote.Transport != "streamable-http" ||
		notes.Title != "Acme Notes" || notes.Icon == "" || len(notes.Config) != 1 || !notes.Config[0].Sensitive {
		t.Fatalf("remote mapping: %+v", notes)
	}
	if trust := listing.Trust(notes, listing.Evidence{}); trust != port.TrustNamespaceVerified || notes.TrustLevel != port.TrustUnreviewed {
		t.Fatalf("namespace proof: computed %s, stored %s", trust, notes.TrustLevel)
	}
	files := got["com.example/files"]
	if files.Kind != port.KindMCPPackage || files.Artifact.Type != "mcpb" || files.Artifact.Digest != "abc123" || files.Config[0].Key != "FILES_ROOT" {
		t.Fatalf("package mapping: %+v", files)
	}
	if first := seen()[0]; !strings.Contains(first, "updated_since=2026-11-01T00%3A00%3A00Z") || !strings.Contains(first, "limit=2") || !strings.Contains(first, "version=latest") {
		t.Fatalf("list query %s", first)
	}
}

func TestFetchAVersion(t *testing.T) {
	srv, seen := serve(t)
	src := registry.New(registry.Config{BaseURL: srv.URL, Guard: egress.Guard{AllowPrivate: true}})
	m, err := src.Fetch(context.Background(), "io.github.acme/notes", "2.0.1")
	if err != nil || m.Version != "2.0.1" || m.TrustLevel != port.TrustUnreviewed || m.Publisher.VerifiedBy != "" {
		t.Fatalf("fetch: %+v %v", m, err)
	}
	if got := seen(); got[0] != "/v0.1/servers/io.github.acme%2Fnotes/versions/2.0.1?" {
		t.Fatalf("path %v", got)
	}
	if _, err := src.Fetch(context.Background(), "io.github.acme/other", "1"); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("absent: %v", err)
	}
}
