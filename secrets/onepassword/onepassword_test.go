package onepassword_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/secrets/onepassword"
	"github.com/versatile-sh/connector-engine/secrets/secretstest"
)

var ctx = context.Background()

const unlock = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"

type connect struct {
	*httptest.Server
	calls atomic.Int32
}

func fakeConnect(t *testing.T, value string) *connect {
	t.Helper()
	c := &connect{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer connect-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/vaults/vault1/items/item1" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "item1", "fields": []map[string]string{
			{"id": "username", "label": "username", "value": "engine"},
			{"id": "f1", "label": "unlock", "value": value},
		}})
	}))
	t.Cleanup(c.Close)
	return c
}

func config(c *connect) onepassword.Config {
	return onepassword.Config{ConnectURL: c.URL, Token: "connect-token", Vault: "vault1", Item: "item1", Field: "unlock"}
}

func TestConformance(t *testing.T) {
	c := fakeConnect(t, unlock)
	secretstest.Run(t, func(t *testing.T) port.SecretStore {
		s, err := onepassword.Open(ctx, t.TempDir(), config(c), c.Client())
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestTheRootKeyRestsWrappedAndReopensWithTheSameItem(t *testing.T) {
	c := fakeConnect(t, unlock)
	dir := t.TempDir()
	s, err := onepassword.Open(ctx, dir, config(c), c.Client())
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := s.Seal(ctx, "ta", []byte("secret"))
	if local.FormOf(dir) != local.Wrapped {
		t.Fatalf("form %s", local.FormOf(dir))
	}
	again, err := onepassword.Open(ctx, dir, config(c), c.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := again.Open(ctx, "ta", sealed); err != nil || string(got) != "secret" {
		t.Fatalf("reopen: %q %v", got, err)
	}
	other := fakeConnect(t, strings.Repeat("ab", 32))
	if _, err := onepassword.Open(ctx, dir, config(other), other.Client()); !port.IsCode(err, port.CodePermissionDenied) {
		t.Fatalf("another item's key: %v", err)
	}
}

func TestOptingInMigratesAPlaintextRootAndKeepsItsRecords(t *testing.T) {
	dir := t.TempDir()
	plain, _ := local.Open(dir)
	sealed, _ := plain.Seal(ctx, "ta", []byte("secret"))
	c := fakeConnect(t, unlock)
	s, err := onepassword.Open(ctx, dir, config(c), c.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "root.key")); !os.IsNotExist(err) || s.Form() != local.Wrapped {
		t.Fatalf("the plaintext root is migrated: %v %s", err, s.Form())
	}
	if got, err := s.Open(ctx, "ta", sealed); err != nil || string(got) != "secret" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestABadItemOrTokenOpensNothing(t *testing.T) {
	c := fakeConnect(t, "not-hex")
	cases := map[string]func(*onepassword.Config){
		"wrong token":       func(cfg *onepassword.Config) { cfg.Token = "other" },
		"unknown item":      func(cfg *onepassword.Config) { cfg.Item = "item2" },
		"missing field":     func(cfg *onepassword.Config) { cfg.Field = "password" },
		"malformed value":   func(*onepassword.Config) {},
		"path in the vault": func(cfg *onepassword.Config) { cfg.Vault = "../v1" },
		"no scheme":         func(cfg *onepassword.Config) { cfg.ConnectURL = "connect:8080" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := config(c)
			change(&cfg)
			dir := t.TempDir()
			if _, err := onepassword.Open(ctx, dir, cfg, c.Client()); err == nil {
				t.Fatal("opened")
			}
			if local.FormOf(dir) != local.Fresh {
				t.Fatal("a failed fetch must not create a root key")
			}
		})
	}
}
