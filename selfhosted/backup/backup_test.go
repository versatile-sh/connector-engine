package backup_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/backup"
)

var files = map[string][]byte{"engine/snapshot.json": []byte(`{"root":"the plaintext root key"}`), "console/state.json": []byte(`{"owner":{}}`)}

func sealed(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := backup.Seal(&buf, []byte("a long passphrase"), backup.Manifest{Build: "v1", CreatedAt: time.Unix(0, 0).UTC(), Counts: map[string]int{"connectors": 2}}, files); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestABackupOpensWithItsPassphraseOnly(t *testing.T) {
	raw := sealed(t)
	if bytes.Contains(raw, []byte("plaintext root key")) || bytes.Contains(raw, []byte("connectors")) {
		t.Fatal("the backup holds plaintext")
	}
	m, got, err := backup.Open(bytes.NewReader(raw), []byte("a long passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != backup.Format || m.Build != "v1" || m.Counts["connectors"] != 2 || len(m.Files) != 2 {
		t.Fatalf("manifest %+v", m)
	}
	for name, b := range files {
		if !bytes.Equal(got[name], b) {
			t.Errorf("%s: %q", name, got[name])
		}
	}
	if _, _, err := backup.Open(bytes.NewReader(raw), []byte("another passphrase")); !errors.Is(err, backup.ErrPassphrase) {
		t.Fatalf("a wrong passphrase: %v", err)
	}
}

func TestATamperedOrCutBackupIsRefused(t *testing.T) {
	raw := sealed(t)
	flipped := bytes.Clone(raw)
	flipped[len(flipped)-40] ^= 1
	format := fmt.Appendf(nil, `"format":%d`, backup.Format)
	header := bytes.Replace(bytes.Clone(raw), format, append(bytes.Clone(format), ' '), 1)
	for name, b := range map[string][]byte{"a flipped byte": flipped, "cut": raw[:len(raw)-10], "an edited header": header} {
		if _, _, err := backup.Open(bytes.NewReader(b), []byte("a long passphrase")); !errors.Is(err, backup.ErrTampered) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := backup.Open(strings.NewReader("PK\x03\x04 a zip"), []byte("x")); !errors.Is(err, backup.ErrNotBackup) {
		t.Errorf("not a backup: %v", err)
	}
}

func TestABackupOfANewerFormatIsRefusedBeforeItsPassphrase(t *testing.T) {
	newer := bytes.Replace(sealed(t), fmt.Appendf(nil, `"format":%d`, backup.Format), fmt.Appendf(nil, `"format":%d`, backup.Format+1), 1)
	if _, _, err := backup.Open(bytes.NewReader(newer), []byte("wrong")); !errors.Is(err, backup.ErrVersion) {
		t.Fatalf("a newer format: %v", err)
	}
}
