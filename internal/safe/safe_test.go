package safe_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/internal/safe"
)

func TestRunReturnsTheErrorOfFn(t *testing.T) {
	want := errors.New("boom")
	if err := safe.Run(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestRunTurnsAPanicIntoAnError(t *testing.T) {
	err := safe.Run(func() error { panic("bad state") })
	var p *safe.Panic
	if !errors.As(err, &p) || p.Value != "bad state" || !strings.HasPrefix(err.Error(), "panic: bad state") {
		t.Fatalf("got %v", err)
	}
}
