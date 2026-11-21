// Package catalog is the event catalog the engine serves: every published and consumed type, its payload and its JSON Schema.
package catalog

import (
	"encoding/json"
	"reflect"

	"github.com/invopop/jsonschema"
	"github.com/versatile-sh/connector-engine/internal/plugkit/usage"
)

// Type is one event type of the catalog; Number is its row, zero for a consumed type.
type Type struct {
	Number   int             `json:"number,omitempty"`
	Name     string          `json:"name"`
	Consumed bool            `json:"consumed"`
	Payload  any             `json:"-"`
	Usage    *usage.Event    `json:"-"`
	Schema   json.RawMessage `json:"schema"`
}

// Published types, in their order; a type is never renamed and the catalog only grows.
var Published = []Type{
	{Number: 1, Name: "connector.listing.imported", Payload: ListingImported{}},
	{Number: 2, Name: "connector.listing.reviewed", Payload: ListingReviewed{}},
	{Number: 3, Name: "connector.connector.installed", Payload: ConnectorInstalled{}},
	{Number: 4, Name: "connector.connector.updated", Payload: ConnectorUpdated{}},
	{Number: 5, Name: "connector.connector.removed", Payload: ConnectorRef{}},
	{Number: 6, Name: "connector.function.quarantined", Payload: FunctionQuarantined{}},
	{Number: 7, Name: "connector.function.accepted", Payload: FunctionAccepted{}},
	{Number: 8, Name: "connector.function.removed", Payload: FunctionRemoved{}},
	{Number: 9, Name: "connector.binding.changed", Payload: BindingChanged{}},
	{Number: 10, Name: "connector.grant.created", Payload: GrantCreated{}},
	{Number: 11, Name: "connector.grant.revoked", Payload: GrantRevoked{}},
	{Number: 12, Name: "connector.grant.expired", Payload: GrantRef{}},
	{Number: 13, Name: "connector.agent.exposure_warning", Payload: ExposureWarning{}},
	{Number: 14, Name: "connector.approval.requested", Payload: ApprovalRequested{}},
	{Number: 15, Name: "connector.approval.decided", Payload: ApprovalDecided{}},
	{Number: 16, Name: "connector.approval.expired", Payload: ApprovalRef{}},
	{Number: 17, Name: "connector.approval.cancelled", Payload: ApprovalCancelled{}},
	{Number: 18, Name: "connector.approval.brake", Payload: Brake{}},
	{Number: 19, Name: "connector.call.started", Payload: CallStarted{}},
	{Number: 20, Name: "connector.call.finished", Payload: CallFinished{}, Usage: &usage.Event{Count: "call"}},
	{Number: 21, Name: "connector.call.failed", Payload: CallFailed{}},
	{Number: 22, Name: "connector.call.denied", Payload: CallDenied{}},
	{Number: 23, Name: "connector.credential.connected", Payload: CredentialConnected{}},
	{Number: 24, Name: "connector.credential.needs_reauth", Payload: CredentialNeedsReauth{}},
	{Number: 25, Name: "connector.credential.revoked", Payload: CredentialRevoked{}},
	{Number: 26, Name: "connector.approval.brake_cleared", Payload: BrakeCleared{}},
	{Number: 27, Name: "connector.call.outcome_unknown", Payload: CallOutcomeUnknown{}},
	{Number: 28, Name: "connector.connector.instructions_changed", Payload: InstructionsChanged{}},
	{Number: 29, Name: "connector.connector.enabled_changed", Payload: EnabledChanged{}},
	{Number: 30, Name: "connector.grant.confirmed", Payload: GrantConfirmed{}},
	{Number: 31, Name: "connector.catalog.rule_changed", Payload: RuleChanged{}},
	{Number: 32, Name: "connector.catalog.policy_changed", Payload: PolicyChanged{}},
	{Number: 33, Name: "connector.catalog.offer_changed", Payload: OfferChanged{}},
	{Number: 34, Name: "connector.listing.created", Payload: ListingCreated{}},
	{Number: 35, Name: "connector.listing.status_changed", Payload: ListingStatusChanged{}},
	{Number: 36, Name: "connector.connector.blocked_changed", Payload: BlockedChanged{}},
	{Number: 37, Name: "connector.tenant.created", Payload: TenantCreated{}},
	{Number: 38, Name: "connector.tenant.removed", Payload: TenantRemoved{}},
	{Number: 39, Name: "connector.overlay.changed", Payload: OverlayChanged{}},
	{Number: 40, Name: "connector.overlay.suggested", Payload: OverlaySuggested{}},
	{Number: 41, Name: "connector.function.class_changed", Payload: FunctionClassChanged{}},
	{Number: 42, Name: "connector.function.state_changed", Payload: FunctionStateChanged{}},
	{Number: 43, Name: "connector.function.projection_changed", Payload: FunctionProjectionChanged{}},
	{Number: 44, Name: "connector.connector.instructions_accepted", Payload: InstructionsAccepted{}},
	{Number: 45, Name: "connector.snapshot.restored", Payload: SnapshotRestored{}},
}

// Consumed are the three outside facts; a recipe maps a producer's own event to one of them through the inbox.
var Consumed = []Type{
	{Name: "session.ended", Consumed: true, Payload: SessionEnded{}},
	{Name: "agent.removed", Consumed: true, Payload: AgentRemoved{}},
	{Name: "person.removed", Consumed: true, Payload: PersonRemoved{}},
}

// InstallationTypes are the published types on the installation stream; 34 and 35 also, for an installation listing.
var InstallationTypes = []string{"connector.listing.imported", "connector.listing.reviewed", "connector.catalog.offer_changed", "connector.tenant.created", "connector.tenant.removed", "connector.snapshot.restored"}

// SchemaID prefixes the $id of every event schema.
const SchemaID = "https://agents-control.dev/connector-engine/events/v1/"

// UsageRoles maps the engine's principal kinds to the neutral actor roles of x-usage.
var UsageRoles = map[string]string{"agent": "agent", "user": "user"}

// Schema reflects the JSON Schema of one type, with x-usage where the type counts usage.
func Schema(t Type) (json.RawMessage, error) {
	r := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
	s := r.Reflect(t.Payload)
	s.ID = jsonschema.ID(SchemaID + t.Name)
	s.Title = t.Name
	if t.Usage != nil {
		usage.Annotate(s, reflect.TypeOf(t.Payload), *t.Usage, UsageRoles)
	}
	return json.Marshal(s)
}

// All returns every type, published first, each with its JSON Schema.
func All() ([]Type, error) {
	out := make([]Type, 0, len(Published)+len(Consumed))
	for _, t := range append(append([]Type(nil), Published...), Consumed...) {
		s, err := Schema(t)
		if err != nil {
			return nil, err
		}
		t.Schema = s
		out = append(out, t)
	}
	return out, nil
}

// Get returns one type by name with its schema.
func Get(name string) (Type, bool, error) {
	all, err := All()
	for _, t := range all {
		if t.Name == name {
			return t, true, err
		}
	}
	return Type{}, false, err
}

// Known reports whether name is a published type.
func Known(name string) bool {
	for _, t := range Published {
		if t.Name == name {
			return true
		}
	}
	return false
}
