package sealed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/versatile-sh/connector-engine/console/sealed"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/secrets/memory"
)

var ctx = context.Background()

func TestALockedBoxNeitherSealsNorOpens(t *testing.T) {
	var b sealed.Box
	if !b.Locked() {
		t.Fatal("a new box is locked")
	}
	if _, err := b.Seal(ctx, "token"); !errors.Is(err, sealed.ErrLocked) {
		t.Fatalf("seal: %v", err)
	}
	if _, err := b.Open(ctx, port.Sealed{}); !errors.Is(err, sealed.ErrLocked) {
		t.Fatalf("open: %v", err)
	}
}

func TestAnUnlockedBoxRoundTripsThroughJSONWithoutPlaintext(t *testing.T) {
	var b sealed.Box
	b.Unlock(memory.New())
	v, err := b.Seal(ctx, "xoxb-bot-token")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(v)
	if err != nil || bytes.Contains(raw, []byte("xoxb-bot-token")) {
		t.Fatalf("the stored form holds no plaintext: %s %v", raw, err)
	}
	var back port.Sealed
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Open(ctx, back); err != nil || got != "xoxb-bot-token" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestProtectionNamesHowTheRootKeyRests(t *testing.T) {
	var nilBox *sealed.Box
	var b sealed.Box
	if nilBox.Protection() != "locked" || b.Protection() != "locked" {
		t.Fatal("a box without a store is locked")
	}
	b.Unlock(memory.New())
	if b.Protection() != "" {
		t.Fatalf("a store without a key file: %q", b.Protection())
	}
	store, err := local.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b.Unlock(store)
	if b.Protection() != "plaintext" {
		t.Fatalf("a plaintext root key: %q", b.Protection())
	}
}

func TestReadyClosesOnTheFirstUnlock(t *testing.T) {
	var b sealed.Box
	ready := b.Ready()
	select {
	case <-ready:
		t.Fatal("ready before a store")
	default:
	}
	b.Unlock(memory.New())
	b.Unlock(memory.New())
	select {
	case <-ready:
	default:
		t.Fatal("not ready after Unlock")
	}
	if (*sealed.Box)(nil).Ready() != nil {
		t.Fatal("a nil box is never ready")
	}
}
