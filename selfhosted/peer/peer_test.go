package peer_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/peer"
)

func accepts(t *testing.T, uid int) bool {
	t.Helper()
	dir, err := os.MkdirTemp("", "peer")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	raw, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	lis := peer.Only(raw, uid)
	got := make(chan bool, 1)
	go func() {
		c, err := lis.Accept()
		if err == nil {
			c.Close()
		}
		got <- err == nil
	}()
	c, err := net.Dial("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case ok := <-got:
		return ok
	case <-time.After(300 * time.Millisecond):
		lis.Close()
		return <-got
	}
}

func TestOnlyServesThePeerThatRunsAsTheGivenUser(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root peers are always served")
	}
	if !accepts(t, os.Geteuid()) {
		t.Error("the owning user was refused")
	}
	if accepts(t, os.Geteuid()+1) {
		t.Error("another user was served")
	}
}
