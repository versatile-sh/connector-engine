// Package port holds the neutral model, the port interfaces and the error codes of connector-engine.
package port

// TenantID names a company: the isolation boundary and the row-level security key.
type TenantID string

// WorkspaceID names an organisational unit of one tenant.
type WorkspaceID string

// PersonID names a member of a tenant.
type PersonID string

// GroupID names a group of people, owned by the authorization service.
type GroupID string

// AgentID names one configured agent of a workspace.
type AgentID string

// SessionID names one run or conversation of an agent.
type SessionID string

// ConnectorID names an installed listing version in a workspace.
type ConnectorID string

// GrantID names one row of the grant table.
type GrantID string

// RuleID names one catalog rule.
type RuleID string

// ApprovalID names one approval record; it is random and unguessable.
type ApprovalID string

// CallID names one audit row of a forwarded call.
type CallID string

// CredentialID names one sealed upstream credential.
type CredentialID string

// OperationID names one long-running operation.
type OperationID string

// PauseID names one denial-brake pause.
type PauseID string

// Hash is a lowercase hexadecimal SHA-256 digest.
type Hash string

// Installation is the owner of an installation listing, as opposed to a tenant.
const Installation TenantID = ""

// ListingRef is the identity of a listing, (owner, name): what rules, offers and tenant rows name (§5.1).
type ListingRef struct {
	Owner TenantID `json:"owner,omitempty"`
	Name  string   `json:"name"`
}

// String renders the ref as owner/name, with an empty owner for the installation.
func (r ListingRef) String() string { return string(r.Owner) + "/" + r.Name }

// VersionRef names one immutable listing version.
type VersionRef struct {
	Listing ListingRef `json:"listing"`
	Version string     `json:"version"`
}
