package catalog

import "time"

// ListingImported is event 1.
type ListingImported struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	TrustLevel string `json:"trust_level"`
}

// ListingReviewed is event 2.
type ListingReviewed struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	By            string         `json:"by"`
	CountsByClass map[string]int `json:"counts_by_class"`
}

// ConnectorInstalled is event 3.
type ConnectorInstalled struct {
	Connector string `json:"connector"`
	Listing   string `json:"listing"`
	Version   string `json:"version"`
	Level     string `json:"level"`
}

// ConnectorUpdated is event 4; tenant_rows_unmatched lists tenant fn: rows whose function is absent from the new version.
type ConnectorUpdated struct {
	Connector           string   `json:"connector"`
	FromVersion         string   `json:"from_version"`
	ToVersion           string   `json:"to_version"`
	Quarantined         []string `json:"quarantined"`
	TenantRowsUnmatched []string `json:"tenant_rows_unmatched"`
}

// ConnectorRef is the data of events 5 (removed).
type ConnectorRef struct {
	Connector string `json:"connector"`
}

// FunctionQuarantined is event 6.
type FunctionQuarantined struct {
	Connector     string   `json:"connector"`
	Function      string   `json:"function"`
	Reason        string   `json:"reason" jsonschema:"enum=changed,enum=new,enum=unreviewed"`
	OldHash       string   `json:"old_hash,omitempty"`
	NewHash       string   `json:"new_hash"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// FunctionAccepted is event 7.
type FunctionAccepted struct {
	Connector   string `json:"connector"`
	Function    string `json:"function"`
	Hash        string `json:"hash"`
	Class       string `json:"class"`
	ClassSource string `json:"class_source"`
	By          string `json:"by"`
}

// FunctionClassChanged is event 41: an admin set the class of a function.
type FunctionClassChanged struct {
	Connector string `json:"connector"`
	Function  string `json:"function"`
	Hash      string `json:"hash"`
	OldClass  string `json:"old_class"`
	NewClass  string `json:"new_class"`
	By        string `json:"by"`
}

// FunctionStateChanged is event 42: an admin disabled a function or returned it to its earlier state.
type FunctionStateChanged struct {
	Connector string `json:"connector"`
	Function  string `json:"function"`
	OldState  string `json:"old_state"`
	NewState  string `json:"new_state"`
	By        string `json:"by"`
}

// FunctionProjectionChanged is event 43: an admin turned response projection on or off for a function.
type FunctionProjectionChanged struct {
	Connector  string `json:"connector"`
	Function   string `json:"function"`
	Projection bool   `json:"projection"`
	By         string `json:"by"`
}

// InstructionsAccepted is event 44: an admin pinned the pending instructions hash of a connector.
type InstructionsAccepted struct {
	Connector string `json:"connector"`
	OldHash   string `json:"old_hash"`
	NewHash   string `json:"new_hash"`
	By        string `json:"by"`
}

// FunctionRemoved is event 8.
type FunctionRemoved struct {
	Connector string `json:"connector"`
	Function  string `json:"function"`
}

// BindingChanged is event 9.
type BindingChanged struct {
	Agent            string `json:"agent"`
	Connector        string `json:"connector"`
	CredentialPolicy string `json:"credential_policy"`
	Deleted          bool   `json:"deleted,omitempty"`
}

// GrantCreated is event 10.
type GrantCreated struct {
	Grant     string     `json:"grant"`
	ScopeKind string     `json:"scope_kind"`
	ScopeID   string     `json:"scope_id"`
	Selector  string     `json:"selector"`
	Mode      string     `json:"mode"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Origin    string     `json:"origin"`
	By        string     `json:"by"`
}

// GrantRevoked is event 11.
type GrantRevoked struct {
	Grant  string `json:"grant"`
	By     string `json:"by"`
	Reason string `json:"reason"`
}

// GrantRef is the data of event 12 (expired).
type GrantRef struct {
	Grant string `json:"grant"`
}

// ExposureWarning is event 13: the functions forming each leg of the trifecta check.
type ExposureWarning struct {
	Agent   string   `json:"agent"`
	Exposed bool     `json:"exposed"`
	LegA    []string `json:"leg_a"`
	LegB    []string `json:"leg_b"`
	LegC    []string `json:"leg_c"`
}

// ApprovalRequested is event 14; arguments travel by reference only.
type ApprovalRequested struct {
	Approval  string    `json:"approval"`
	Approvers []string  `json:"approvers"`
	Connector string    `json:"connector"`
	Function  string    `json:"function"`
	Class     string    `json:"class"`
	Agent     string    `json:"agent"`
	Session   string    `json:"session"`
	ExpiresAt time.Time `json:"expires_at"`
	CallHash  string    `json:"call_hash"`
}

// ApprovalDecided is event 15.
type ApprovalDecided struct {
	Approval     string `json:"approval"`
	Decision     string `json:"decision" jsonschema:"enum=approve,enum=approve_for_session,enum=deny"`
	By           string `json:"by"`
	Channel      string `json:"channel,omitempty"`
	SessionGrant bool   `json:"session_grant"`
}

// ApprovalRef is the data of event 16 (expired).
type ApprovalRef struct {
	Approval string `json:"approval"`
}

// ApprovalCancelled is event 17.
type ApprovalCancelled struct {
	Approval string `json:"approval"`
	Reason   string `json:"reason"`
}

// Brake is event 18.
type Brake struct {
	Agent       string `json:"agent"`
	Session     string `json:"session"`
	Connector   string `json:"connector"`
	Consecutive int    `json:"consecutive"`
	Total       int    `json:"total"`
}

// CallStarted is event 19.
type CallStarted struct {
	Call        string `json:"call"`
	Agent       string `json:"agent"`
	Session     string `json:"session"`
	Connector   string `json:"connector"`
	Function    string `json:"function"`
	Class       string `json:"class"`
	Approval    string `json:"approval,omitempty"`
	Credential  string `json:"credential,omitempty"`
	OverlayHash string `json:"overlay_hash,omitempty"`
}

// CallFinished is event 20, the usage fact: one call, attributed to tenant, agent and the person acted for.
type CallFinished struct {
	Call        string `json:"call"`
	Agent       string `json:"agent" usage:"actor=agent"`
	OnBehalfOf  string `json:"on_behalf_of,omitempty" usage:"actor=user"`
	Connector   string `json:"connector" usage:"dimension=connector"`
	Function    string `json:"function" usage:"dimension=function"`
	DurationMS  int64  `json:"duration_ms" usage:"quantity=millisecond"`
	ResultBytes int64  `json:"result_bytes" usage:"quantity=byte"`
	IsError     bool   `json:"is_error"`
}

// CallFailed is event 21.
type CallFailed struct {
	Call   string `json:"call"`
	Reason string `json:"reason"`
}

// CallDenied is event 22.
type CallDenied struct {
	Agent     string `json:"agent"`
	Connector string `json:"connector,omitempty"`
	Function  string `json:"function"`
	Reason    string `json:"reason"`
}

// CredentialConnected is event 23.
type CredentialConnected struct {
	Credential    string   `json:"credential"`
	OwnerKind     string   `json:"owner_kind"`
	Connector     string   `json:"connector"`
	ScopesGranted []string `json:"scopes_granted"`
}

// CredentialNeedsReauth is event 24.
type CredentialNeedsReauth struct {
	Credential string `json:"credential"`
	OwnerKind  string `json:"owner_kind"`
	OwnerID    string `json:"owner_id"`
}

// CredentialRevoked is event 25.
type CredentialRevoked struct {
	Credential string `json:"credential"`
	By         string `json:"by"`
}

// BrakeCleared is event 26.
type BrakeCleared struct {
	Agent     string `json:"agent"`
	Session   string `json:"session"`
	Connector string `json:"connector"`
	By        string `json:"by"`
}

// CallOutcomeUnknown is event 27.
type CallOutcomeUnknown struct {
	Call      string    `json:"call"`
	Approval  string    `json:"approval,omitempty"`
	Function  string    `json:"function"`
	StartedAt time.Time `json:"started_at"`
}

// InstructionsChanged is event 28.
type InstructionsChanged struct {
	Connector string `json:"connector"`
	OldHash   string `json:"old_hash"`
	NewHash   string `json:"new_hash"`
}

// EnabledChanged is event 29.
type EnabledChanged struct {
	Connector string `json:"connector"`
	Enabled   bool   `json:"enabled"`
	By        string `json:"by"`
}

// GrantConfirmed is event 30.
type GrantConfirmed struct {
	Grant   string `json:"grant"`
	OldHash string `json:"old_hash"`
	NewHash string `json:"new_hash"`
	By      string `json:"by"`
}

// RuleChanged is event 31.
type RuleChanged struct {
	Rule        string `json:"rule"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
	TargetKind  string `json:"target_kind"`
	Target      string `json:"target"`
	Show        bool   `json:"show"`
	Install     bool   `json:"install"`
	Ceiling     string `json:"ceiling"`
	Audience    string `json:"audience"`
	Deleted     bool   `json:"deleted,omitempty"`
	By          string `json:"by"`
}

// PolicyChanged is event 32.
type PolicyChanged struct {
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
	Default     string `json:"default"`
	By          string `json:"by"`
}

// OfferChanged is event 33.
type OfferChanged struct {
	Tenant     string `json:"tenant"`
	TargetKind string `json:"target_kind"`
	Target     string `json:"target"`
	Withheld   bool   `json:"withheld"`
}

// ListingCreated is event 34.
type ListingCreated struct {
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	TrustLevel string `json:"trust_level"`
	By         string `json:"by"`
}

// ListingStatusChanged is event 35; version is absent when the status of the listing itself changed.
type ListingStatusChanged struct {
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Starter bool   `json:"starter"`
	By      string `json:"by"`
	Reason  string `json:"reason,omitempty"`
}

// BlockedChanged is event 36.
type BlockedChanged struct {
	Connector string `json:"connector"`
	Blocked   bool   `json:"blocked"`
	Reason    string `json:"reason" jsonschema:"enum=not_in_catalog,enum=listing_quarantined"`
}

// TenantCreated is event 37.
type TenantCreated struct {
	Tenant       string `json:"tenant"`
	By           string `json:"by"`
	StarterRules int    `json:"starter_rules"`
}

// TenantRemoved is event 38.
type TenantRemoved struct {
	Tenant      string `json:"tenant"`
	By          string `json:"by"`
	Connectors  int    `json:"connectors"`
	Credentials int    `json:"credentials"`
	Approvals   int    `json:"approvals"`
	Key         string `json:"key" jsonschema:"enum=wrapped_key_deleted,enum=kms_deletion_scheduled,enum=tenant_held"`
}

// OverlayChanged is event 39.
type OverlayChanged struct {
	Connector      string `json:"connector,omitempty"`
	Listing        string `json:"listing,omitempty"`
	Function       string `json:"function"`
	OverlayVersion int    `json:"overlay_version"`
	OverlayHash    string `json:"overlay_hash"`
	UpstreamHash   string `json:"upstream_hash"`
	Source         string `json:"source"`
	By             string `json:"by"`
}

// OverlaySuggested is event 40.
type OverlaySuggested struct {
	Connector      string `json:"connector"`
	Function       string `json:"function"`
	OverlayVersion int    `json:"overlay_version"`
}

// SessionEnded is the consumed fact behind endSession.
type SessionEnded struct {
	Tenant  string `json:"tenant"`
	Session string `json:"session"`
}

// AgentRemoved is the consumed fact behind removeAgent.
type AgentRemoved struct {
	Tenant string `json:"tenant"`
	Agent  string `json:"agent"`
}

// PersonRemoved is the consumed fact behind removePerson: a workspace, or every workspace of the tenant.
type PersonRemoved struct {
	Tenant    string `json:"tenant"`
	Person    string `json:"person"`
	Workspace string `json:"workspace,omitempty"`
}
