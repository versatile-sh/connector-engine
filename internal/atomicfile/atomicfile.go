// Package atomicfile replaces a file durably: a synced temporary file renamed over it, then the directory synced.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with b, mode 0600, so that a crash leaves either the old or the new content on disk.
func Write(path string, b []byte) error {
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
	return SyncDir(filepath.Dir(path))
}

// SyncDir flushes the entries of dir, making renames and removals in it durable.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
