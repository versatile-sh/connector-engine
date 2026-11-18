package files_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/internal/sourcetest"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/files"
)

func TestImportsEveryFileAsUnreviewed(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	got := sourcetest.Import(t, files.New("testdata", func() time.Time { return now }))
	if len(got) != 2 {
		t.Fatalf("imported %d, want the yaml and the json file", len(got))
	}
	notes := got["com.example/notes"]
	if !notes.Source.ImportedAt.Equal(now) || notes.Functions[0].Class != port.ClassNone || notes.Functions[0].ClassSource != "" {
		t.Fatalf("a declared trust or class survived the import: %+v", notes)
	}
	if got["com.example/crm"].REST.Functions[0].Path != "/contacts/{id}" {
		t.Fatalf("json file: %+v", got["com.example/crm"])
	}
}

func TestPagesAndBadFiles(t *testing.T) {
	src := files.New("testdata", nil)
	src.PageSize = 1
	page, err := src.List(context.Background(), "")
	if err != nil || len(page.Manifests) != 1 || page.Next != "1" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	if page, _ = src.List(context.Background(), page.Next); len(page.Manifests) != 1 || page.Next != "" {
		t.Fatalf("last page: %+v", page)
	}
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "bad.yaml"), []byte("name: [unclosed"), 0o600)
	if _, err := files.New(bad, nil).List(context.Background(), ""); err == nil {
		t.Fatal("a malformed file must be reported")
	}
	if _, err := files.New(filepath.Join(bad, "missing"), nil).List(context.Background(), ""); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("missing directory: %v", err)
	}
}
