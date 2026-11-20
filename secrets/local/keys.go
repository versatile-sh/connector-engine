package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/versatile-sh/connector-engine/internal/atomicfile"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/internal/ring"
)

const (
	stagedExt = ".restore"
	rootBytes = 32
)

// Keys is the key material of a store in plaintext: the root key, the key ring state and the other files kept beside them, such as the local issuer's keys.
type Keys struct {
	Root  []byte            `json:"root"`
	State json.RawMessage   `json:"state"`
	Files map[string][]byte `json:"files,omitempty"`
}

func ownFile(name string) bool {
	return strings.HasPrefix(name, rootFile) || name == stateFile || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, stagedExt)
}

// Keys returns the store's key material in plaintext, for a backup that must open without this host's unlock key.
func (s *Store) Keys() (Keys, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := Keys{Root: s.root, State: json.RawMessage("{}"), Files: map[string][]byte{}}
	if raw, err := os.ReadFile(filepath.Join(s.dir, stateFile)); err == nil {
		k.State = raw
	} else if !errors.Is(err, os.ErrNotExist) {
		return Keys{}, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return Keys{}, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || ownFile(e.Name()) {
			continue
		}
		if k.Files[e.Name()], err = os.ReadFile(filepath.Join(s.dir, e.Name())); err != nil {
			return Keys{}, err
		}
	}
	return k, nil
}

// Restore stages k beside the live keys, runs then, and puts k in place only when then succeeds; the root keeps this store's form, so a wrapped store wraps it with its own unlock key, and the store refuses to persist anything until it is opened again.
func (s *Store) Restore(k Keys, then func() error) error {
	var state ring.State
	if err := json.Unmarshal(k.State, &state); err != nil {
		return port.Errorf(port.CodeInvalidArgument, "the key ring state does not decode: %v", err)
	}
	if len(k.Root) != rootBytes || !ring.Opens(k.Root, state) {
		return port.Errorf(port.CodeInvalidArgument, "the root key does not open the key ring state")
	}
	for name := range k.Files {
		if name != filepath.Base(name) || ownFile(name) || strings.HasPrefix(name, ".") {
			return port.Errorf(port.CodeInvalidArgument, "%q is not a file name the key store keeps", name)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	staged := map[string]string{filepath.Join(s.dir, stateFile) + stagedExt: filepath.Join(s.dir, stateFile)}
	for name := range k.Files {
		staged[filepath.Join(s.dir, name)+stagedExt] = filepath.Join(s.dir, name)
	}
	abort := func(err error) error {
		for from := range staged {
			_ = os.Remove(from)
		}
		_ = os.Remove(filepath.Join(s.dir, s.name(nextFile)))
		return err
	}
	err := s.write(nextFile, k.Root)
	if err == nil {
		err = atomicfile.Write(filepath.Join(s.dir, stateFile)+stagedExt, k.State)
	}
	for name, b := range k.Files {
		if err == nil {
			err = atomicfile.Write(filepath.Join(s.dir, name)+stagedExt, b)
		}
	}
	if err == nil {
		err = then()
	}
	if err != nil {
		return abort(err)
	}
	s.restored = true
	for from, to := range staged {
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}
	return s.promote()
}
