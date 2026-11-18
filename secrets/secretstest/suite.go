// Package secretstest is the conformance suite every port.SecretStore adapter must pass.
package secretstest

import (
	"bytes"
	"context"
	"testing"

	"github.com/agents-control/connector-engine/port"
)

var ctx = context.Background()

// Run executes the whole suite, each case on a fresh store from newStore.
func Run(t *testing.T, newStore func(t *testing.T) port.SecretStore) {
	cases := map[string]func(*testing.T, port.SecretStore){
		"round trip":             roundTrip,
		"tenants are isolated":   isolation,
		"tampering is detected":  tampering,
		"rotate tenant":          rotateTenant,
		"rotate root":            rotateRoot,
		"destroy shreds records": destroy,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) { fn(t, newStore(t)) })
	}
}

func seal(t *testing.T, s port.SecretStore, tenant port.TenantID, text string) port.Sealed {
	t.Helper()
	sealed, err := s.Seal(ctx, tenant, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func opens(t *testing.T, s port.SecretStore, tenant port.TenantID, sealed port.Sealed, want string) {
	t.Helper()
	got, err := s.Open(ctx, tenant, sealed)
	if err != nil || string(got) != want {
		t.Fatalf("open: %q %v, want %q", got, err, want)
	}
}

func roundTrip(t *testing.T, s port.SecretStore) {
	a, b := seal(t, s, "ta", "api-key-1"), seal(t, s, "ta", "api-key-1")
	if bytes.Equal(a.Ciphertext, b.Ciphertext) || bytes.Contains(a.Ciphertext, []byte("api-key-1")) || a.KeyRef == "" {
		t.Fatal("a sealed record holds no plaintext, differs per seal and names its key")
	}
	opens(t, s, "ta", a, "api-key-1")
	opens(t, s, "ta", seal(t, s, "ta", ""), "")
}

func isolation(t *testing.T, s port.SecretStore) {
	sealed := seal(t, s, "ta", "secret")
	seal(t, s, "tb", "other")
	if _, err := s.Open(ctx, "tb", sealed); err == nil {
		t.Fatal("tenant tb opened a record of tenant ta")
	}
	if _, err := s.Open(ctx, "unknown", sealed); err == nil {
		t.Fatal("an unknown tenant opened a record")
	}
}

func tampering(t *testing.T, s port.SecretStore) {
	sealed := seal(t, s, "ta", "secret")
	for _, at := range []int{0, 1, 8, len(sealed.Ciphertext) - 1} {
		bad := port.Sealed{Ciphertext: bytes.Clone(sealed.Ciphertext), KeyRef: sealed.KeyRef}
		bad.Ciphertext[at] ^= 1
		if _, err := s.Open(ctx, "ta", bad); err == nil {
			t.Fatalf("a record changed at byte %d opened", at)
		}
	}
	if _, err := s.Open(ctx, "ta", port.Sealed{Ciphertext: []byte("short")}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("garbage: %v", err)
	}
}

func rotateTenant(t *testing.T, s port.SecretStore) {
	old := seal(t, s, "ta", "old")
	other := seal(t, s, "tb", "other")
	if err := s.Rotate(ctx, port.KeyScope{Tenant: "ta"}); err != nil {
		t.Fatal(err)
	}
	fresh := seal(t, s, "ta", "new")
	if fresh.KeyRef == old.KeyRef {
		t.Fatalf("a rotated tenant seals under a new key: %s", fresh.KeyRef)
	}
	opens(t, s, "ta", old, "old")
	opens(t, s, "ta", fresh, "new")
	opens(t, s, "tb", other, "other")
	if err := s.Rotate(ctx, port.KeyScope{Tenant: "nobody"}); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("rotate of a tenant without a key: %v", err)
	}
}

func rotateRoot(t *testing.T, s port.SecretStore) {
	a, b := seal(t, s, "ta", "a"), seal(t, s, "tb", "b")
	if err := s.Rotate(ctx, port.KeyScope{Root: true}); err != nil {
		t.Fatal(err)
	}
	opens(t, s, "ta", a, "a")
	opens(t, s, "tb", b, "b")
	opens(t, s, "ta", seal(t, s, "ta", "after"), "after")
}

func destroy(t *testing.T, s port.SecretStore) {
	a, b := seal(t, s, "ta", "a"), seal(t, s, "tb", "b")
	if err := s.Destroy(ctx, "ta"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "ta", a); !port.IsCode(err, port.CodeTenantRemoved) {
		t.Fatalf("a destroyed tenant opens nothing: %v", err)
	}
	if _, err := s.Seal(ctx, "ta", []byte("x")); !port.IsCode(err, port.CodeTenantRemoved) {
		t.Fatalf("a destroyed tenant seals nothing: %v", err)
	}
	if err := s.Rotate(ctx, port.KeyScope{Tenant: "ta"}); !port.IsCode(err, port.CodeTenantRemoved) {
		t.Fatalf("a destroyed tenant is not rotated back to life: %v", err)
	}
	if err := s.Destroy(ctx, "ta"); err != nil {
		t.Fatalf("destroy twice: %v", err)
	}
	opens(t, s, "tb", b, "b")
}
