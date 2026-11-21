package hostkey_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/secrets/secretstest"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

var (
	ctx      = context.Background()
	key      = strings.Repeat("ab", 32)
	otherKey = strings.Repeat("cd", 32)
	quiet    = slog.New(slog.DiscardHandler)
)

func socketIn(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "unlock.sock")
}

type opened struct {
	s   *hostkey.Store
	err error
}

func start(t *testing.T, dir, socket string) chan opened {
	t.Helper()
	out := make(chan opened, 1)
	go func() {
		s, err := hostkey.Open(ctx, dir, socket, quiet)
		if err == nil {
			t.Cleanup(s.Close)
		}
		out <- opened{s, err}
	}()
	return out
}

func send(t *testing.T, socket string, op hostkey.Op, k string) (hostkey.Reply, error) {
	t.Helper()
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return hostkey.Send(c, socket, hostkey.Request{Op: op, Key: k})
}

func wait(t *testing.T, ch chan opened) *hostkey.Store {
	t.Helper()
	select {
	case o := <-ch:
		if o.err != nil {
			t.Fatal(o.err)
		}
		return o.s
	case <-time.After(5 * time.Second):
		t.Fatal("the store did not open")
	}
	return nil
}

func unlocked(t *testing.T, dir string) (*hostkey.Store, string) {
	t.Helper()
	socket := socketIn(t)
	ch := start(t, dir, socket)
	if r, err := send(t, socket, hostkey.OpUnlock, key); err != nil || r.Protection == hostkey.Locked {
		t.Fatalf("unlock: %+v %v", r, err)
	}
	return wait(t, ch), socket
}

func TestConformance(t *testing.T) {
	secretstest.Run(t, func(t *testing.T) port.SecretStore {
		s, _ := unlocked(t, t.TempDir())
		return s
	})
}

func TestAFreshStoreWaitsAndAWrongKeyLeavesItLocked(t *testing.T) {
	dir, socket := t.TempDir(), socketIn(t)
	ch := start(t, dir, socket)
	if r, err := send(t, socket, hostkey.OpStatus, ""); err != nil || r.Protection != hostkey.Locked {
		t.Fatalf("status: %+v %v", r, err)
	}
	if _, err := send(t, socket, hostkey.OpMigrate, key); err == nil {
		t.Fatal("a locked store migrates nothing")
	}
	if _, err := send(t, socket, hostkey.OpUnlock, "short"); err == nil {
		t.Fatal("a malformed key is refused")
	}
	if r, err := send(t, socket, hostkey.OpUnlock, key); err != nil || r.Protection != "wrapped" {
		t.Fatalf("unlock: %+v %v", r, err)
	}
	s := wait(t, ch)
	sealed, _ := s.Seal(ctx, "ta", []byte("secret"))
	s.Close()

	socket = socketIn(t)
	ch = start(t, dir, socket)
	if _, err := send(t, socket, hostkey.OpUnlock, otherKey); err == nil {
		t.Fatal("another key does not unlock")
	}
	if _, err := send(t, socket, hostkey.OpPlaintext, ""); err == nil {
		t.Fatal("a wrapped root is never replaced by a plaintext one")
	}
	if _, err := send(t, socket, hostkey.OpUnlock, key); err != nil {
		t.Fatal(err)
	}
	if got, err := wait(t, ch).Open(ctx, "ta", sealed); err != nil || string(got) != "secret" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestAHostWithoutKeychainChoosesPlaintextOnceAndThenOpensAtOnce(t *testing.T) {
	dir, socket := t.TempDir(), socketIn(t)
	ch := start(t, dir, socket)
	if r, err := send(t, socket, hostkey.OpPlaintext, ""); err != nil || r.Protection != "plaintext" {
		t.Fatalf("plaintext: %+v %v", r, err)
	}
	wait(t, ch).Close()
	s, err := hostkey.Open(ctx, dir, socketIn(t), quiet)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Form() != local.Plaintext {
		t.Fatal(s.Form())
	}
}

func TestMigrationOverTheSocketKeepsEveryRecordAndRunsAgain(t *testing.T) {
	dir := t.TempDir()
	plain, _ := local.Open(dir)
	sealed, _ := plain.Seal(ctx, "ta", []byte("secret"))
	socket := socketIn(t)
	s, err := hostkey.Open(ctx, dir, socket, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := send(t, socket, hostkey.OpUnlock, key); err != nil || r.Protection != "plaintext" {
		t.Fatalf("a plaintext store reports itself and is not migrated by unlock: %+v %v", r, err)
	}
	for range 2 {
		if r, err := send(t, socket, hostkey.OpMigrate, key); err != nil || r.Protection != "wrapped" {
			t.Fatalf("migrate: %+v %v", r, err)
		}
	}
	if _, err := send(t, socket, hostkey.OpUnlock, otherKey); err == nil {
		t.Fatal("unlock with a key that does not wrap the root is an error")
	}
	if _, err := os.Stat(filepath.Join(dir, "root.key")); !os.IsNotExist(err) {
		t.Fatalf("plaintext left: %v", err)
	}
	s.Close()
	again, socket := unlocked(t, dir)
	if got, err := again.Open(ctx, "ta", sealed); err != nil || string(got) != "secret" {
		t.Fatalf("after restart: %q %v", got, err)
	}
	if r, err := send(t, socket, hostkey.OpRevert, ""); err == nil && r.Error == "" {
		t.Fatalf("a keyless revert unwrapped the root key: %+v", r)
	}
	if again.Form() != local.Wrapped {
		t.Fatalf("a keyless revert changed the form to %s", again.Form())
	}
	if r, err := send(t, socket, hostkey.OpRevert, key); err != nil || r.Protection != "plaintext" {
		t.Fatalf("revert: %+v %v", r, err)
	}
}

func TestListenMovesTheSocketIntoPlaceAt0600AndLeavesNothingBehind(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	path := filepath.Join(dir, "s")
	lis, err := hostkey.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	si, err := os.Stat(path)
	if err != nil || si.Mode().Perm() != 0o600 {
		t.Fatalf("socket %v %v", si, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".sock-*")); len(left) != 0 {
		t.Fatalf("left behind %v", left)
	}
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("the moved socket does not accept: %v", err)
	}
	c.Close()
}

func TestCancellingTheWaitReturns(t *testing.T) {
	c, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := hostkey.Open(c, t.TempDir(), socketIn(t), quiet); err == nil {
		t.Fatal("a cancelled wait returns an error")
	}
}

func TestMainReadsTheKeyOnStdin(t *testing.T) {
	dir, socket := t.TempDir(), socketIn(t)
	ch := start(t, dir, socket)
	var out, errs bytes.Buffer
	if code := hostkey.Main(nil, socket, strings.NewReader(key+"\n"), &out, &errs); code != 0 || out.String() != "wrapped\n" {
		t.Fatalf("%d %q %q", code, out.String(), errs.String())
	}
	wait(t, ch)
	if code := hostkey.Main([]string{"--op", "status"}, socket, nil, &out, &errs); code != 0 {
		t.Fatal(errs.String())
	}
}
