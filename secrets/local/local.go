// Package local is a file-backed SecretStore: a root key file wraps one key-encryption key per tenant, which wraps a data key per record.
package local

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/secrets/internal/ring"
)

const (
	rootFile  = "root.key"
	nextFile  = "root.key.next"
	stateFile = "keystore.json"
)

// Open loads the key store in dir, creating the directory and a new root key on first use; a root key readable by group or others is refused.
func Open(dir string) (port.SecretStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	root, err := readRoot(filepath.Join(dir, rootFile))
	if errors.Is(err, fs.ErrNotExist) {
		if root, err = ring.NewRootKey(); err == nil {
			err = writeFile(filepath.Join(dir, rootFile), []byte(hex.EncodeToString(root)+"\n"))
		}
	}
	if err != nil {
		return nil, err
	}
	var state ring.State
	switch b, err := os.ReadFile(filepath.Join(dir, stateFile)); {
	case err == nil:
		if err := json.Unmarshal(b, &state); err != nil {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	if root, err = recoverRotation(dir, root, state); err != nil {
		return nil, err
	}
	return ring.New(root, state, func(s ring.State, newRoot []byte) error { return save(dir, s, newRoot) })
}

func save(dir string, s ring.State, newRoot []byte) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if newRoot == nil {
		return writeFile(filepath.Join(dir, stateFile), b)
	}
	if err := writeFile(filepath.Join(dir, nextFile), []byte(hex.EncodeToString(newRoot)+"\n")); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, stateFile), b); err != nil {
		return err
	}
	return promote(dir)
}

func promote(dir string) error {
	if err := os.Rename(filepath.Join(dir, nextFile), filepath.Join(dir, rootFile)); err != nil {
		return err
	}
	return syncDir(dir)
}

func recoverRotation(dir string, root []byte, state ring.State) ([]byte, error) {
	next, err := readRoot(filepath.Join(dir, nextFile))
	if errors.Is(err, fs.ErrNotExist) {
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	switch {
	case ring.Opens(root, state):
		return root, os.Remove(filepath.Join(dir, nextFile))
	case ring.Opens(next, state):
		return next, promote(dir)
	}
	return nil, port.Errorf(port.CodeInvalidArgument, "neither %s nor %s opens the key store in %s", rootFile, nextFile, dir)
}

func readRoot(path string) ([]byte, error) {
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
	root, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "root key file %s is not hexadecimal", path)
	}
	return root, nil
}

func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
