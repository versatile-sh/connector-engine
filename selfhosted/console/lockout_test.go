package console

import (
	"fmt"
	"testing"
	"time"
)

func TestALockoutCountsPerFoldedUsernameWithACeilingAndForgetsAfterAMinute(t *testing.T) {
	var l lockout
	now := time.Now()
	for range 5 {
		l.fail(now, "Bob")
	}
	if !l.locked(now, "bob") || l.locked(now, "ann") {
		t.Fatalf("after five failures bob is locked %v and ann %v", l.locked(now, "bob"), l.locked(now, "ann"))
	}
	if l.locked(now.Add(61*time.Second), "bob") {
		t.Error("the lock outlived its minute")
	}
	var spread lockout
	for i := range 30 {
		spread.fail(now, fmt.Sprintf("user-%d", i))
	}
	if !spread.locked(now, "someone-new") {
		t.Error("thirty failures over many names do not lock the console")
	}
}

func TestALockoutKeepsAtMostAThousandNames(t *testing.T) {
	var l lockout
	now := time.Now()
	for i := range 3000 {
		l.fail(now, fmt.Sprintf("name-%d", i))
	}
	if len(l.names) > maxLockoutNames {
		t.Fatalf("%d names kept", len(l.names))
	}
}
