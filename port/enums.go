package port

// Mode is the answer of a grant row or of the evaluation, in order of strictness (§6.1).
type Mode string

// Modes of §6.1; Inherit exists only on a tenant fn: row (§6.4 G10).
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

// Class is the trusted kind of a function; the empty class is unclassified and counts as destructive (§5.2).
type Class string

// Classes of §5.2.
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

// Raises reports whether moving from c to n raises the effective class (§6.4 G7).
func (c Class) Raises(n Class) bool { return classRank[n] > classRank[c] }

// ClassSource says where the class of a function came from (§5.2).
type ClassSource string

// Class sources of §5.2, in the order they are trusted.
const (
	ClassSourceAdmin           ClassSource = "admin"
	ClassSourceReviewed        ClassSource = "reviewed"
	ClassSourceAnnotationRaise ClassSource = "annotation_raise"
	ClassSourceDefault         ClassSource = "default"
)

// FunctionState is the lifecycle state of a function of a connector (§5.3).
type FunctionState string

// Function states of §5.3.
const (
	FunctionActive      FunctionState = "active"
	FunctionQuarantined FunctionState = "quarantined"
	FunctionRemoved     FunctionState = "removed"
	FunctionDisabled    FunctionState = "disabled"
)

// Caller is the kind of client that sends a request on the agent endpoint (§4.3).
type Caller string

// Callers of §4.3; an absent caller claim is a model.
const (
	CallerModel Caller = "model"
	CallerView  Caller = "view"
)

// Visibility values of a pinned function (_meta ui.visibility).
const (
	VisibleToModel = "model"
	VisibleToApp   = "app"
)

// ScopeKind is the layer a grant row belongs to (§6.1).
type ScopeKind string

// Grant scopes of §6.1.
const (
	ScopeTenant    ScopeKind = "tenant"
	ScopeWorkspace ScopeKind = "workspace"
	ScopeAgent     ScopeKind = "agent"
	ScopeUser      ScopeKind = "user"
	ScopeSession   ScopeKind = "session"
)

// Origin says how a grant row was written (§6.1).
type Origin string

// Origins of §6.1.
const (
	OriginSettings Origin = "settings"
	OriginLevel    Origin = "level"
	OriginApproval Origin = "approval"
)

// Level is a named set of class rows (§6.2).
type Level string

// Levels of §6.2.
const (
	LevelOff    Level = "off"
	LevelRead   Level = "read"
	LevelWrite  Level = "write"
	LevelCustom Level = "custom"
)

// Ceiling is the catalog ceiling a rule gives a listing (§5.5 V7).
type Ceiling string

// Ceilings of §5.5, from lowest to highest.
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

// ListingStatus is the status of a listing or of a listing version, from least to most strict (§5.2).
type ListingStatus string

// Listing statuses of §5.2.
const (
	ListingActive      ListingStatus = "active"
	ListingDeprecated  ListingStatus = "deprecated"
	ListingQuarantined ListingStatus = "quarantined"
	ListingDeleted     ListingStatus = "deleted"
)

var statusRank = map[ListingStatus]int{ListingActive: 0, ListingDeprecated: 1, ListingQuarantined: 2, ListingDeleted: 3}

// EffectiveStatus is the stricter of a version's own status and its listing's (§5.2).
func EffectiveStatus(listing, version ListingStatus) ListingStatus {
	if statusRank[version] > statusRank[listing] {
		return version
	}
	return listing
}

// Exists reports whether a version of this effective status exists for a tenant (§5.5 V1).
func (s ListingStatus) Exists() bool { return s == ListingActive || s == ListingDeprecated }

// TrustLevel is the computed store badge of a listing version (§5.2).
type TrustLevel string

// Trust levels of §5.2.
const (
	TrustUnreviewed        TrustLevel = "unreviewed"
	TrustNamespaceVerified TrustLevel = "namespace_verified"
	TrustSigned            TrustLevel = "signed"
	TrustReviewed          TrustLevel = "reviewed"
	TrustFirstParty        TrustLevel = "first_party"
)

// Kind selects the runtime adapter of a listing (§5.2).
type Kind string

// Listing kinds of §5.2; Piece and WasmComponent are reserved.
const (
	KindMCPRemote       Kind = "mcp_remote"
	KindMCPPackage      Kind = "mcp_package"
	KindRESTDeclarative Kind = "rest_declarative"
	KindGRPCDeclarative Kind = "grpc_declarative"
	KindPiece           Kind = "piece"
	KindWasmComponent   Kind = "wasm_component"
)

// SubjectKind is who a catalog rule or policy is about (§5.5).
type SubjectKind string

// Catalog subjects of §5.5.
const (
	SubjectTenant    SubjectKind = "tenant"
	SubjectWorkspace SubjectKind = "workspace"
	SubjectGroup     SubjectKind = "group"
	SubjectUser      SubjectKind = "user"
)

// TargetKind is what a catalog rule or offer names (§5.5), from least to most specific.
type TargetKind string

// Catalog targets of §5.5.
const (
	TargetSource    TargetKind = "source"
	TargetPublisher TargetKind = "publisher"
	TargetListing   TargetKind = "listing"
)

// Audience says whether a tenant or workspace rule shows a listing to all or only to named subjects (§5.5 V8).
type Audience string

// Audiences of §5.5 V8.
const (
	AudienceAll   Audience = "all"
	AudienceNamed Audience = "named"
)

// PolicyDefault is the catalog policy default of a workspace (§5.5).
type PolicyDefault string

// Policy defaults of §5.5.
const (
	PolicyVisible PolicyDefault = "visible"
	PolicyHidden  PolicyDefault = "hidden"
)

// CredentialPolicy names which owner's credential a binding uses (§6.3, §11.2).
type CredentialPolicy string

// Credential policies of §6.3.
const (
	CredentialPerUser  CredentialPolicy = "per_user"
	CredentialPerAgent CredentialPolicy = "per_agent"
	CredentialShared   CredentialPolicy = "shared"
)

// OwnerKind is who owns a credential (§11.1).
type OwnerKind string

// Credential owners of §11.1.
const (
	OwnerUser      OwnerKind = "user"
	OwnerWorkspace OwnerKind = "workspace"
	OwnerAgent     OwnerKind = "agent"
)

// CredentialKind is the kind of an upstream credential (§11.1).
type CredentialKind string

// Credential kinds of §11.1.
const (
	CredentialOAuth2User              CredentialKind = "oauth2_user"
	CredentialOAuth2ClientCredentials CredentialKind = "oauth2_client_credentials"
	CredentialIDJAG                   CredentialKind = "id_jag"
	CredentialAPIKey                  CredentialKind = "api_key"
	CredentialBasic                   CredentialKind = "basic"
	CredentialCustomHeaders           CredentialKind = "custom_headers"
)

// Registration is how the OAuth client of an issuer was obtained (§11.1).
type Registration string

// Client registrations of §11.1.
const (
	RegistrationPreRegistered Registration = "pre_registered"
	RegistrationCIMD          Registration = "cimd"
	RegistrationDCR           Registration = "dcr"
	RegistrationManual        Registration = "manual"
)

// CredentialStatus is the state of a credential (§11.1).
type CredentialStatus string

// Credential statuses of §11.1.
const (
	CredentialActive      CredentialStatus = "active"
	CredentialNeedsReauth CredentialStatus = "needs_reauth"
	CredentialRevoked     CredentialStatus = "revoked"
	CredentialFailed      CredentialStatus = "failed"
)

// ApprovalState is the state of an approval record (§8.2).
type ApprovalState string

// Approval states of §8.2.
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

// Open reports whether the record is pending, approved or executing (§8.1).
func (s ApprovalState) Open() bool {
	return s == ApprovalPending || s == ApprovalApproved || s == ApprovalExecuting
}

// Terminal reports whether no transition but A13 and A14 leaves the state.
func (s ApprovalState) Terminal() bool { return s != "" && !s.Open() }

// Decision is what an approver answers (§8.2 A3 to A5).
type Decision string

// Decisions of §8.2.
const (
	DecisionApprove           Decision = "approve"
	DecisionApproveForSession Decision = "approve_for_session"
	DecisionDeny              Decision = "deny"
)

// CallState is the state of the audit row of a forwarded call (§10.3 C8 to C12).
type CallState string

// Call states of §10.3.
const (
	CallStarted        CallState = "started"
	CallFinished       CallState = "finished"
	CallFailed         CallState = "failed"
	CallOutcomeUnknown CallState = "outcome_unknown"
)

// OverlayState is the state of a description overlay version (§9.1).
type OverlayState string

// Overlay states of §9.1.
const (
	OverlaySuggested  OverlayState = "suggested"
	OverlayProposed   OverlayState = "proposed"
	OverlayActive     OverlayState = "active"
	OverlaySuperseded OverlayState = "superseded"
)

// OverlaySource says who wrote an overlay (§9.1).
type OverlaySource string

// Overlay sources of §9.1.
const (
	OverlayByPerson OverlaySource = "person"
	OverlayByModel  OverlaySource = "model"
)

// TenantState is the registry state of a tenant (§4.4).
type TenantState string

// Tenant states of §4.4.
const (
	TenantActive   TenantState = "active"
	TenantRemoving TenantState = "removing"
	TenantRemoved  TenantState = "removed"
)
