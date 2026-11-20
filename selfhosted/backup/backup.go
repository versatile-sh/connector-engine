// Package backup is the encrypted file a person install exports and imports: a tar of named files and their manifest, sealed with AES-256-GCM under a key derived from a passphrase with Argon2id.
package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"golang.org/x/crypto/argon2"
)

// Extension is the file extension of a backup.
const Extension = ".cebak"

// Format is the version of the backup format this build writes and the newest it reads.
const Format = 2

const (
	magic        = "CEBAK\n"
	manifestName = "manifest.json"
	maxHeader    = 4 << 10
	maxMemoryKiB = 256 << 10
	maxTime      = 8
	maxThreads   = 16
	maxSize      = 1 << 30
)

var opening = make(chan struct{}, 1)

var (
	// ErrNotBackup is returned for a file that does not start like a backup.
	ErrNotBackup = errors.New("this is not a connector-engine backup")
	// ErrVersion is returned for a backup written in a format this build does not read.
	ErrVersion = errors.New("this backup was written by a newer connector-engine: update before you import it")
	// ErrPassphrase is returned when the passphrase does not open the backup.
	ErrPassphrase = errors.New("wrong passphrase")
	// ErrTampered is returned when the passphrase is right but the content was changed or cut.
	ErrTampered = errors.New("the backup is damaged or was changed")
)

// Manifest describes a backup: its format, the engine build that wrote it, when, how many of each thing it holds and the SHA-256 of every file.
type Manifest struct {
	Format    int               `json:"format"`
	Build     string            `json:"build"`
	CreatedAt time.Time         `json:"created_at"`
	Counts    map[string]int    `json:"counts"`
	Files     map[string]string `json:"files"`
}

type header struct {
	Format    int    `json:"format"`
	KDF       string `json:"kdf"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
	Salt      []byte `json:"salt"`
	Nonce     []byte `json:"nonce"`
	Check     []byte `json:"check"`
}

func (h header) keys(passphrase []byte) (enc, check []byte) {
	k := argon2.IDKey(passphrase, h.Salt, h.Time, h.MemoryKiB, h.Threads, 64)
	mac := hmac.New(sha256.New, k[32:])
	mac.Write([]byte(magic))
	return k[:32], mac.Sum(nil)
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Seal writes files and their manifest m as a backup encrypted with passphrase; m.Format and m.Files are filled in.
func Seal(w io.Writer, passphrase []byte, m Manifest, files map[string][]byte) error {
	if len(passphrase) == 0 {
		return errors.New("the passphrase is empty")
	}
	m.Format, m.Files = Format, map[string]string{}
	for name, b := range files {
		m.Files[name] = sum(b)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var plain bytes.Buffer
	tw := tar.NewWriter(&plain)
	put := func(name string, b []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), Format: tar.FormatPAX}); err != nil {
			return err
		}
		_, err := tw.Write(b)
		return err
	}
	if err := put(manifestName, raw); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if name == manifestName {
			return fmt.Errorf("%s is the manifest's own name", name)
		}
		if err := put(name, files[name]); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	h := header{Format: Format, KDF: "argon2id", Time: 3, MemoryKiB: 64 << 10, Threads: 4, Salt: make([]byte, 16), Nonce: make([]byte, 12)}
	rand.Read(h.Salt)
	rand.Read(h.Nonce)
	enc, check := h.keys(passphrase)
	h.Check = check
	line, err := json.Marshal(h)
	if err != nil {
		return err
	}
	aad := append(append([]byte(magic), line...), '\n')
	aead, err := gcm(enc)
	if err != nil {
		return err
	}
	_, err = w.Write(append(aad, aead.Seal(nil, h.Nonce, plain.Bytes(), aad)...))
	return err
}

func gcm(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Open reads a backup: it checks the format, the passphrase, the integrity and every checksum before it returns the manifest and the files.
func Open(r io.Reader, passphrase []byte) (Manifest, map[string][]byte, error) {
	br := bufio.NewReader(io.LimitReader(r, maxSize+1))
	head := make([]byte, len(magic))
	if _, err := io.ReadFull(br, head); err != nil || string(head) != magic {
		return Manifest{}, nil, ErrNotBackup
	}
	line, err := br.ReadSlice('\n')
	if err != nil || len(line) > maxHeader {
		return Manifest{}, nil, ErrNotBackup
	}
	aad := append([]byte(magic), line...)
	var h header
	if err := json.Unmarshal(line, &h); err != nil {
		return Manifest{}, nil, ErrNotBackup
	}
	if h.Format > Format || h.Format < 1 {
		return Manifest{}, nil, ErrVersion
	}
	if h.KDF != "argon2id" || h.Time == 0 || h.Time > maxTime || h.MemoryKiB == 0 || h.MemoryKiB > maxMemoryKiB || h.Threads == 0 || h.Threads > maxThreads || len(h.Nonce) != 12 || len(h.Salt) < 16 {
		return Manifest{}, nil, ErrTampered
	}
	opening <- struct{}{}
	enc, check := h.keys(passphrase)
	<-opening
	if !hmac.Equal(check, h.Check) {
		return Manifest{}, nil, ErrPassphrase
	}
	sealed, err := io.ReadAll(br)
	if err != nil || len(sealed) > maxSize {
		return Manifest{}, nil, ErrTampered
	}
	aead, err := gcm(enc)
	if err != nil {
		return Manifest{}, nil, err
	}
	plain, err := aead.Open(sealed[:0], h.Nonce, sealed, aad)
	if err != nil {
		return Manifest{}, nil, ErrTampered
	}
	return unpack(plain)
}

func unpack(plain []byte) (Manifest, map[string][]byte, error) {
	var m Manifest
	files := map[string][]byte{}
	budget := int64(len(plain))
	tr := tar.NewReader(bytes.NewReader(plain))
	for {
		hd, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || hd.Typeflag != tar.TypeReg || hd.Size < 0 || hd.Size > budget {
			return Manifest{}, nil, ErrTampered
		}
		b, err := io.ReadAll(io.LimitReader(tr, hd.Size))
		if err != nil || int64(len(b)) != hd.Size {
			return Manifest{}, nil, ErrTampered
		}
		budget -= hd.Size
		if hd.Name == manifestName {
			if err := json.Unmarshal(b, &m); err != nil {
				return Manifest{}, nil, ErrTampered
			}
			continue
		}
		files[hd.Name] = b
	}
	if m.Format > Format || m.Format < 1 {
		return Manifest{}, nil, ErrVersion
	}
	if len(m.Files) != len(files) {
		return Manifest{}, nil, ErrTampered
	}
	for name, b := range files {
		if m.Files[name] != sum(b) {
			return Manifest{}, nil, ErrTampered
		}
	}
	return m, files, nil
}
