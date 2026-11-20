package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/backup"
)

const (
	maxBackupBytes = 512 << 20
	minPassphrase  = 12
	engineFile     = "engine/snapshot.json"
	stateBackup    = "console/state.json"
	keysBackup     = "console/keys.json"
	activityPrefix = "console/activity/"
)

// BackupSummary is what an import shows before it replaces anything: the kind of file, who wrote it and when, what it holds and whether this install has data it would replace.
type BackupSummary struct {
	Kind      string         `json:"kind"`
	Build     string         `json:"build,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitzero"`
	Counts    map[string]int `json:"counts"`
	Replaces  bool           `json:"replaces"`
	Setup     *Setup         `json:"setup,omitempty"`
}

func backupName(at time.Time) string {
	return "connector-engine-backup-" + at.UTC().Format("20060102T150405Z") + backup.Extension
}

func checkPassphrase(p []byte) error {
	switch {
	case utf8.RuneCount(p) < minPassphrase:
		return invalid("the passphrase has at least %d characters", minPassphrase)
	case bytes.ContainsAny(p, "\r\n"):
		return invalid("the passphrase is one line")
	}
	return nil
}

// ExportBackup writes a full backup of the engine and the console, keys included, encrypted with passphrase, and records the time of the export.
func (c *Console) ExportBackup(ctx context.Context, passphrase []byte) ([]byte, string, error) {
	if err := checkPassphrase(passphrase); err != nil {
		return nil, "", err
	}
	keys, err := c.Secrets.Keys()
	if err != nil {
		return nil, "", err
	}
	connectors, err := c.Connectors(ctx)
	if err != nil {
		return nil, "", err
	}
	skills, err := c.Skills(ctx)
	if err != nil {
		return nil, "", err
	}
	snapshot, err := c.Engine.Snapshot(ctx)
	if err != nil {
		return nil, "", err
	}
	var build struct {
		Build string `json:"build"`
	}
	if err := json.Unmarshal(snapshot, &build); err != nil {
		return nil, "", unreachable(fmt.Errorf("the engine's snapshot does not decode: %w", err))
	}
	state, err := c.State.backup()
	if err != nil {
		return nil, "", err
	}
	files := map[string][]byte{engineFile: snapshot, stateBackup: state}
	if files[keysBackup], err = json.Marshal(keys); err != nil {
		return nil, "", err
	}
	if c.Activity != nil {
		days, err := c.Activity.files()
		if err != nil {
			return nil, "", err
		}
		for name, b := range days {
			files[activityPrefix+name] = b
		}
	}
	now := time.Now()
	m := backup.Manifest{Build: build.Build, CreatedAt: now.UTC().Truncate(time.Second), Counts: map[string]int{
		"connectors": len(connectors), "skills": len(skills), "keys": len(c.State.Agents()), "oauth_clients": c.State.clientCount()}}
	var out bytes.Buffer
	if err := backup.Seal(&out, passphrase, m, files); err != nil {
		return nil, "", err
	}
	if err := c.State.MarkExported(now); err != nil {
		return nil, "", err
	}
	return out.Bytes(), backupName(now), nil
}

func backupRefusal(err error) error {
	switch {
	case errors.Is(err, backup.ErrPassphrase), errors.Is(err, backup.ErrTampered), errors.Is(err, backup.ErrVersion), errors.Is(err, backup.ErrNotBackup):
		return invalid("%s", err.Error())
	}
	return err
}

func (c *Console) openBackup(raw, passphrase []byte) (BackupSummary, map[string][]byte, error) {
	m, files, err := backup.Open(bytes.NewReader(raw), passphrase)
	if err != nil {
		return BackupSummary{}, nil, backupRefusal(err)
	}
	for _, name := range []string{engineFile, stateBackup, keysBackup} {
		if files[name] == nil {
			return BackupSummary{}, nil, invalid("the backup has no %s", name)
		}
	}
	return BackupSummary{Kind: "full", Build: m.Build, CreatedAt: m.CreatedAt, Counts: m.Counts, Replaces: c.State.HasOwner()}, files, nil
}

// InspectBackup opens a backup or a setup file and returns its summary without changing anything.
func (c *Console) InspectBackup(ctx context.Context, raw, passphrase []byte) (BackupSummary, error) {
	if !isBackup(raw) {
		return c.inspectSetup(ctx, raw)
	}
	s, _, err := c.openBackup(raw, passphrase)
	return s, err
}

func isBackup(raw []byte) bool { return bytes.HasPrefix(raw, []byte("CEBAK\n")) }

// RestoreBackup replaces the engine and the console with a full backup, one restore at a time, then asks the process to restart so both start on the restored keys; firstRun refuses once an owner exists.
func (c *Console) RestoreBackup(ctx context.Context, raw, passphrase []byte, firstRun bool) (BackupSummary, error) {
	c.restoring.Lock()
	defer c.restoring.Unlock()
	if firstRun && c.State.HasOwner() {
		return BackupSummary{}, ErrOwnerExists
	}
	summary, files, err := c.openBackup(raw, passphrase)
	if err != nil {
		return BackupSummary{}, err
	}
	state, err := decodeState(files[stateBackup])
	if err != nil {
		return BackupSummary{}, invalid("the backup's console state does not decode: %v", err)
	}
	var keys local.Keys
	if err := json.Unmarshal(files[keysBackup], &keys); err != nil {
		return BackupSummary{}, invalid("the backup's console keys do not decode: %v", err)
	}
	days := map[string][]byte{}
	for name, b := range files {
		if day, ok := strings.CutPrefix(name, activityPrefix); ok {
			days[day] = b
		}
	}
	staged := ""
	if c.Activity != nil {
		if staged, err = c.Activity.stage(days); err != nil {
			return BackupSummary{}, err
		}
	}
	err = c.Secrets.Restore(keys, func() error { return c.Engine.Restore(ctx, files[engineFile]) })
	if err != nil {
		if staged != "" {
			_ = os.RemoveAll(staged)
		}
		return BackupSummary{}, err
	}
	if staged != "" {
		c.Activity.Close()
		if err := c.Activity.swap(staged); err != nil {
			return BackupSummary{}, err
		}
	}
	if err := c.State.replace(state, firstRun); err != nil {
		return BackupSummary{}, err
	}
	if c.Restart != nil {
		go c.Restart()
	}
	return summary, nil
}

func decodeState(raw []byte) (stateFile, error) {
	var d stateFile
	err := json.Unmarshal(raw, &d)
	if err == nil && d.Owner == nil {
		err = errors.New("it has no owner account")
	}
	migrateSlots(&d)
	return d, err
}

func (s *State) backup() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.data
	d.Sessions = nil
	return json.Marshal(d)
}

func (s *State) replace(d stateFile, firstRun bool) error {
	return s.update(func(next *stateFile) error {
		if firstRun && next.Owner != nil {
			return ErrOwnerExists
		}
		d.Sessions = nil
		*next = d
		return nil
	})
}

func (s *State) clientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Clients)
}

func (a *Activity) files() (map[string][]byte, error) {
	a.Flush()
	a.mu.Lock()
	defer a.mu.Unlock()
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(a.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if i := bytes.LastIndexByte(b, '\n'); filepath.Ext(e.Name()) == activityExt {
			b = b[:i+1]
		}
		out[e.Name()] = b
	}
	return out, nil
}

func (a *Activity) stage(days map[string][]byte) (string, error) {
	dir := a.dir + ".restore"
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for name, b := range days {
		if name != filepath.Base(name) || strings.HasPrefix(name, ".") {
			_ = os.RemoveAll(dir)
			return "", invalid("the backup's activity file %q has a path", name)
		}
		if err := writeAtomic(filepath.Join(dir, name), b); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

func (a *Activity) swap(staged string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.dir + ".old"
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.Rename(a.dir, old); err != nil {
		return err
	}
	if err := os.Rename(staged, a.dir); err != nil {
		return err
	}
	return os.RemoveAll(old)
}
