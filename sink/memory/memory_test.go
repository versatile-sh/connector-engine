package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/sink/memory"
)

func TestSinkRecordsInOrderAndFailsOnDemand(t *testing.T) {
	s := memory.New()
	ctx := context.Background()
	boom := errors.New("boom")
	s.FailWith(func([]port.Event) error { return boom })
	if err := s.Deliver(ctx, []port.Event{{ID: "a"}}); !errors.Is(err, boom) || len(s.Events()) != 0 {
		t.Fatalf("a failed delivery records nothing: %v", err)
	}
	s.FailWith(nil)
	if err := s.Deliver(ctx, []port.Event{{ID: "a"}, {ID: "b"}}); err != nil {
		t.Fatal(err)
	}
	if got := s.Events(); len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("got %+v", got)
	}
}
