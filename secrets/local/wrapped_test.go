package local_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/secrets/secretstest"
)

var unlockKey, otherKey = bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{9}, 32)

func TestWrappedConformance(t *testing.T) {
	secretstest.Run(t, func(t *testing.T) port.SecretStore {
		s, err := local.OpenWrapped(t.TempDir(), unlockKey)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func absent(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("%s is still there: %v", name, err)
	}
}

func opensRecord(t *testing.T, s port.SecretStore, sealed port.Sealed, want string) {
	t.Helper()
	if got, err := s.Open(ctx, "ta", sealed); err != nil || string(got) != want {
		t.Fatalf("open: %q %v, want %q", got, err, want)
	}
}

func TestAWrappedRootNeverRestsInPlaintextAndSurvivesARootRotation(t *testing.T) {
	dir := t.TempDir()
	s, err := local.OpenWrapped(dir, unlockKey)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := s.Seal(ctx, "ta", []byte("secret"))
	if err := s.Rotate(ctx, port.KeyScope{Root: true}); err != nil {
		t.Fatal(err)
	}
	absent(t, dir, "root.key")
	absent(t, dir, "root.key.next.wrapped")
	if local.FormOf(dir) != local.Wrapped {
		t.Fatalf("form %s", local.FormOf(dir))
	}
	again, err := local.OpenWrapped(dir, unlockKey)
	if err != nil {
		t.Fatal(err)
	}
	opensRecord(t, again, sealed, "secret")
	if _, err := local.OpenWrapped(dir, otherKey); !port.IsCode(err, port.CodePermissionDenied) {
		t.Fatalf("another unlock key: %v", err)
	}
	if _, err := local.Open(dir); !port.IsCode(err, port.CodeConflict) {
		t.Fatalf("the plaintext adapter must not mint a new root over a wrapped one: %v", err)
	}
}

func TestMigrateWrapsTheLiveRootThenRemovesThePlaintextAndAgainDoesNothing(t *testing.T) {
	dir := t.TempDir()
	plain, _ := local.Open(dir)
	sealed, _ := plain.Seal(ctx, "ta", []byte("secret"))
	s, err := local.OpenWrapped(dir, unlockKey)
	if err != nil {
		t.Fatal(err)
	}
	if s.Form() != local.Plaintext {
		t.Fatal("opening with an unlock key never migrates by itself")
	}
	for range 2 {
		if err := s.Migrate(unlockKey); err != nil {
			t.Fatal(err)
		}
	}
	absent(t, dir, "root.key")
	if !s.WrappedBy(unlockKey) || s.WrappedBy(otherKey) {
		t.Fatal("wrapped by the migrated key only")
	}
	if err := s.Migrate(otherKey); !port.IsCode(err, port.CodeConflict) {
		t.Fatalf("a second key must not rewrap silently: %v", err)
	}
	opensRecord(t, s, sealed, "secret")
	again, err := local.OpenWrapped(dir, unlockKey)
	if err != nil {
		t.Fatal(err)
	}
	opensRecord(t, again, sealed, "secret")
}

func TestAMigrationThatFailsMidwayLosesNothingAndCanRunAgain(t *testing.T) {
	dir := t.TempDir()
	plain, _ := local.Open(dir)
	sealed, _ := plain.Seal(ctx, "ta", []byte("secret"))
	s, _ := local.OpenWrapped(dir, unlockKey)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(unlockKey); err == nil {
		t.Fatal("a migration that cannot write must fail")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if s.Form() != local.Plaintext || local.FormOf(dir) != local.Plaintext {
		t.Fatal("a failed migration keeps the plaintext root")
	}
	opensRecord(t, s, sealed, "secret")
	reopened, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	opensRecord(t, reopened, sealed, "secret")
	if err := s.Migrate(unlockKey); err != nil {
		t.Fatal(err)
	}
	absent(t, dir, "root.key")
}

func TestACrashBetweenTheWrappedWriteAndThePlaintextRemovalKeepsThePlaintext(t *testing.T) {
	dir := t.TempDir()
	plain, _ := local.Open(dir)
	sealed, _ := plain.Seal(ctx, "ta", []byte("secret"))
	root := read(t, dir, "root.key")
	s, _ := local.OpenWrapped(dir, unlockKey)
	if err := s.Migrate(unlockKey); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "root.key", root)
	if local.FormOf(dir) != local.Plaintext {
		t.Fatal("a plaintext root left by an interrupted migration wins")
	}
	again, err := local.OpenWrapped(dir, unlockKey)
	if err != nil || again.Form() != local.Plaintext {
		t.Fatalf("reopen: %v", err)
	}
	opensRecord(t, again, sealed, "secret")
	if err := again.Migrate(unlockKey); err != nil {
		t.Fatal(err)
	}
	absent(t, dir, "root.key")
	opensRecord(t, again, sealed, "secret")
}

func TestRevertBringsThePlaintextFileBack(t *testing.T) {
	dir := t.TempDir()
	s, _ := local.OpenWrapped(dir, unlockKey)
	sealed, _ := s.Seal(ctx, "ta", []byte("secret"))
	for range 2 {
		if err := s.Revert(); err != nil {
			t.Fatal(err)
		}
	}
	absent(t, dir, "root.key.wrapped")
	plain, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	opensRecord(t, plain, sealed, "secret")
}

func TestKeysWithoutAnyRootAreNotGivenANewOne(t *testing.T) {
	dir := t.TempDir()
	s, _ := local.OpenWrapped(dir, unlockKey)
	s.Seal(ctx, "ta", []byte("secret"))
	if err := os.Remove(filepath.Join(dir, "root.key.wrapped")); err != nil {
		t.Fatal(err)
	}
	if _, err := local.OpenWrapped(dir, unlockKey); !port.IsCode(err, port.CodeConflict) {
		t.Fatalf("a lost root must not be replaced: %v", err)
	}
}
