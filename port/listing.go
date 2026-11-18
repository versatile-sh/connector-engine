package port

import (
	"encoding/json"
	"time"
)

// Manifest is one immutable listing version document (§5.2).
type Manifest struct {
	Owner            TenantID      `json:"owner,omitempty"`
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	Title            string        `json:"title,omitempty"`
	Description      string        `json:"description,omitempty"`
	Icon             string        `json:"icon,omitempty"`
	Kind             Kind          `json:"kind"`
	Source           *SourceInfo   `json:"source,omitempty"`
	Remote           *Remote       `json:"remote,omitempty"`
	Artifact         *Artifact     `json:"artifact,omitempty"`
	REST             *REST         `json:"rest,omitempty"`
	GRPC             *GRPC         `json:"grpc,omitempty"`
	Signature        *Signature    `json:"signature,omitempty"`
	Functions        []FunctionDef `json:"functions,omitempty"`
	InstructionsHash Hash          `json:"instructions_hash,omitempty"`
	Auth             []AuthSpec    `json:"auth,omitempty"`
	Config           []ConfigField `json:"config,omitempty"`
	Egress           *Egress       `json:"egress,omitempty"`
	Runtime          *RuntimeNeeds `json:"runtime,omitempty"`
	UI               *UI           `json:"ui,omitempty"`
	Publisher        Publisher     `json:"publisher"`
	Licence          string        `json:"licence,omitempty"`
	Review           *Review       `json:"review,omitempty"`
	TrustLevel       TrustLevel    `json:"trust_level,omitempty"`
}

// SourceInfo is the provenance of an imported listing.
type SourceInfo struct {
	Registry        string    `json:"registry"`
	UpstreamName    string    `json:"upstream_name,omitempty"`
	UpstreamVersion string    `json:"upstream_version,omitempty"`
	Status          string    `json:"status,omitempty"`
	ImportedAt      time.Time `json:"imported_at,omitempty"`
}

// PrivateSource is the source the engine sets on every private listing (§5.2).
const PrivateSource = "private"

// Remote is where an mcp_remote connector is reached.
type Remote struct {
	URL       string `json:"url"`
	Transport string `json:"transport"`
}

// Artifact is what an mcp_package connector runs; the digest is required.
type Artifact struct {
	Type   string `json:"type"`
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

// REST is the input of the declarative REST executor.
type REST struct {
	BaseURL   string         `json:"base_url"`
	Functions []RESTFunction `json:"functions"`
}

// RESTFunction is one declared REST function.
type RESTFunction struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// GRPC is the input of the declarative gRPC executor (OD-44).
type GRPC struct {
	Target     string         `json:"target"`
	Descriptor Descriptor     `json:"descriptor"`
	Services   []string       `json:"services,omitempty"`
	Methods    []GRPCFunction `json:"methods"`
}

// Descriptor says where a gRPC descriptor set comes from: reflection, file_descriptor_set or bsr.
type Descriptor struct {
	From   string `json:"from"`
	Ref    string `json:"ref,omitempty"`
	Digest string `json:"digest,omitempty"`
}

// Descriptor sources of §5.2.
const (
	DescriptorReflection = "reflection"
	DescriptorFileSet    = "file_descriptor_set"
	DescriptorBSR        = "bsr"
)

// GRPCFunction is one declared unary method, named <Service>_<Method>.
type GRPCFunction struct {
	Name         string          `json:"name"`
	FullMethod   string          `json:"full_method"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// Signature is the declared signature of a listing.
type Signature struct {
	Type     string `json:"type"`
	Identity string `json:"identity,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
}

// FunctionDef is one pinned function of a listing version, captured or declared.
type FunctionDef struct {
	Name              string          `json:"name"`
	Title             string          `json:"title,omitempty"`
	Description       string          `json:"description,omitempty"`
	InputSchema       json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema      json.RawMessage `json:"output_schema,omitempty"`
	Annotations       *Annotations    `json:"annotations,omitempty"`
	MetaUI            *MetaUI         `json:"meta_ui,omitempty"`
	DefinitionHash    Hash            `json:"definition_hash,omitempty"`
	Class             Class           `json:"class,omitempty"`
	ClassSource       ClassSource     `json:"class_source,omitempty"`
	ReadsPrivate      *bool           `json:"reads_private,omitempty"`
	ReturnsThirdParty *bool           `json:"returns_third_party,omitempty"`
	Idempotent        bool            `json:"idempotent,omitempty"`
	NeedsReason       bool            `json:"needs_reason,omitempty"`
	Redact            []string        `json:"redact,omitempty"`
}

// Annotations are the MCP tool hints; a nil pointer is an absent hint.
type Annotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

// MetaUI is the MCP Apps _meta.ui of a function.
type MetaUI struct {
	ResourceURI string   `json:"resourceUri,omitempty"`
	Visibility  []string `json:"visibility,omitempty"`
}

// AuthSpec says what a listing needs to connect (§5.2, §11).
type AuthSpec struct {
	Kind              CredentialKind      `json:"kind"`
	Issuer            string              `json:"issuer,omitempty"`
	Resource          string              `json:"resource,omitempty"`
	ScopesByFunction  map[string][]string `json:"scopes_by_function,omitempty"`
	Registration      Registration        `json:"registration,omitempty"`
	OwnerKindsAllowed []OwnerKind         `json:"owner_kinds_allowed,omitempty"`
}

// ConfigField is one field of the install form.
type ConfigField struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Sensitive bool   `json:"sensitive,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

// Egress is the network policy input of a listing.
type Egress struct {
	AllowHosts []string `json:"allow_hosts"`
}

// RuntimeNeeds is the placement input of a listing.
type RuntimeNeeds struct {
	IsolationMin string `json:"isolation_min,omitempty"`
	CPU          string `json:"cpu,omitempty"`
	Memory       string `json:"memory,omitempty"`
	LongLived    bool   `json:"long_lived,omitempty"`
}

// UI marks a listing as an MCP app.
type UI struct {
	MCPApps bool `json:"mcp_apps"`
}

// Publisher is who publishes a listing; for a private listing the engine sets the tenant.
type Publisher struct {
	Name       string `json:"name"`
	VerifiedBy string `json:"verified_by,omitempty"`
	Contact    string `json:"contact,omitempty"`
}

// Review makes the class map of a version trusted at the hashes it covers.
type Review struct {
	By               string    `json:"by"`
	Scope            TenantID  `json:"scope,omitempty"`
	At               time.Time `json:"at"`
	DefinitionHashes []Hash    `json:"definition_hashes"`
	Note             string    `json:"note,omitempty"`
}

// Listing is the (owner, name) row: its own status and, for an installation listing, the starter mark.
type Listing struct {
	Ref       ListingRef    `json:"ref"`
	Source    string        `json:"source"`
	Publisher string        `json:"publisher"`
	Status    ListingStatus `json:"status"`
	Starter   bool          `json:"starter,omitempty"`
	Versions  []string      `json:"versions,omitempty"`
}

// ListingVersion is an immutable manifest with the state kept beside it.
type ListingVersion struct {
	Ref        VersionRef    `json:"ref"`
	Manifest   Manifest      `json:"manifest"`
	Status     ListingStatus `json:"status"`
	ImportedAt time.Time     `json:"imported_at"`
}

// Offer is the operator's decision that a source or one installation listing is withheld from a tenant.
type Offer struct {
	Tenant     TenantID   `json:"tenant"`
	TargetKind TargetKind `json:"target_kind"`
	Target     string     `json:"target"`
	Withheld   bool       `json:"withheld"`
}

// CatalogPolicy is the default of one workspace; there is none for the tenant (§5.5).
type CatalogPolicy struct {
	Tenant    TenantID      `json:"tenant"`
	Workspace WorkspaceID   `json:"workspace"`
	Default   PolicyDefault `json:"default"`
}

// CatalogRule shows or hides listings to a subject of a tenant at a ceiling (§5.5).
type CatalogRule struct {
	ID          RuleID      `json:"id"`
	Tenant      TenantID    `json:"tenant"`
	SubjectKind SubjectKind `json:"subject_kind"`
	SubjectID   string      `json:"subject_id"`
	TargetKind  TargetKind  `json:"target_kind"`
	Target      string      `json:"target"`
	Show        bool        `json:"show"`
	Install     bool        `json:"install"`
	Ceiling     Ceiling     `json:"ceiling"`
	Audience    Audience    `json:"audience,omitempty"`
	CreatedBy   string      `json:"created_by,omitempty"`
}

// StarterCreator marks the rules createTenant writes from the starter set (§5.5 V2).
const StarterCreator = "starter"
