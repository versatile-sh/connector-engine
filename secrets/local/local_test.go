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
