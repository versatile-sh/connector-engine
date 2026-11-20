//go:build linux || darwin

package standalone

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTheControlSocketServesOnlyItsOwnUser(t *testing.T) {
	path := filepath.Join(filepath.Dir(shortDir(t)), "control.sock")
	lis, err := listenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the socket: %v %v", info.Mode(), err)
	}
	accepted := make(chan int, 1)
	go func() {
		if c, err := lis.Accept(); err == nil {
			uid, _ := peerUID(c)
			accepted <- uid
			c.Close()
		}
	}()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if uid := <-accepted; uid != os.Geteuid() {
		t.Fatalf("the peer runs as %d, want %d", uid, os.Geteuid())
	}
	c.Close()

	other := peerListener{Listener: lis.(peerListener).Listener, uid: os.Geteuid() + 1}
	go func() { _, _ = other.Accept() }()
	c, err = net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("a peer of another user stayed connected: %v", err)
	}
}

func TestTheControlSocketNeverReplacesAFileThatIsNotASocket(t *testing.T) {
	path := filepath.Join(filepath.Dir(shortDir(t)), "control.sock")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lis, err := listenControl(path); err == nil {
		lis.Close()
		t.Fatal("the control socket replaced a regular file")
	}
	if got, _ := os.ReadFile(path); string(got) != "x" {
		t.Fatalf("the file became %q", got)
	}
}
