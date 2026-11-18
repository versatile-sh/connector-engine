package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/secrets/local"
	"github.com/agents-control/connector-engine/secrets/secretstest"
)

var ctx = context.Background()

func TestConformance(t *testing.T) {
	secretstest.Run(t, func(t *testing.T) port.SecretStore {
		s, err := local.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestKeysSurviveARestartAndDestroyIsPermanent(t *testing.T) {
	dir := t.TempDir()
	s, _ := local.Open(dir)
	a, _ := s.Seal(ctx, "ta", []byte("a"))
	b, _ := s.Seal(ctx, "tb", []byte("b"))
	if err := s.Rotate(ctx, port.KeyScope{Root: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Destroy(ctx, "ta"); err != nil {
		t.Fatal(err)
	}
	again, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := again.Open(ctx, "tb", b); err != nil || string(got) != "b" {
		t.Fatalf("after restart: %q %v", got, err)
	}
	if _, err := again.Open(ctx, "ta", a); !port.IsCode(err, port.CodeTenantRemoved) {
		t.Fatalf("a destroyed tenant stays destroyed after a restart: %v", err)
	}
}

func TestWrongRootKeyOpensNothing(t *testing.T) {
	dir := t.TempDir()
	s, _ := local.Open(dir)
	sealed, _ := s.Seal(ctx, "ta", []byte("a"))
	if err := os.Remove(filepath.Join(dir, "root.key")); err != nil {
		t.Fatal(err)
	}
	other, err := local.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(ctx, "ta", sealed); err == nil {
		t.Fatal("a new root key opened a record sealed under the old one")
	}
}

func TestRootKeyFileMustBePrivate(t *testing.T) {
	dir := t.TempDir()
	if _, err := local.Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "root.key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := local.Open(dir); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("a world-readable root key is refused: %v", err)
	}
}

func read(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func put(t *testing.T, dir, name string, b []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRootRotationSurvivesACrashBetweenItsTwoWrites(t *testing.T) {
	before := t.TempDir()
	s, err := local.Open(before)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := s.Seal(ctx, "ta", []byte("secret"))
	oldRoot, oldState := read(t, before, "root.key"), read(t, before, "keystore.json")
	if err := s.Rotate(ctx, port.KeyScope{Root: true}); err != nil {
		t.Fatal(err)
	}
	newRoot, newState := read(t, before, "root.key"), read(t, before, "keystore.json")
	if string(oldRoot) == string(newRoot) {
		t.Fatal("the root did not change")
	}
	for name, files := range map[string]map[string][]byte{
		"crash after the new root was written, before the key store":     {"root.key": oldRoot, "root.key.next": newRoot, "keystore.json": oldState},
		"crash after the key store was written, before the root renamed": {"root.key": oldRoot, "root.key.next": newRoot, "keystore.json": newState},
		"no crash": {"root.key": newRoot, "keystore.json": newState},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for f, b := range files {
				put(t, dir, f, b)
			}
			again, err := local.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := again.Open(ctx, "ta", sealed); err != nil || string(got) != "secret" {
				t.Fatalf("the old record after recovery: %q %v", got, err)
			}
			fresh, _ := again.Seal(ctx, "ta", []byte("new"))
			reopened, err := local.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := reopened.Open(ctx, "ta", fresh); err != nil || string(got) != "new" {
				t.Fatalf("a record sealed after recovery: %q %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "root.key.next")); !os.IsNotExist(err) {
				t.Fatalf("the recovery left root.key.next behind: %v", err)
			}
		})
	}
}

func TestAnUnrelatedNextRootIsRefused(t *testing.T) {
	dir := t.TempDir()
	s, _ := local.Open(dir)
	s.Seal(ctx, "ta", []byte("a"))
	other := t.TempDir()
	o, _ := local.Open(other)
	o.Seal(ctx, "tz", []byte("z"))
	put(t, dir, "root.key", read(t, other, "root.key"))
	put(t, dir, "root.key.next", read(t, other, "root.key"))
	if _, err := local.Open(dir); err == nil {
		t.Fatal("a key store no root opens must not boot")
	}
}
