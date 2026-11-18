package port

import (
	"context"
	"time"
)

// Scope is what a unit of work may touch: the rows of one tenant, the installation tables, or both (§4.4).
type Scope struct {
	Tenant       TenantID
	Installation bool
}

// Store runs units of work; every write of state, audit and outbox rows happens in one Write.
type Store interface {
	Read(ctx context.Context, scope Scope, fn func(Tx) error) error
	Write(ctx context.Context, scope Scope, fn func(Tx) error) error
}

// Outbox is the delivery side of the store, used by the relay outside any tenant scope.
type Outbox interface {
	Pending(ctx context.Context, limit int) ([]Event, error)
	MarkDelivered(ctx context.Context, ids []string) error
}

// AnyRevision skips the optimistic-lock check of a put.
const AnyRevision int64 = -1

// ConnectorFilter narrows ListConnectors; zero fields match all.
type ConnectorFilter struct {
	Workspace WorkspaceID
	Listing   *ListingRef
}

// BindingFilter narrows ListBindings; zero fields match all.
type BindingFilter struct {
	Connector ConnectorID
	Agent     AgentID
	Owner     PersonID
}

// GrantFilter narrows ListGrants; zero fields match all.
type GrantFilter struct {
	ScopeKind ScopeKind
	ScopeID   string
	Agent     AgentID
	Connector ConnectorID
	Listing   *ListingRef
}

// ApprovalFilter narrows ListApprovals; zero fields match all.
type ApprovalFilter struct {
	States    []ApprovalState
	Session   SessionID
	Agent     AgentID
	Connector ConnectorID
	CallHash  Hash
}

// CredentialFilter narrows ListCredentials; zero fields match all.
type CredentialFilter struct {
	OwnerKind OwnerKind
	OwnerID   string
	Connector ConnectorID
}

// Tx is the view of the store inside one unit of work; reads outside the scope answer not found.
type Tx interface {
	GetTenant(id TenantID) (Tenant, error)
	ListTenants() ([]Tenant, error)
	PutTenant(t Tenant) error

	GetListing(ref ListingRef) (Listing, error)
	ListListings() ([]Listing, error)
	PutListing(l Listing) error
	GetVersion(ref VersionRef) (ListingVersion, error)
	ListVersions(ref ListingRef) ([]ListingVersion, error)
	PutVersion(v ListingVersion) error

	ListOffers(tenant TenantID) ([]Offer, error)
	PutOffer(o Offer) error
	GetPolicy(workspace WorkspaceID) (CatalogPolicy, bool, error)
	PutPolicy(p CatalogPolicy) error
	ListRules() ([]CatalogRule, error)
	PutRule(r CatalogRule) error
	DeleteRule(id RuleID) error

	GetConnector(id ConnectorID) (Connector, error)
	ListConnectors(f ConnectorFilter) ([]Connector, error)
	PutConnector(c Connector, expectRevision int64) (Connector, error)
	GetFunction(connector ConnectorID, name string) (Function, error)
	ListFunctions(connector ConnectorID) ([]Function, error)
	PutFunction(f Function) error

	GetBinding(connector ConnectorID, agent AgentID) (Binding, error)
	ListBindings(f BindingFilter) ([]Binding, error)
	PutBinding(b Binding) error
	DeleteBinding(connector ConnectorID, agent AgentID) error

	GetGrant(id GrantID) (Grant, error)
	ListGrants(f GrantFilter) ([]Grant, error)
	PutGrant(g Grant) error
	DeleteGrant(id GrantID) error
	ReserveGrants(ids []GrantID) error

	GetApproval(id ApprovalID) (Approval, error)
	ListApprovals(f ApprovalFilter) ([]Approval, error)
	CreateApproval(a Approval) error
	UpdateApproval(a Approval, from ApprovalState) error

	GetPause(session SessionID, agent AgentID, connector ConnectorID) (Pause, bool, error)
	ListPauses() ([]Pause, error)
	PutPause(p Pause) error
	DeletePause(id PauseID) error

	ListTombstones(kind TombstoneKind) ([]Tombstone, error)
	PutTombstone(t Tombstone) error
	DeleteTombstone(kind TombstoneKind, subject string, workspace WorkspaceID) error

	GetCredential(id CredentialID) (Credential, error)
	ListCredentials(f CredentialFilter) ([]Credential, error)
	PutCredential(c Credential, expectRevision int64) (Credential, error)
	DeleteCredential(id CredentialID) error

	ListOverlays(connector ConnectorID, listing *ListingRef, function string) ([]Overlay, error)
	PutOverlay(o Overlay) error

	GetCall(id CallID) (Call, error)
	LatestCall(session SessionID, callHash Hash) (Call, bool, error)
	CreateCall(c Call) error
	UpdateCall(c Call, from CallState) error

	GetExposure(agent AgentID) (Exposure, bool, error)
	PutExposure(e Exposure) error

	GetOperation(id OperationID) (Operation, error)
	PutOperation(op Operation) error
	ListFanOuts() ([]FanOut, error)
	PutFanOut(f FanOut) error

	DueWork(now time.Time, limit int) ([]WorkItem, error)

	Audit(row AuditRow) error
	ListAudit(subject string) ([]AuditRow, error)

	Emit(e Event) (Event, error)
	EventsAfter(stream string, sequence int64, limit int) ([]Event, error)

	SeenInbox(id string) (bool, error)
	MarkInbox(id string, at time.Time) (bool, error)
}

// TenantPurger is an optional capability of a Store: it deletes every row of a removed tenant and returns the counts (§4.4 step 4).
type TenantPurger interface {
	PurgeTenant(ctx context.Context, tenant TenantID) (map[string]int, error)
}
