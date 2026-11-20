package local

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/versatile-sh/connector-engine/internal/atomicfile"
	"github.com/versatile-sh/connector-engine/port"
)

// Keyring is where Open keeps the issuer's keys: one file of keys sealed by Secrets under the installation, the age that rotates the signing key, and how long a retired key keeps verifying.
type Keyring struct {
	Path        string
	Secrets     port.SecretStore
	RotateEvery time.Duration
	Keep        time.Duration
}

type storedKey struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"created_at"`
	RetiredAt time.Time   `json:"retired_at,omitzero"`
	Key       port.Sealed `json:"key"`
}

// Open makes an issuer from the keys in k.Path: it rotates a signing key older than RotateEvery, drops keys retired longer than Keep ago, and persists any change before it signs.
func Open(ctx context.Context, name string, now func() time.Time, k Keyring) (*Issuer, error) {
	if now == nil {
		now = time.Now
	}
	stored, err := k.read()
	if err != nil {
		return nil, err
	}
	at := now()
	kept := stored[:0]
	for _, s := range stored {
		if s.RetiredAt.IsZero() || at.Sub(s.RetiredAt) < k.Keep {
			kept = append(kept, s)
		}
	}
	changed := len(kept) != len(stored)
	if len(kept) == 0 || at.Sub(kept[len(kept)-1].CreatedAt) >= k.RotateEvery {
		if len(kept) > 0 {
			kept[len(kept)-1].RetiredAt = at
		}
		fresh, err := k.mint(ctx, at)
		if err != nil {
			return nil, err
		}
		kept, changed = append(kept, fresh), true
	}
	if changed {
		if err := k.write(kept); err != nil {
			return nil, err
		}
	}
	privs := make([]ed25519.PrivateKey, len(kept))
	for i, s := range kept {
		seed, err := k.Secrets.Open(ctx, port.Installation, s.Key)
		if err != nil {
			return nil, fmt.Errorf("open issuer key %s: %w", s.ID, err)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("issuer key %s is not an Ed25519 seed", s.ID)
		}
		privs[i] = ed25519.NewKeyFromSeed(seed)
	}
	return build(name, now, privs)
}

func (k Keyring) mint(ctx context.Context, at time.Time) (storedKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return storedKey{}, err
	}
	sealed, err := k.Secrets.Seal(ctx, port.Installation, priv.Seed())
	if err != nil {
		return storedKey{}, fmt.Errorf("seal issuer key: %w", err)
	}
	return storedKey{ID: keyID(pub), CreatedAt: at, Key: sealed}, nil
}

func (k Keyring) read() ([]storedKey, error) {
	raw, err := os.ReadFile(k.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []storedKey
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", k.Path, err)
	}
	return out, nil
}

func (k Keyring) write(keys []storedKey) error {
	raw, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(k.Path), 0o700); err != nil {
		return err
	}
	return atomicfile.Write(k.Path, raw)
}
