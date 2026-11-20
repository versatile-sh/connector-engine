package console

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAConnectorLockGivesWayToTheCallersContext(t *testing.T) {
	c := &Console{}
	unlock, err := c.lockConnector(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.lockConnector(ctx, "c1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a second writer got %v", err)
	}
	other, err := c.lockConnector(context.Background(), "c2")
	if err != nil {
		t.Fatalf("another connector is not held: %v", err)
	}
	other()
	unlock()
	again, err := c.lockConnector(context.Background(), "c1")
	if err != nil {
		t.Fatalf("a released lock: %v", err)
	}
	again()
}

func TestRemovingAConnectorWaitsForItsWriters(t *testing.T) {
	state, err := OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Console{State: state}
	unlock, err := c.lockConnector(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Remove(ctx, "c1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Remove ran beside a writer: %v", err)
	}
	unlock()
}
