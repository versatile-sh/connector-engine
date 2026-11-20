package hostkey_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

type fakeKeychain map[string]string

func (f fakeKeychain) Get(_ context.Context, a string) (string, error) {
	if v, ok := f[a]; ok {
		return v, nil
	}
	return "", keychain.ErrNotFound
}

func (f fakeKeychain) Set(_ context.Context, a, s string) error { f[a] = s; return nil }

func (f fakeKeychain) Delete(_ context.Context, a string) error { delete(f, a); return nil }

type brokenKeychain struct{ fakeKeychain }

func (brokenKeychain) Get(context.Context, string) (string, error) {
	return "", errors.Join(keychain.ErrUnavailable, errors.New("no D-Bus session"))
}

func unlockedIn(t *testing.T, dir string, kc keychain.Keychain) *hostkey.Store {
	t.Helper()
	s, err := hostkey.Unlock(ctx, dir, kc, "local-test", quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestUnlockWrapsAFreshRootWithTheKeychainKey(t *testing.T) {
	dir, kc := filepath.Join(t.TempDir(), "secrets"), fakeKeychain{}
	if got := unlockedIn(t, dir, kc).Form(); got != local.Wrapped {
		t.Fatalf("form %s, want wrapped", got)
	}
	if len(kc["local-test"]) != 64 {
		t.Fatalf("the keychain holds %q", kc["local-test"])
	}
	if got := unlockedIn(t, dir, kc).Capabilities(); len(got) != 1 || got[0] != "secrets.root_key_wrapped" {
		t.Fatalf("capabilities %v", got)
	}
}

func TestUnlockKeepsPlaintextWithoutAKeychainAndWrapsItOnceOneExists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	for _, kc := range []keychain.Keychain{nil, brokenKeychain{}} {
		if got := unlockedIn(t, dir, kc).Form(); got != local.Plaintext {
			t.Fatalf("form %s, want plaintext", got)
		}
	}
	if got := unlockedIn(t, dir, fakeKeychain{}).Form(); got != local.Wrapped {
		t.Fatalf("form %s, want wrapped after a keychain appears", got)
	}
}

func TestUnlockRefusesAWrappedRootWithoutAKeychain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	unlockedIn(t, dir, fakeKeychain{})
	if _, err := hostkey.Unlock(ctx, dir, nil, "local-test", quiet); err == nil {
		t.Fatal("a wrapped root opened without its unlock key")
	}
	if _, err := hostkey.Unlock(ctx, dir, fakeKeychain{"local-test": otherKey}, "local-test", quiet); err == nil {
		t.Fatal("a wrapped root opened with another unlock key")
	}
}
