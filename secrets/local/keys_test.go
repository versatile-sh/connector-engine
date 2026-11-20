package local_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/versatile-sh/connector-engine/secrets/local"
)

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestRestoredKeysOpenTheOldRecordsUnderTheNewUnlockKey(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	src, err := local.OpenWrapped(from, unlockKey)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := src.Seal(ctx, "ta", []byte("provider token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "local-issuer.json"), []byte(`{"keys":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := src.Keys()
	if err != nil {
		t.Fatal(err)
	}
	if string(keys.Files["local-issuer.json"]) != `{"keys":[]}` || len(keys.Root) != 32 {
		t.Fatalf("keys: %d-byte root, files %v", len(keys.Root), keys.Files)
	}

	dst, err := local.OpenWrapped(to, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Seal(ctx, "tb", []byte("kept")); err != nil {
		t.Fatal(err)
	}
	before := names(t, to)
	failed := errors.New("the database refused")
	if err := dst.Restore(keys, func() error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("a failed restore: %v", err)
	}
	if got := names(t, to); !slices.Equal(got, before) {
		t.Fatalf("a failed restore left %v, had %v", got, before)
	}
	bad := keys
	bad.Root = otherKey
	if err := dst.Restore(bad, func() error { return nil }); err == nil {
		t.Fatal("a root that does not open the state was restored")
	}

	if err := dst.Restore(keys, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Seal(ctx, "tc", []byte("x")); err == nil {
		t.Fatal("a restored store persisted a new key before it was opened again")
	}
	if local.FormOf(to) != local.Wrapped {
		t.Fatalf("the restored root rests %s", local.FormOf(to))
	}
	if _, err := local.OpenWrapped(to, unlockKey); err == nil {
		t.Fatal("the old unlock key opens the restored store")
	}
	again, err := local.OpenWrapped(to, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	opensRecord(t, again, sealed, "provider token")
	if raw, _ := os.ReadFile(filepath.Join(to, "local-issuer.json")); string(raw) != `{"keys":[]}` {
		t.Fatalf("local-issuer.json: %q", raw)
	}
	for _, n := range names(t, to) {
		if filepath.Ext(n) == ".restore" || filepath.Ext(n) == ".next" {
			t.Fatalf("%s left behind", n)
		}
	}
}
