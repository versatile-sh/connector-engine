// Package local is a file-backed SecretStore: a root key file wraps one key-encryption key per tenant, which wraps a data key per record.
package local

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/versatile-sh/connector-engine/internal/atomicfile"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/internal/ring"
)

const (
	rootFile    = "root.key"
	nextFile    = "root.key.next"
	stateFile   = "keystore.json"
	wrappedExt  = ".wrapped"
	unlockBytes = 32
)

// Form is how the root key of a directory rests on disk.
type Form string

// The forms of a root key: none yet, a plaintext file, or a file wrapped by an unlock key.
const (
	Fresh     Form = "fresh"
	Plaintext Form = "plaintext"
	Wrapped   Form = "wrapped"
)

// Store is the file-backed SecretStore; its root key file is plaintext or wrapped by an unlock key.
type Store struct {
	*ring.Ring
	dir    string
	mu     sync.Mutex
	root     []byte
	unlock   []byte
	restored bool
}

// FormOf reports how the root key in dir rests; a plaintext file wins over a wrapped one, as after an interrupted migration.
func FormOf(dir string) Form {
	switch {
	case exists(filepath.Join(dir, rootFile)):
		return Plaintext
	case exists(filepath.Join(dir, rootFile+wrappedExt)):
		return Wrapped
	}
	return Fresh
}

// Open loads the key store in dir, creating the directory and a new plaintext root key on first use; a root key readable by group or others is refused.
func Open(dir string) (*Store, error) { return open(dir, nil) }

// OpenWrapped loads the key store in dir with its root key wrapped by unlock, creating it wrapped on first use; a plaintext root key already there stays plaintext until Migrate.
func OpenWrapped(dir string, unlock []byte) (*Store, error) {
	if err := checkUnlock(unlock); err != nil {
		return nil, err
	}
	return open(dir, unlock)
}

func open(dir string, unlock []byte) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	state, err := readState(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	switch FormOf(dir) {
	case Wrapped:
		if unlock == nil {
			return nil, port.Errorf(port.CodeConflict, "the root key in %s is wrapped: it needs its unlock key", dir)
		}
		s.unlock = unlock
	case Fresh:
		if unlock != nil && len(state.Tenants) > 0 {
			return nil, port.Errorf(port.CodeConflict, "%s holds keys but no root key: restore %s or %s", dir, rootFile, rootFile+wrappedExt)
		}
		s.unlock = unlock
		root, err := ring.NewRootKey()
		if err == nil {
			err = s.write(rootFile, root)
		}
		if err != nil {
			return nil, err
		}
	}
	root, err := s.read(rootFile)
	if err != nil {
		return nil, err
	}
	if s.root, err = s.recoverRotation(root, state); err != nil {
		return nil, err
	}
	if s.Ring, err = ring.New(s.root, state, s.save); err != nil {
		return nil, err
	}
	return s, nil
}

// Form reports how the root key rests now.
func (s *Store) Form() Form {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unlock == nil {
		return Plaintext
	}
	return Wrapped
}

// WrappedBy reports whether the root key rests wrapped by unlock.
func (s *Store) WrappedBy(unlock []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unlock != nil && subtle.ConstantTimeCompare(s.unlock, unlock) == 1
}

// Migrate wraps the root key with unlock, checks that the wrapped file opens the key store, and only then removes the plaintext file; the same key again does nothing.
func (s *Store) Migrate(unlock []byte) error {
	if err := checkUnlock(unlock); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unlock != nil {
		if subtle.ConstantTimeCompare(s.unlock, unlock) == 1 {
			return nil
		}
		return port.Errorf(port.CodeConflict, "the root key is already wrapped by another unlock key")
	}
	return s.switchTo(unlock)
}

// Revert writes the root key back as a plaintext file, checks it, and only then removes the wrapped file; a plaintext store stays as it is.
func (s *Store) Revert() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unlock == nil {
		return nil
	}
	return s.switchTo(nil)
}

func (s *Store) switchTo(unlock []byte) error {
	old := filepath.Join(s.dir, s.name(rootFile))
	to := &Store{dir: s.dir, unlock: unlock}
	if err := to.write(rootFile, s.root); err != nil {
		return err
	}
	if err := to.verify(s.root); err != nil {
		_ = os.Remove(filepath.Join(s.dir, to.name(rootFile)))
		return err
	}
	if err := os.Remove(old); err != nil {
		return err
	}
	s.unlock = unlock
	return atomicfile.SyncDir(s.dir)
}

func (s *Store) verify(root []byte) error {
	back, err := s.read(rootFile)
	if err != nil {
		return err
	}
	state, err := readState(s.dir)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(back, root) != 1 || !ring.Opens(back, state) {
		return port.Errorf(port.CodeInternal, "the rewritten root key does not open the key store in %s", s.dir)
	}
	return nil
}

func (s *Store) name(base string) string {
	if s.unlock == nil {
		return base
	}
	return base + wrappedExt
}

func (s *Store) save(st ring.State, newRoot []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restored {
		return port.Errorf(port.CodeUnavailable, "the key store in %s was restored: open it again", s.dir)
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if newRoot == nil {
		return atomicfile.Write(filepath.Join(s.dir, stateFile), b)
	}
	if err := s.write(nextFile, newRoot); err != nil {
		return err
	}
	if err := atomicfile.Write(filepath.Join(s.dir, stateFile), b); err != nil {
		return err
	}
	if err := s.promote(); err != nil {
		return err
	}
	s.root = newRoot
	return nil
}

func (s *Store) promote() error {
	if err := os.Rename(filepath.Join(s.dir, s.name(nextFile)), filepath.Join(s.dir, s.name(rootFile))); err != nil {
		return err
	}
	return atomicfile.SyncDir(s.dir)
}

func (s *Store) recoverRotation(root []byte, state ring.State) ([]byte, error) {
	next, err := s.read(nextFile)
	if errors.Is(err, fs.ErrNotExist) {
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	switch {
	case ring.Opens(root, state):
		return root, os.Remove(filepath.Join(s.dir, s.name(nextFile)))
	case ring.Opens(next, state):
		return next, s.promote()
	}
	return nil, port.Errorf(port.CodeInvalidArgument, "neither %s nor %s opens the key store in %s", s.name(rootFile), s.name(nextFile), s.dir)
}

func (s *Store) read(base string) ([]byte, error) {
	path := filepath.Join(s.dir, s.name(base))
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, port.Errorf(port.CodeInvalidArgument, "root key file %s is readable by group or others", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "root key file %s is not hexadecimal", path)
	}
	if s.unlock == nil {
		return key, nil
	}
	if key, err = ring.UnwrapRoot(s.unlock, key); err != nil {
		return nil, port.Errorf(port.CodePermissionDenied, "the unlock key does not open %s", path)
	}
	return key, nil
}

func (s *Store) write(base string, root []byte) error {
	b := root
	if s.unlock != nil {
		var err error
		if b, err = ring.WrapRoot(s.unlock, root); err != nil {
			return err
		}
	}
	return atomicfile.Write(filepath.Join(s.dir, s.name(base)), []byte(hex.EncodeToString(b)+"\n"))
}

// ParseUnlockKey decodes the 64 hexadecimal characters of an unlock key.
func ParseUnlockKey(text string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(text))
	if err != nil || len(key) != unlockBytes {
		return nil, port.Errorf(port.CodeInvalidArgument, "the unlock key is %d hexadecimal characters", 2*unlockBytes)
	}
	return key, nil
}

func checkUnlock(unlock []byte) error {
	if len(unlock) != unlockBytes {
		return port.Errorf(port.CodeInvalidArgument, "the unlock key is %d bytes", unlockBytes)
	}
	return nil
}

func readState(dir string) (ring.State, error) {
	var state ring.State
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return state, nil
	case err != nil:
		return state, err
	}
	return state, json.Unmarshal(b, &state)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
