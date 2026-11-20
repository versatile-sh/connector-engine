package sqlite_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/store/sqlite"
)

func TestTheDatabaseAndItsWALFilesAreOnlyTheOwners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no unix modes")
	}
	path := filepath.Join(t.TempDir(), "store.db")
	if err := os.WriteFile(path+"-wal", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, path)
	if err := s.Write(ctx, port.Scope{Tenant: "ta"}, func(tx port.Tx) error { return tx.PutPolicy(port.CatalogPolicy{Tenant: "ta", Workspace: "w"}) }); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s is %v", filepath.Base(f), info.Mode().Perm())
		}
	}
}

func TestAPathTheDriverWouldSplitIsRefused(t *testing.T) {
	if _, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "a?mode=memory", "store.db")); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("a path with a ?: %v", err)
	}
}
