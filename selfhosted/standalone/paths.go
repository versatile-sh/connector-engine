package standalone

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Name is the directory every install keeps its data in, under the per-user data directory of the OS.
const Name = "connector-engine"

// DataDir returns the data directory of an install: $CN_DATA_DIR, else $XDG_DATA_HOME or ~/.local/share on Linux and the BSDs, Application Support on macOS, %AppData% on Windows.
func DataDir(getenv func(string) string) (string, error) {
	if d := getenv("CN_DATA_DIR"); d != "" {
		return filepath.Abs(d)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		base, err := os.UserConfigDir()
		return filepath.Join(base, Name), err
	}
	if x := getenv("XDG_DATA_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, Name), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", Name), err
}

type paths struct {
	root           string
	store          string
	engineSecrets  string
	consoleState   string
	consoleSecrets string
	control        string
	operatorFile   string
	operator       string
	account        string
}

func layout(root string) paths {
	sum := sha256.Sum256([]byte(root))
	return paths{
		root:           root,
		store:          filepath.Join(root, "engine", "store.db"),
		engineSecrets:  filepath.Join(root, "engine", "secrets"),
		consoleState:   filepath.Join(root, "console", "state.json"),
		consoleSecrets: filepath.Join(root, "console", "secrets"),
		control:        filepath.Join(root, "control.sock"),
		operatorFile:   filepath.Join(root, "operator-token"),
		account:        "local-" + hex.EncodeToString(sum[:6]),
	}
}

func prepare(root string) (paths, error) {
	if !filepath.IsAbs(root) {
		return paths{}, errors.New("the data directory " + root + " is not an absolute path")
	}
	p := layout(filepath.Clean(root))
	for _, d := range []string{p.root, filepath.Dir(p.store), filepath.Dir(p.consoleState)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return p, err
		}
		info, err := os.Stat(d)
		if err != nil {
			return p, err
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return p, fmt.Errorf("%s is open to other users; run chmod 700 on it", d)
		}
		if uid := ownerUID(info); uid >= 0 && uid != os.Geteuid() {
			return p, fmt.Errorf("%s belongs to another user; an install keeps its data in a directory of its own user", d)
		}
	}
	token, err := operatorToken(p.operatorFile)
	p.operator = token
	return p, err
}

func holdsInstall(root string) error {
	p := layout(root)
	if info, err := os.Lstat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a directory (a symlink is refused): nothing is deleted", root)
	}
	if info, err := os.Lstat(p.operatorFile); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s holds no install (no operator-token): nothing is deleted", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, f := range []string{filepath.Dir(p.store), filepath.Dir(p.consoleState), p.control, p.operatorFile, ".DS_Store", "desktop.ini"} {
		known[filepath.Base(f)] = true
	}
	for _, e := range entries {
		if !known[e.Name()] {
			return fmt.Errorf("%s also holds %s, which no install writes: nothing is deleted", root, e.Name())
		}
	}
	return nil
}

func operatorToken(path string) (string, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, err = f.WriteString(hex.EncodeToString(b))
		err = errors.Join(err, f.Close())
	}
	if err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if token := strings.TrimSpace(string(raw)); err == nil && token == "" {
		return "", errors.New("the operator token file " + path + " is empty")
	}
	return strings.TrimSpace(string(raw)), err
}
