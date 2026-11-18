// Package port holds the neutral model, the port interfaces and the error codes of connector-engine.
package port

import "regexp"

var (
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	subjectPattern = regexp.MustCompile(`^[!-.0-~]{1,255}$`)
	subjectKinds   = map[string]bool{"person": true, "agent_owner": true, "on_behalf_of": true, "subject": true}
)

// CheckIDs takes kind, id pairs and refuses a malformed non-empty id or the reserved "installation".
// People are issuer subjects (printable ASCII without "/" or space, at most 255); every other id is [A-Za-z0-9._-]{1,128}.
func CheckIDs(pairs ...string) error {
	for i := 0; i+1 < len(pairs); i += 2 {
		kind, id := pairs[i], pairs[i+1]
		pattern := idPattern
		if subjectKinds[kind] {
			pattern = subjectPattern
		}
		if id != "" && (!pattern.MatchString(id) || id == "installation") {
			return Errorf(CodeInvalidArgument, "%s id %q is malformed or reserved", kind, id)
		}
	}
	return nil
}

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
