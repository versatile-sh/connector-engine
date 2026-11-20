package port

// Mode is the answer of a grant row or of the evaluation, in order of strictness.
type Mode string

// Modes; Inherit exists only on a tenant fn: row.
const (
	ModeDeny    Mode = "deny"
	ModeAsk     Mode = "ask"
	ModeAskOnce Mode = "ask_once"
	ModeAllow   Mode = "allow"
	ModeInherit Mode = "inherit"
)

var modeRank = map[Mode]int{ModeDeny: 0, ModeAsk: 1, ModeAskOnce: 2, ModeAllow: 3}

// Rank orders the four grant modes from deny (0) to allow (3); Inherit and unknown values rank -1.
func (m Mode) Rank() int {
	if r, ok := modeRank[m]; ok {
		return r
	}
	return -1
}

// Stricter returns the stricter of m and o.
func (m Mode) Stricter(o Mode) Mode {
	if o.Rank() < m.Rank() {
		return o
	}
	return m
}

// Class is the trusted kind of a function; the empty class is unclassified and counts as destructive.
type Class string

// Classes.
const (
	ClassNone        Class = ""
	ClassRead        Class = "read"
	ClassWrite       Class = "write"
	ClassDestructive Class = "destructive"
)

var classRank = map[Class]int{ClassRead: 0, ClassWrite: 1, ClassDestructive: 2, ClassNone: 2}

// Effective returns the class the evaluation uses: destructive for an unclassified function.
func (c Class) Effective() Class {
	if c == ClassNone {
		return ClassDestructive
	}
	return c
}

// Raises reports whether moving from c to n raises the effective class.
func (c Class) Raises(n Class) bool { return classRank[n] > classRank[c] }

// ClassSource says where the class of a function came from.
type ClassSource string

// Class sources, in the order they are trusted.
const (
	ClassSourceAdmin           ClassSource = "admin"
	ClassSourceReviewed        ClassSource = "reviewed"
	ClassSourceAnnotationRaise ClassSource = "annotation_raise"
	ClassSourceDefault         ClassSource = "default"
	ClassSourceItem            ClassSource = "item"
)

// FunctionState is the lifecycle state of a function of a connector.
type FunctionState string

// Function states.
const (
	FunctionActive      FunctionState = "active"
	FunctionQuarantined FunctionState = "quarantined"
	FunctionRemoved     FunctionState = "removed"
	FunctionDisabled    FunctionState = "disabled"
)

// Caller is the kind of client that sends a request on the agent endpoint.
type Caller string

// Callers; an absent caller claim is a model.
const (
	CallerModel Caller = "model"
	CallerView  Caller = "view"
)

// Visibility values of a pinned function (_meta ui.visibility).
const (
	VisibleToModel = "model"
	VisibleToApp   = "app"
)

// ScopeKind is the layer a grant row belongs to.
type ScopeKind string

// Grant scopes.
const (
	ScopeTenant    ScopeKind = "tenant"
	ScopeWorkspace ScopeKind = "workspace"
	ScopeAgent     ScopeKind = "agent"
	ScopeUser      ScopeKind = "user"
	ScopeSession   ScopeKind = "session"
)

// Origin says how a grant row was written.
type Origin string

// Origins.
const (
	OriginSettings Origin = "settings"
	OriginLevel    Origin = "level"
	OriginApproval Origin = "approval"
)

// Level is a named set of class rows.
type Level string

// Levels.
const (
	LevelOff    Level = "off"
	LevelRead   Level = "read"
	LevelWrite  Level = "write"
	LevelCustom Level = "custom"
)

// Ceiling is the catalog ceiling a rule gives a listing.
type Ceiling string

// Ceilings, from lowest to highest.
const (
	CeilingRead  Ceiling = "read"
	CeilingWrite Ceiling = "write"
	CeilingFull  Ceiling = "full"
)

var ceilingRank = map[Ceiling]int{CeilingRead: 0, CeilingWrite: 1, CeilingFull: 2}

// Lower returns the lower of c and o.
func (c Ceiling) Lower(o Ceiling) Ceiling {
	if ceilingRank[o] < ceilingRank[c] {
		return o
	}
	return c
}

// Valid reports whether c is one of the three ceilings.
func (c Ceiling) Valid() bool { _, ok := ceilingRank[c]; return ok }

// ListingStatus is the status of a listing or of a listing version, from least to most strict.
type ListingStatus string

// Listing statuses.
const (
	ListingActive      ListingStatus = "active"
	ListingDeprecated  ListingStatus = "deprecated"
	ListingQuarantined ListingStatus = "quarantined"
	ListingDeleted     ListingStatus = "deleted"
)

var statusRank = map[ListingStatus]int{ListingActive: 0, ListingDeprecated: 1, ListingQuarantined: 2, ListingDeleted: 3}

// EffectiveStatus is the stricter of a version's own status and its listing's.
func EffectiveStatus(listing, version ListingStatus) ListingStatus {
	if statusRank[version] > statusRank[listing] {
		return version
	}
	return listing
}

// Exists reports whether a version of this effective status exists for a tenant.
func (s ListingStatus) Exists() bool { return s == ListingActive || s == ListingDeprecated }

// TrustLevel is the computed store badge of a listing version.
type TrustLevel string

// Trust levels.
const (
	TrustUnreviewed        TrustLevel = "unreviewed"
	TrustNamespaceVerified TrustLevel = "namespace_verified"
	TrustSigned            TrustLevel = "signed"
	TrustReviewed          TrustLevel = "reviewed"
	TrustFirstParty        TrustLevel = "first_party"
)

// Kind selects the runtime adapter of a listing.
type Kind string

// Listing kinds and the skill kind; Piece and WasmComponent are reserved.
const (
	KindMCPRemote       Kind = "mcp_remote"
	KindMCPPackage      Kind = "mcp_package"
	KindRESTDeclarative Kind = "rest_declarative"
	KindGRPCDeclarative Kind = "grpc_declarative"
	KindSkill           Kind = "skill"
	KindPiece           Kind = "piece"
	KindWasmComponent   Kind = "wasm_component"
)

// SubjectKind is who a catalog rule or policy is about.
type SubjectKind string

// Catalog subjects.
const (
	SubjectTenant    SubjectKind = "tenant"
	SubjectWorkspace SubjectKind = "workspace"
	SubjectGroup     SubjectKind = "group"
	SubjectUser      SubjectKind = "user"
)

// TargetKind is what a catalog rule or offer names, from least to most specific.
type TargetKind string

// Catalog targets.
const (
	TargetSource    TargetKind = "source"
	TargetPublisher TargetKind = "publisher"
	TargetListing   TargetKind = "listing"
)

// Audience says whether a tenant or workspace rule shows a listing to all or only to named subjects.
type Audience string

// Audiences V8.
const (
	AudienceAll   Audience = "all"
	AudienceNamed Audience = "named"
)

// PolicyDefault is the catalog policy default of a workspace.
type PolicyDefault string

// Policy defaults.
const (
	PolicyVisible PolicyDefault = "visible"
	PolicyHidden  PolicyDefault = "hidden"
)

// CredentialPolicy names which owner's credential a binding uses.
type CredentialPolicy string

// Credential policies.
const (
	CredentialPerUser  CredentialPolicy = "per_user"
	CredentialPerAgent CredentialPolicy = "per_agent"
	CredentialShared   CredentialPolicy = "shared"
)

// OwnerKind is who owns a credential.
type OwnerKind string

// Credential owners.
const (
	OwnerUser      OwnerKind = "user"
	OwnerWorkspace OwnerKind = "workspace"
	OwnerAgent     OwnerKind = "agent"
)

// CredentialKind is the kind of an upstream credential.
type CredentialKind string

// Credential kinds.
const (
	CredentialOAuth2User              CredentialKind = "oauth2_user"
	CredentialOAuth2ClientCredentials CredentialKind = "oauth2_client_credentials"
	CredentialIDJAG                   CredentialKind = "id_jag"
	CredentialAPIKey                  CredentialKind = "api_key"
	CredentialBasic                   CredentialKind = "basic"
	CredentialCustomHeaders           CredentialKind = "custom_headers"
)

// Registration is how the OAuth client of an issuer was obtained.
type Registration string

// Client registrations.
const (
	RegistrationPreRegistered Registration = "pre_registered"
	RegistrationCIMD          Registration = "cimd"
	RegistrationDCR           Registration = "dcr"
	RegistrationManual        Registration = "manual"
)

// CredentialStatus is the state of a credential.
type CredentialStatus string

// Credential statuses.
const (
	CredentialActive      CredentialStatus = "active"
	CredentialNeedsReauth CredentialStatus = "needs_reauth"
	CredentialRevoked     CredentialStatus = "revoked"
	CredentialFailed      CredentialStatus = "failed"
)

// ApprovalState is the state of an approval record.
type ApprovalState string

// Approval states.
const (
	ApprovalPending        ApprovalState = "pending"
	ApprovalApproved       ApprovalState = "approved"
	ApprovalExecuting      ApprovalState = "executing"
	ApprovalExecuted       ApprovalState = "executed"
	ApprovalFailed         ApprovalState = "failed"
	ApprovalOutcomeUnknown ApprovalState = "outcome_unknown"
	ApprovalDenied         ApprovalState = "denied"
	ApprovalExpired        ApprovalState = "expired"
	ApprovalCancelled      ApprovalState = "cancelled"
)

// Open reports whether the record is pending, approved or executing.
func (s ApprovalState) Open() bool {
	return s == ApprovalPending || s == ApprovalApproved || s == ApprovalExecuting
}

// Terminal reports whether no transition but A13 and A14 leaves the state.
func (s ApprovalState) Terminal() bool { return s != "" && !s.Open() }

// Decision is what an approver answers.
type Decision string

// Decisions.
const (
	DecisionApprove           Decision = "approve"
	DecisionApproveForSession Decision = "approve_for_session"
	DecisionDeny              Decision = "deny"
)

// CallState is the state of the audit row of a forwarded call.
type CallState string

// Call states.
const (
	CallStarted        CallState = "started"
	CallFinished       CallState = "finished"
	CallFailed         CallState = "failed"
	CallOutcomeUnknown CallState = "outcome_unknown"
)

// OverlayState is the state of a description overlay version.
type OverlayState string

// Overlay states.
const (
	OverlaySuggested  OverlayState = "suggested"
	OverlayProposed   OverlayState = "proposed"
	OverlayActive     OverlayState = "active"
	OverlaySuperseded OverlayState = "superseded"
)

// OverlaySource says who wrote an overlay.
type OverlaySource string

// Overlay sources.
const (
	OverlayByPerson OverlaySource = "person"
	OverlayByModel  OverlaySource = "model"
)

// TenantState is the registry state of a tenant.
type TenantState string

// Tenant states.
const (
	TenantActive   TenantState = "active"
	TenantRemoving TenantState = "removing"
	TenantRemoved  TenantState = "removed"
)
