package app

import (
	"context"
	"testing"

	"github.com/agents-control/connector-engine/catalog"
	"github.com/agents-control/connector-engine/port"
	sinkmemory "github.com/agents-control/connector-engine/sink/memory"
)

func TestEveryListChangingEventIsInTheCatalog(t *testing.T) {
	all, err := catalog.All()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, ty := range all {
		known[ty.Name] = true
	}
	for _, name := range listChangingEvents {
		if !known[name] {
			t.Errorf("%s is not an event type", name)
		}
	}
}

func TestListChangedNotifiesOnceForTheFactsThatChangeAList(t *testing.T) {
	next, notified := sinkmemory.New(), 0
	sink := listChanged{next: next, notify: func() { notified++ }}
	deliver := func(types ...string) {
		var evs []port.Event
		for _, ty := range types {
			evs = append(evs, port.Event{Type: ty})
		}
		if err := sink.Deliver(context.Background(), evs); err != nil {
			t.Fatal(err)
		}
	}
	deliver("connector.call.finished", "connector.approval.requested")
	if notified != 0 {
		t.Fatalf("calls and approvals do not change a list: %d", notified)
	}
	deliver("connector.call.finished", "connector.grant.created", "connector.function.quarantined")
	if notified != 1 || len(next.Events()) != 5 {
		t.Fatalf("one notification per batch, every event forwarded: %d %d", notified, len(next.Events()))
	}
}
