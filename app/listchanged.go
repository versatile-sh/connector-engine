package app

import (
	"context"
	"slices"

	"github.com/versatile-sh/connector-engine/port"
)

var listChangingEvents = []string{
	"connector.grant.created", "connector.grant.revoked", "connector.grant.expired", "connector.grant.confirmed",
	"connector.approval.brake", "connector.approval.brake_cleared",
	"connector.function.quarantined", "connector.function.accepted", "connector.function.removed",
	"connector.function.class_changed", "connector.function.state_changed", "connector.function.projection_changed",
	"connector.connector.installed", "connector.connector.updated", "connector.connector.removed", "connector.connector.enabled_changed",
	"connector.connector.blocked_changed", "connector.binding.changed", "connector.overlay.changed",
	"connector.catalog.rule_changed", "connector.catalog.policy_changed", "connector.listing.status_changed",
}

type listChanged struct {
	next   port.EventSink
	notify func()
}

func (l listChanged) Deliver(ctx context.Context, events []port.Event) error {
	if slices.ContainsFunc(events, func(e port.Event) bool { return slices.Contains(listChangingEvents, e.Type) }) {
		l.notify()
	}
	return l.next.Deliver(ctx, events)
}
