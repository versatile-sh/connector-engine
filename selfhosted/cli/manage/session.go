package manage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/versatile-sh/connector-engine/internal/atomicfile"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
)

const keychainService = "vsh"

// Session is a console session of one user as vsh keeps it: the cookie value, its CSRF token and the username.
type Session struct {
	Cookie string `json:"cookie"`
	CSRF   string `json:"csrf"`
	User   string `json:"user"`
}

// Store keeps one session per console address in the OS keychain, or in a 0600 file under Dir when there is none.
type Store struct {
	Keychain keychain.Keychain
	Dir      string
}

// NewStore returns the store of this user: the keychain under service vsh unless $VSH_NO_KEYCHAIN is set, and $VSH_CONFIG_DIR or the user config dir.
func NewStore(getenv func(string) string) Store {
	s := Store{Dir: getenv("VSH_CONFIG_DIR")}
	if s.Dir == "" {
		if base, err := os.UserConfigDir(); err == nil {
			s.Dir = filepath.Join(base, "vsh")
		}
	}
	if getenv("VSH_NO_KEYCHAIN") == "" {
		s.Keychain = keychain.SystemFor(keychainService)
	}
	return s
}

func account(base string) string {
	sum := sha256.Sum256([]byte(base))
	return "session-" + hex.EncodeToString(sum[:8])
}

func (s Store) file(base string) string {
	return filepath.Join(s.Dir, "sessions", account(base)+".json")
}

// Load returns the session kept for the console at base.
func (s Store) Load(ctx context.Context, base string) (Session, error) {
	var raw []byte
	if s.Keychain != nil {
		if v, err := s.Keychain.Get(ctx, account(base)); err == nil {
			raw, _ = hex.DecodeString(v)
		}
	}
	if raw == nil && s.Dir != "" {
		b, err := os.ReadFile(s.file(base))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Session{}, err
		}
		raw = b
	}
	var out Session
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil || out.Cookie == "" {
		return Session{}, errNoSession
	}
	return out, nil
}

// Save keeps v for the console at base and says where: the keychain, or the file it wrote.
func (s Store) Save(ctx context.Context, base string, v Session) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if s.Keychain != nil && s.Keychain.Set(ctx, account(base), hex.EncodeToString(raw)) == nil {
		_ = os.Remove(s.file(base))
		return "the OS keychain", nil
	}
	if s.Dir == "" {
		return "", errors.New("no OS keychain and no config directory to keep the session in; set $VSH_CONFIG_DIR")
	}
	if err := os.MkdirAll(filepath.Dir(s.file(base)), 0o700); err != nil {
		return "", err
	}
	if err := atomicfile.Write(s.file(base), raw); err != nil {
		return "", err
	}
	return s.file(base), nil
}

// Delete forgets the session of the console at base, in the keychain and on disk.
func (s Store) Delete(ctx context.Context, base string) error {
	if s.Keychain != nil {
		_ = s.Keychain.Delete(ctx, account(base))
	}
	if err := os.Remove(s.file(base)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
