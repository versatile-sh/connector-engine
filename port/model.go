package port

import (
	"encoding/json"
	"strings"
	"time"
)

// Tenant is one row of the installation's tenant registry.
type Tenant struct {
	ID        TenantID       `json:"id"`
	State     TenantState    `json:"state"`
	KeyRef    string         `json:"key_ref,omitempty"`
	CreatedBy string         `json:"created_by,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	RemovedAt time.Time      `json:"removed_at,omitzero"`
	Counts    map[string]int `json:"counts,omitempty"`
}

// Connector is an installed listing version in one workspace.
type Connector struct {
	ID               ConnectorID       `json:"id"`
	Tenant           TenantID          `json:"tenant"`
	Workspace        WorkspaceID       `json:"workspace"`
	Alias            string            `json:"alias"`
	Listing          VersionRef        `json:"listing"`
	Enabled          bool              `json:"enabled"`
	Config           map[string]string `json:"config,omitempty"`
	InstructionsHash Hash              `json:"instructions_hash,omitempty"`
	CallTimeout      time.Duration     `json:"call_timeout,omitempty"`
	ApprovalTimeout  time.Duration     `json:"approval_timeout,omitempty"`
	LastRelistAt     time.Time         `json:"last_relist_at,omitzero"`
	RelistAfter      time.Time         `json:"relist_after,omitzero"`
	RemovedAt        time.Time         `json:"removed_at,omitzero"`
	Revision         int64             `json:"revision"`
}

// Function is one pinned function of a connector.
type Function struct {
	Tenant            TenantID        `json:"tenant"`
	Connector         ConnectorID     `json:"connector"`
	Name              string          `json:"name"`
	Definition        json.RawMessage `json:"definition"`
	DefinitionHash    Hash            `json:"definition_hash"`
	UIResourceHash    Hash            `json:"ui_resource_hash,omitempty"`
	Class             Class           `json:"class,omitempty"`
	ClassSource       ClassSource     `json:"class_source,omitempty"`
	State             FunctionState   `json:"state"`
	PreviousState     FunctionState   `json:"previous_state,omitempty"`
	Visibility        []string        `json:"visibility,omitempty"`
	OpenWorld         bool            `json:"open_world"`
	ReadsPrivate      *bool           `json:"reads_private,omitempty"`
	ReturnsThirdParty *bool           `json:"returns_third_party,omitempty"`
	Projection        bool            `json:"projection,omitempty"`
	AcceptedAt        time.Time       `json:"accepted_at,omitzero"`
	AcceptedBy        string          `json:"accepted_by,omitempty"`
}

// Binding gives an agent a connector; grants hang off it.
type Binding struct {
	Tenant           TenantID         `json:"tenant"`
	Workspace        WorkspaceID      `json:"workspace"`
	Connector        ConnectorID      `json:"connector"`
	Agent            AgentID          `json:"agent"`
	AgentOwner       PersonID         `json:"agent_owner,omitempty"`
	OwnerRemoved     bool             `json:"owner_removed,omitempty"`
	CredentialPolicy CredentialPolicy `json:"credential_policy"`
	AskRequester     bool             `json:"ask_requester"`
	Approvers        []string         `json:"approvers,omitempty"`
	ApprovalTimeout  time.Duration    `json:"approval_timeout,omitempty"`
}

// Grant is one row of the grant table.
type Grant struct {
	ID             GrantID     `json:"id"`
	Tenant         TenantID    `json:"tenant"`
	ScopeKind      ScopeKind   `json:"scope_kind"`
	ScopeID        string      `json:"scope_id"`
	Agent          AgentID     `json:"agent,omitempty"`
	Connector      ConnectorID `json:"connector,omitempty"`
	Listing        *ListingRef `json:"listing,omitempty"`
	Selector       string      `json:"selector"`
	Mode           Mode        `json:"mode"`
	DefinitionHash Hash        `json:"definition_hash,omitempty"`
	ExpiresAt      time.Time   `json:"expires_at,omitzero"`
	MaxUses        int         `json:"max_uses,omitempty"`
	Uses           int         `json:"uses,omitempty"`
	CreatedBy      string      `json:"created_by,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	Reason         string      `json:"reason,omitempty"`
	Origin         Origin      `json:"origin"`
}

// Key is the uniqueness key of a row: writing a row with an existing key replaces it.
func (g Grant) Key() string {
	target := string(g.Connector)
	if g.Listing != nil {
		target = "listing:" + g.Listing.String()
	}
	return strings.Join([]string{string(g.Tenant), string(g.ScopeKind), g.ScopeID, string(g.Agent), target, g.Selector}, "\x00")
}

// Selectors; a function selector is SelectorFunction followed by the name.
const (
	SelectorAll      = "*"
	SelectorClass    = "class:"
	SelectorFunction = "fn:"
)

// Item kinds: a function row whose name starts with one of these is a connector item, not a tool; * and class: never cover it.
const (
	ItemInstructions = "instructions:"
	ItemPrompt       = "prompt:"
	ItemResource     = "res:"
)

// InstructionsItem is the name of the item that pins a connector's server instructions.
const InstructionsItem = ItemInstructions + "server"

var itemKinds = []string{ItemInstructions, ItemPrompt, ItemResource}

// ItemKind returns the item kind a function name starts with, or "" for a tool.
func ItemKind(name string) string {
	for _, k := range itemKinds {
		if strings.HasPrefix(name, k) {
			return k
		}
	}
	return ""
}

// SelectorOf returns the selector that names one function or item exactly: fn:<name> for a tool, the item name itself for an item.
func SelectorOf(name string) string {
	if ItemKind(name) != "" {
		return name
	}
	return SelectorFunction + name
}

// Approval is one held call.
type Approval struct {
	ID                 ApprovalID    `json:"id"`
	Tenant             TenantID      `json:"tenant"`
	Workspace          WorkspaceID   `json:"workspace"`
	Agent              AgentID       `json:"agent"`
	Session            SessionID     `json:"session"`
	OnBehalfOf         PersonID      `json:"on_behalf_of,omitempty"`
	Connector          ConnectorID   `json:"connector"`
	Function           string        `json:"function"`
	DefinitionHash     Hash          `json:"definition_hash"`
	Class              Class         `json:"class,omitempty"`
	InnerOf            string        `json:"inner_of,omitempty"`
	Arguments          *Sealed       `json:"arguments,omitempty"`
	CallHash           Hash          `json:"call_hash"`
	Fields             []string      `json:"fields,omitempty"`
	Mode               Mode          `json:"mode"`
	Approvers          []string      `json:"approvers"`
	State              ApprovalState `json:"state"`
	CreatedAt          time.Time     `json:"created_at"`
	ExpiresAt          time.Time     `json:"expires_at"`
	DecidedBy          string        `json:"decided_by,omitempty"`
	DecidedAt          time.Time     `json:"decided_at,omitzero"`
	Channel            string        `json:"channel,omitempty"`
	DecisionNote       string        `json:"decision_note,omitempty"`
	SessionGrant       bool          `json:"session_grant,omitempty"`
	Reason             string        `json:"reason,omitempty"`
	DeliveredAt        time.Time     `json:"delivered_at,omitzero"`
	ExecutionStartedAt time.Time     `json:"execution_started_at,omitzero"`
	CallDeadline       time.Duration `json:"call_deadline,omitempty"`
	ResultRef          *Sealed       `json:"result_ref,omitempty"`
	ResultExpiresAt    time.Time     `json:"result_expires_at,omitzero"`
	TokenExpiresAt     time.Time     `json:"token_expires_at,omitzero"`
}

// Sealed is a record sealed by the SecretStore: the database never holds plaintext.
type Sealed struct {
	Ciphertext []byte `json:"ciphertext"`
	KeyRef     string `json:"key_ref"`
}

// Pause is the denial-brake row of (session, agent, connector).
type Pause struct {
	ID          PauseID     `json:"id"`
	Tenant      TenantID    `json:"tenant"`
	Session     SessionID   `json:"session"`
	Agent       AgentID     `json:"agent"`
	Connector   ConnectorID `json:"connector"`
	Paused      bool        `json:"paused"`
	PausedAt    time.Time   `json:"paused_at,omitzero"`
	Consecutive int         `json:"consecutive"`
	Total       int         `json:"total"`
	ExpiresAt   time.Time   `json:"expires_at,omitzero"`
}

// TombstoneKind says what a tombstone ends.
type TombstoneKind string

// Tombstone kinds.
const (
	TombstoneSession TombstoneKind = "session"
	TombstonePerson  TombstoneKind = "person"
)

// Tombstone records an ended session or a removed person for auth.agentTokenMaxLifetime.
type Tombstone struct {
	Tenant    TenantID      `json:"tenant"`
	Kind      TombstoneKind `json:"kind"`
	Subject   string        `json:"subject"`
	Workspace WorkspaceID   `json:"workspace,omitempty"`
	At        time.Time     `json:"at"`
	ExpiresAt time.Time     `json:"expires_at"`
}

// Live reports whether the tombstone still refuses at now.
func (t Tombstone) Live(now time.Time) bool { return t.ExpiresAt.IsZero() || now.Before(t.ExpiresAt) }

// Credential is one sealed upstream credential.
type Credential struct {
	ID                 CredentialID     `json:"id"`
	Tenant             TenantID         `json:"tenant"`
	OwnerKind          OwnerKind        `json:"owner_kind"`
	OwnerID            string           `json:"owner_id"`
	Connector          ConnectorID      `json:"connector"`
	Kind               CredentialKind   `json:"kind"`
	Issuer             string           `json:"issuer,omitempty"`
	Resource           string           `json:"resource,omitempty"`
	ClientRegistration Registration     `json:"client_registration,omitempty"`
	ScopesGranted      []string         `json:"scopes_granted,omitempty"`
	Secret             Sealed           `json:"secret"`
	ExpiresAt          time.Time        `json:"expires_at,omitzero"`
	RefreshAfter       time.Time        `json:"refresh_after,omitzero"`
	Status             CredentialStatus `json:"status"`
	CreatedBy          string           `json:"created_by,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	LastUsedAt         time.Time        `json:"last_used_at,omitzero"`
	LastRefreshedAt    time.Time        `json:"last_refreshed_at,omitzero"`
	Revision           int64            `json:"revision"`
}

// Overlay is one version of a title and description overlay of a function.
type Overlay struct {
	Tenant       TenantID      `json:"tenant"`
	Connector    ConnectorID   `json:"connector,omitempty"`
	Listing      *ListingRef   `json:"listing,omitempty"`
	Function     string        `json:"function"`
	Version      int           `json:"overlay_version"`
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description,omitempty"`
	UpstreamHash Hash          `json:"upstream_hash"`
	OverlayHash  Hash          `json:"overlay_hash"`
	State        OverlayState  `json:"state"`
	By           string        `json:"by"`
	Source       OverlaySource `json:"source"`
	At           time.Time     `json:"at"`
}

// Call is the audit row of a forwarded call, written before it runs.
type Call struct {
	ID          CallID         `json:"id"`
	Tenant      TenantID       `json:"tenant"`
	Workspace   WorkspaceID    `json:"workspace"`
	Agent       AgentID        `json:"agent"`
	Session     SessionID      `json:"session"`
	OnBehalfOf  PersonID       `json:"on_behalf_of,omitempty"`
	Connector   ConnectorID    `json:"connector"`
	Function    string         `json:"function"`
	Class       Class          `json:"class,omitempty"`
	CallHash    Hash           `json:"call_hash"`
	Approval    ApprovalID     `json:"approval,omitempty"`
	Credential  CredentialID   `json:"credential,omitempty"`
	OverlayHash Hash           `json:"overlay_hash,omitempty"`
	State       CallState      `json:"state"`
	StartedAt   time.Time      `json:"started_at"`
	Deadline    time.Time      `json:"deadline"`
	FinishedAt  time.Time      `json:"finished_at,omitzero"`
	ResultBytes int            `json:"result_bytes,omitempty"`
	IsError     bool           `json:"is_error,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Findings    map[string]int `json:"findings,omitempty"`
	LateAnswer  bool           `json:"late_answer,omitempty"`
}

// ClientRegistration is the OAuth client an admin registered for an issuer of one tenant; the secret is sealed under the tenant key.
type ClientRegistration struct {
	Tenant      TenantID  `json:"tenant"`
	Issuer      string    `json:"issuer"`
	ClientID    string    `json:"client_id"`
	Secret      *Sealed   `json:"secret,omitempty"`
	RedirectURL string    `json:"redirect_url,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SourceSync is what the last sync of a listing source did.
type SourceSync struct {
	Source   string    `json:"source"`
	LastSync time.Time `json:"last_sync"`
	Imported int       `json:"imported"`
	Skipped  int       `json:"skipped"`
	Error    string    `json:"error,omitempty"`
}

// Exposure is the trifecta check result of one agent.
type Exposure struct {
	Tenant  TenantID `json:"tenant"`
	Agent   AgentID  `json:"agent"`
	Exposed bool     `json:"exposed"`
	LegA    []string `json:"leg_a,omitempty"`
	LegB    []string `json:"leg_b,omitempty"`
	LegC    []string `json:"leg_c,omitempty"`
}

// Operation is the progress of a 202 operation; operator operations report counts only.
type Operation struct {
	ID        OperationID    `json:"id"`
	Tenant    TenantID       `json:"tenant,omitempty"`
	Kind      string         `json:"kind"`
	Done      bool           `json:"done"`
	Error     string         `json:"error,omitempty"`
	Counts    map[string]int `json:"counts,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FanOut is the installation work row of an operator's change that must reach tenant rows.
type FanOut struct {
	ID        OperationID `json:"id"`
	Kind      string      `json:"kind"`
	Listing   *ListingRef `json:"listing,omitempty"`
	Tenant    TenantID    `json:"tenant,omitempty"`
	Pending   []TenantID  `json:"pending,omitempty"`
	Done      bool        `json:"done"`
	CreatedAt time.Time   `json:"created_at"`
}

// WorkKind names a kind of due work the worker finds through the one cross-tenant view.
type WorkKind string

// Kinds of due work, the call-deadline sweep included.
const (
	WorkApprovalExpiry  WorkKind = "approval_expiry"
	WorkApprovedRecord  WorkKind = "approved_record"
	WorkExecuteDeadline WorkKind = "execute_deadline"
	WorkResultRetention WorkKind = "result_retention"
	WorkCallDeadline    WorkKind = "call_deadline"
	WorkGrantExpiry     WorkKind = "grant_expiry"
	WorkPauseExpiry     WorkKind = "pause_expiry"
)

// WorkItem is one row of the cross-tenant due-work view: tenant, kind, record id and due time only.
type WorkItem struct {
	Tenant TenantID  `json:"tenant"`
	Kind   WorkKind  `json:"kind"`
	ID     string    `json:"id"`
	Due    time.Time `json:"due"`
}

// AuditRow records who did what to which record.
type AuditRow struct {
	At      time.Time      `json:"at"`
	Tenant  TenantID       `json:"tenant,omitempty"`
	Actor   string         `json:"actor"`
	Action  string         `json:"action"`
	Subject string         `json:"subject,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// Event is one outbox row, delivered as a CloudEvent; the store assigns ID, Source, Stream and Sequence.
type Event struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Tenant      TenantID       `json:"tenant,omitempty"`
	Source      string         `json:"source"`
	Subject     string         `json:"subject,omitempty"`
	Stream      string         `json:"stream"`
	Sequence    int64          `json:"sequence"`
	Time        time.Time      `json:"time"`
	TraceParent string         `json:"traceparent,omitempty"`
	Data        map[string]any `json:"data"`
}

// SourcePrefix starts the CloudEvents source of every event; the tenant or "installation" follows.
const SourcePrefix = "connector-engine/"

// SourceOf returns the stream of a tenant, or of the installation for the empty tenant.
func SourceOf(t TenantID) string {
	if t == Installation {
		return SourcePrefix + "installation"
	}
	return SourcePrefix + string(t)
}

// RoleAssignment gives the person of an issuer a role over a resource scope; an empty Tenant is an installation role.
// An installation role names its issuer; a tenant role without one matches the person of any issuer.
type RoleAssignment struct {
	Tenant TenantID `json:"tenant,omitempty"`
	Issuer string   `json:"issuer,omitempty"`
	Person PersonID `json:"person"`
	Role   string   `json:"role"`
	Scope  string   `json:"scope"`
}

// GroupMember makes a person a member of a group of a tenant.
type GroupMember struct {
	Tenant TenantID `json:"tenant"`
	Person PersonID `json:"person"`
	Group  GroupID  `json:"group"`
}
