package console

import (
	"fmt"
	"testing"
	"time"
)

func TestALockoutCountsPerClientAndFoldedUsernameAndForgetsAfterAMinute(t *testing.T) {
	var l lockout
	now := time.Now()
	for range 5 {
		l.fail(now, "10.0.0.1", "Bob")
	}
	if !l.locked(now, "10.0.0.1", "bob") || l.locked(now, "10.0.0.1", "ann") {
		t.Fatal("five failures lock bob for that client only")
	}
	if l.locked(now, "10.0.0.2", "bob") {
		t.Error("another client was locked out of bob by the first client's failures")
	}
	if l.locked(now.Add(61*time.Second), "10.0.0.1", "bob") {
		t.Error("the lock outlived its minute")
	}
}

func TestALockoutCeilingIsPerClientNeverGlobal(t *testing.T) {
	var l lockout
	now := time.Now()
	for i := range 30 {
		l.fail(now, "10.0.0.1", fmt.Sprintf("user-%d", i))
	}
	if !l.locked(now, "10.0.0.1", "someone-new") {
		t.Error("thirty failures over many names do not lock that client")
	}
	if l.locked(now, "10.0.0.2", "someone-new") || l.locked(now, "10.0.0.2", "user-0") {
		t.Error("one attacker locked every other client")
	}
}

func TestALockoutKeepsAtMostAThousandKeys(t *testing.T) {
	var l lockout
	now := time.Now()
	for i := range 3000 {
		l.fail(now, fmt.Sprintf("10.1.%d.%d", i/250, i%250), fmt.Sprintf("name-%d", i))
	}
	if len(l.pairs) > maxLockoutKeys || len(l.ips) > maxLockoutKeys {
		t.Fatalf("%d pairs and %d clients kept", len(l.pairs), len(l.ips))
	}
}
