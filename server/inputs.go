package server

import (
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
)

// ListingKey names a listing version; an empty owner is an installation listing.
type ListingKey struct {
	Owner   port.TenantID `json:"owner,omitempty"`
	Name    string        `json:"name"`
	Version string        `json:"version,omitempty"`
}

func (k ListingKey) listing() port.ListingRef { return port.ListingRef{Owner: k.Owner, Name: k.Name} }

func (k ListingKey) version() port.VersionRef {
	return port.VersionRef{Listing: k.listing(), Version: k.Version}
}

// ListListingsRequest filters the listings visible in a workspace.
type ListListingsRequest struct {
	Workspace  port.WorkspaceID `json:"workspace"`
	Source     string           `json:"source,omitempty"`
	TrustLevel port.TrustLevel  `json:"trust_level,omitempty"`
	Kind       port.Kind        `json:"kind,omitempty"`
	Q          string           `json:"q,omitempty"`
}

// GetListingRequest names one listing version as a workspace sees it.
type GetListingRequest struct {
	Workspace port.WorkspaceID `json:"workspace"`
	ListingKey
}

// WorkspaceRequest names a workspace.
type WorkspaceRequest struct {
	Workspace port.WorkspaceID `json:"workspace"`
}

// PutCatalogPolicyRequest sets the default of one workspace.
type PutCatalogPolicyRequest struct {
	Workspace port.WorkspaceID   `json:"workspace"`
	Subject   port.SubjectKind   `json:"subject"`
	Default   port.PolicyDefault `json:"default"`
}

// RuleRequest names one catalog rule.
type RuleRequest struct {
	ID port.RuleID `json:"id"`
}

// ExplainVisibilityRequest asks why a listing is or is not visible to a person in a workspace.
type ExplainVisibilityRequest struct {
	Workspace port.WorkspaceID `json:"workspace"`
	Person    port.PersonID    `json:"person,omitempty"`
	Owner     port.TenantID    `json:"owner,omitempty"`
	Name      string           `json:"name"`
}

// TenantRequest names a tenant.
type TenantRequest struct {
	Tenant port.TenantID `json:"tenant"`
}

// OfferRequest withholds or offers one target to a tenant.
type OfferRequest struct {
	Tenant     port.TenantID   `json:"tenant"`
	TargetKind port.TargetKind `json:"target_kind"`
	Target     string          `json:"target"`
	Withheld   bool            `json:"withheld"`
}

// ListingStatusRequest moves a listing, or one of its versions, to a new status.
type ListingStatusRequest struct {
	ListingKey
	Reason string `json:"reason,omitempty"`
}

// StarterRequest marks an installation listing as part of the starter set.
type StarterRequest struct {
	Owner   port.TenantID `json:"owner,omitempty"`
	Name    string        `json:"name"`
	Starter bool          `json:"starter"`
}

// LintListingRequest lints a manifest; with capture it also starts the listing and reads its functions.
type LintListingRequest struct {
	Manifest port.Manifest `json:"manifest"`
	Capture  bool          `json:"capture,omitempty"`
}

// ReviewListingRequest marks a listing version reviewed with the functions the reviewer read.
type ReviewListingRequest struct {
	ListingKey
	service.ReviewInput
}

// SourceRequest names a listing source.
type SourceRequest struct {
	Source string `json:"source"`
}

// ConnectorRequest names a connector.
type ConnectorRequest struct {
	ID port.ConnectorID `json:"id"`
}

// UpdateConnectorRequest moves a connector to a listing version.
type UpdateConnectorRequest struct {
	ID      port.ConnectorID `json:"id"`
	Version string           `json:"version"`
}

// LevelRequest sets the permission level of a connector.
type LevelRequest struct {
	ID    port.ConnectorID `json:"id"`
	Level port.Level       `json:"level"`
}

// FunctionRequest names a function of a connector.
type FunctionRequest struct {
	ID   port.ConnectorID `json:"id"`
	Name string           `json:"name"`
}

// AcceptFunctionRequest accepts one function at the hash the admin saw.
type AcceptFunctionRequest struct {
	ID   port.ConnectorID `json:"id"`
	Name string           `json:"name"`
	Seen port.Hash        `json:"seen"`
}

// AcceptFunctionsRequest accepts many functions, each at the hash the admin saw.
type AcceptFunctionsRequest struct {
	ID   port.ConnectorID     `json:"id"`
	Seen map[string]port.Hash `json:"seen"`
}

// ClassifyFunctionRequest sets the class of a function.
type ClassifyFunctionRequest struct {
	ID    port.ConnectorID `json:"id"`
	Name  string           `json:"name"`
	Class port.Class       `json:"class"`
}

// ProjectionRequest turns the response projection of a function on or off.
type ProjectionRequest struct {
	ID   port.ConnectorID `json:"id"`
	Name string           `json:"name"`
	On   bool             `json:"on"`
}

// OverlayTargetRequest names the connector, or the listing, whose function an overlay describes.
type OverlayTargetRequest struct {
	Connector    port.ConnectorID `json:"connector,omitempty"`
	ListingOwner port.TenantID    `json:"listing_owner,omitempty"`
	ListingName  string           `json:"listing_name,omitempty"`
	Function     string           `json:"function"`
}

func (r OverlayTargetRequest) target() service.OverlayTarget {
	t := service.OverlayTarget{Connector: r.Connector}
	if r.ListingName != "" {
		t.Listing = &port.ListingRef{Owner: r.ListingOwner, Name: r.ListingName}
	}
	return t
}

// ProposeOverlayRequest writes a new description overlay over the upstream hash the author saw.
type ProposeOverlayRequest struct {
	OverlayTargetRequest
	Title        string    `json:"title,omitempty"`
	Description  string    `json:"description"`
	UpstreamHash port.Hash `json:"upstream_hash"`
}

// OverlayVersionRequest accepts or restores one overlay version at the upstream hash the admin saw.
type OverlayVersionRequest struct {
	OverlayTargetRequest
	Version      int       `json:"version"`
	UpstreamSeen port.Hash `json:"upstream_seen"`
}

// BindingRequest names the binding of an agent to a connector.
type BindingRequest struct {
	ID    port.ConnectorID `json:"id"`
	Agent port.AgentID     `json:"agent"`
}

// PutBindingRequest binds an agent to a connector.
type PutBindingRequest struct {
	ID               port.ConnectorID      `json:"id"`
	Agent            port.AgentID          `json:"agent"`
	AgentOwner       port.PersonID         `json:"agent_owner,omitempty"`
	CredentialPolicy port.CredentialPolicy `json:"credential_policy"`
	AskRequester     bool                  `json:"ask_requester,omitempty"`
	Approvers        []string              `json:"approvers,omitempty"`
}

// AgentLevelRequest sets the permission level of an agent on a connector.
type AgentLevelRequest struct {
	ID    port.ConnectorID `json:"id"`
	Agent port.AgentID     `json:"agent"`
	Level port.Level       `json:"level"`
}

// ListBindingsRequest filters bindings; the connector comes from the path.
type ListBindingsRequest struct {
	ID    port.ConnectorID `json:"id"`
	Agent port.AgentID     `json:"agent,omitempty"`
	Owner port.PersonID    `json:"owner,omitempty"`
}

// GrantRequest names a grant row.
type GrantRequest struct {
	ID port.GrantID `json:"id"`
}

// ListGrantsRequest filters grant rows.
type ListGrantsRequest struct {
	ScopeKind port.ScopeKind   `json:"scope_kind,omitempty"`
	ScopeID   string           `json:"scope_id,omitempty"`
	Agent     port.AgentID     `json:"agent,omitempty"`
	Connector port.ConnectorID `json:"connector,omitempty"`
}

// SubjectRequest names whose view of the engine is asked for: an agent run as a token would carry it.
type SubjectRequest struct {
	Workspace  port.WorkspaceID `json:"workspace"`
	Agent      port.AgentID     `json:"agent"`
	AgentOwner port.PersonID    `json:"agent_owner,omitempty"`
	OnBehalfOf port.PersonID    `json:"on_behalf_of,omitempty"`
	Session    port.SessionID   `json:"session,omitempty"`
	Caller     port.Caller      `json:"caller,omitempty"`
}

func (r SubjectRequest) subject(p port.Principal) port.Subject {
	return port.Subject{Tenant: p.Claims.Tenant, Workspace: r.Workspace, Agent: r.Agent, AgentOwner: r.AgentOwner, OnBehalfOf: r.OnBehalfOf, Session: r.Session, Caller: r.Caller}
}

// ExplainAccessRequest asks how the 16 steps decide one function for one subject.
type ExplainAccessRequest struct {
	SubjectRequest
	Connector port.ConnectorID `json:"connector"`
	Function  string           `json:"function"`
}

// ListAgentFunctionsRequest asks what an agent would be listed; the agent comes from the path.
type ListAgentFunctionsRequest struct {
	SubjectRequest
}

// AgentFunction is one function an agent would be listed, with its mode.
type AgentFunction struct {
	Name        string           `json:"name"`
	Connector   port.ConnectorID `json:"connector"`
	Function    string           `json:"function"`
	Mode        port.Mode        `json:"mode"`
	OverlayHash port.Hash        `json:"overlay_hash,omitempty"`
}

// AgentFunctions is what an agent would be listed.
type AgentFunctions struct {
	Functions     []AgentFunction `json:"functions"`
	ApprovalTools bool            `json:"approval_tools"`
	Exposure      port.Exposure   `json:"exposure"`
}

// ListApprovalsRequest filters approvals.
type ListApprovalsRequest struct {
	State     port.ApprovalState `json:"state,omitempty"`
	Session   port.SessionID     `json:"session,omitempty"`
	Agent     port.AgentID       `json:"agent,omitempty"`
	Connector port.ConnectorID   `json:"connector,omitempty"`
}

// ApprovalRequest names an approval.
type ApprovalRequest struct {
	ID port.ApprovalID `json:"id"`
}

// DecideApprovalRequest answers a pending approval.
type DecideApprovalRequest struct {
	ID       port.ApprovalID `json:"id"`
	Decision port.Decision   `json:"decision"`
	Note     string          `json:"note,omitempty"`
	Channel  string          `json:"channel,omitempty"`
}

// PauseRequest names a denial-brake pause.
type PauseRequest struct {
	ID port.PauseID `json:"id"`
}

// WatchApprovalsRequest follows the approval events of a tenant after a sequence number.
type WatchApprovalsRequest struct {
	After int64 `json:"after,omitempty"`
	Limit int   `json:"limit,omitempty"`
}

// Advance moves the cursor past an event sent.
func (r *WatchApprovalsRequest) Advance(sequence int64) { r.After = sequence }

// CallRequest names one audited call.
type CallRequest struct {
	ID port.CallID `json:"id"`
}

// BeginConnectRequest starts the authorization of a credential for a connector.
type BeginConnectRequest struct {
	ID    port.ConnectorID `json:"id"`
	Owner port.OwnerKind   `json:"owner"`
}

// CompleteConnectRequest finishes an authorization with the state and the code the issuer returned.
type CompleteConnectRequest struct {
	State string `json:"state"`
	Code  string `json:"code"`
}

// CredentialRequest names a credential.
type CredentialRequest struct {
	ID port.CredentialID `json:"id"`
}

// ListCredentialsRequest filters credential metadata.
type ListCredentialsRequest struct {
	OwnerKind port.OwnerKind   `json:"owner_kind,omitempty"`
	OwnerID   string           `json:"owner_id,omitempty"`
	Connector port.ConnectorID `json:"connector,omitempty"`
}

// SessionRequest names a session.
type SessionRequest struct {
	ID port.SessionID `json:"id"`
}

// AgentRequest names an agent.
type AgentRequest struct {
	ID port.AgentID `json:"id"`
}

// PersonRequest names a person, of the whole tenant or of one workspace.
type PersonRequest struct {
	ID        port.PersonID    `json:"id"`
	Workspace port.WorkspaceID `json:"workspace,omitempty"`
}

// OperationRequest names a long-running operation.
type OperationRequest struct {
	ID port.OperationID `json:"id"`
}

// EventTypes is the event catalog of §12.
type EventTypes struct {
	Types []EventType `json:"types"`
}

// EventType is one published or consumed event type with its JSON Schema.
type EventType struct {
	Number   int            `json:"number,omitempty"`
	Name     string         `json:"name"`
	Consumed bool           `json:"consumed"`
	Schema   map[string]any `json:"schema"`
}

// EventTypeRequest names an event type.
type EventTypeRequest struct {
	Type string `json:"type"`
}

// ListRolesRequest filters the roles of a tenant, or of the installation for the empty tenant.
type ListRolesRequest struct {
	Tenant port.TenantID `json:"tenant,omitempty"`
	Person port.PersonID `json:"person,omitempty"`
}

// ListMembersRequest filters group memberships.
type ListMembersRequest struct {
	Person port.PersonID `json:"person,omitempty"`
	Group  port.GroupID  `json:"group,omitempty"`
}
