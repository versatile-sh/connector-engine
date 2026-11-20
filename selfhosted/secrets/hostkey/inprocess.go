package hostkey

import (
	"context"
	"errors"
	"log/slog"

	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
)

// Unlock opens the store in dir in-process with the unlock key of account in kc, wrapping a plaintext root on the way; with no keychain the root stays a plaintext file.
func Unlock(ctx context.Context, dir string, kc keychain.Keychain, account string, log *slog.Logger) (*Store, error) {
	st, err := unlock(ctx, dir, kc, account, log)
	if err != nil {
		return nil, err
	}
	return &Store{Store: st, dir: dir}, nil
}

func unlock(ctx context.Context, dir string, kc keychain.Keychain, account string, log *slog.Logger) (*local.Store, error) {
	key, err := keychain.Ensure(ctx, kc, account)
	if errors.Is(err, keychain.ErrUnavailable) {
		if local.FormOf(dir) == local.Wrapped {
			return nil, errors.New("the root key in " + dir + " is wrapped and this host has no OS keychain to unwrap it")
		}
		log.Warn("no OS keychain here: the root key stays a plaintext file", "dir", dir, "reason", err)
		return local.Open(dir)
	}
	if err != nil {
		return nil, err
	}
	unlockKey, err := local.ParseUnlockKey(key)
	if err != nil {
		return nil, err
	}
	st, err := local.OpenWrapped(dir, unlockKey)
	if err != nil || st.Form() != local.Plaintext {
		return st, err
	}
	if err := st.Migrate(unlockKey); err != nil {
		return nil, err
	}
	log.Info("the plaintext root key is now wrapped by the keychain", "dir", dir)
	return st, nil
}
