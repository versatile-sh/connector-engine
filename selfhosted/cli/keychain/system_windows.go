package keychain

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// System returns the keychain of the unlock keys, under Service.
func System() Keychain { return SystemFor(Service) }

// SystemFor returns DPAPI: each secret is encrypted for the signed-in Windows user in %LOCALAPPDATA%\<service>.
func SystemFor(service string) Keychain {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	return dpapi{dir: filepath.Join(dir, service)}
}

type dpapi struct{ dir string }

func (d dpapi) path(account string) string { return filepath.Join(d.dir, account+".unlock") }

func (d dpapi) Get(_ context.Context, account string) (string, error) {
	b, err := os.ReadFile(d.path(account))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	plain, err := crypt(b, false)
	return string(plain), err
}

func (d dpapi) Set(_ context.Context, account, secret string) error {
	b, err := crypt([]byte(secret), true)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(d.path(account), b, 0o600)
}

func (d dpapi) Delete(_ context.Context, account string) error {
	if err := os.Remove(d.path(account)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func crypt(in []byte, protect bool) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("empty DPAPI input")
	}
	src := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&src, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&src, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
