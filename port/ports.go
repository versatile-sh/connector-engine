package port

import (
	"context"
	"encoding/json"
	"time"
)

// Clock is the time source every use case and the worker read.
type Clock interface {
	Now() time.Time
}

// EventSink delivers outbox rows as CloudEvents signed as Standard Webhooks; one sink per installation.
type EventSink interface {
	Deliver(ctx context.Context, events []Event) error
}

// PrincipalVerifier verifies the bearer token of a request for one audience and returns the principal.
type PrincipalVerifier interface {
	Verify(ctx context.Context, token, audience string) (Principal, error)
}

// Query is the AuthZEN-shaped question of one function; Arguments is nil for the list-time query.
type Query struct {
	Tenant     TenantID        `json:"tenant"`
	Workspace  WorkspaceID     `json:"workspace"`
	Agent      AgentID         `json:"agent"`
	AgentOwner PersonID        `json:"agent_owner,omitempty"`
	OnBehalfOf PersonID        `json:"on_behalf_of,omitempty"`
	Caller     Caller          `json:"caller"`
	Session    SessionID       `json:"session"`
	Connector  ConnectorID     `json:"connector"`
	Function   string          `json:"function"`
	Class      Class           `json:"class,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
}

// AccessDecision is the mode of one function and the step, reason and rows that gave it.
type AccessDecision struct {
	Mode    Mode      `json:"mode"`
	Reason  Code      `json:"reason,omitempty"`
	Step    int       `json:"step"`
	Notes   []Code    `json:"notes,omitempty"`
	Matched []GrantID `json:"matched,omitempty"`
}

// FunctionDecision is the answer of Search for one function of a connector.
type FunctionDecision struct {
	Function string         `json:"function"`
	Decision AccessDecision `json:"decision"`
}

// Subject is who Search answers for: the claims of one per-run token.
type Subject struct {
	Tenant     TenantID    `json:"tenant"`
	Workspace  WorkspaceID `json:"workspace"`
	Agent      AgentID     `json:"agent"`
	AgentOwner PersonID    `json:"agent_owner,omitempty"`
	OnBehalfOf PersonID    `json:"on_behalf_of,omitempty"`
	Session    SessionID   `json:"session"`
	Caller     Caller      `json:"caller"`
}

// Permission is a question: may principal act on resource; the engine names the permission.
type Permission struct {
	Principal Principal `json:"principal"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
}

// Authorizer decides calls (Evaluate, Search, AuthZEN-shaped) and answers the role and group questions.
type Authorizer interface {
	Evaluate(ctx context.Context, q Query) (AccessDecision, error)
	Search(ctx context.Context, s Subject, connector ConnectorID) ([]FunctionDecision, error)
	Allowed(ctx context.Context, p Permission) (bool, error)
	Groups(ctx context.Context, tenant TenantID, person PersonID) ([]GroupID, error)
}

// ListingPage is one page of a ListingSource; Next is empty on the last page.
type ListingPage struct {
	Manifests []Manifest `json:"manifests"`
	Next      string     `json:"next,omitempty"`
}

// ListingSource lists and fetches listings from one catalog; every import is unreviewed.
type ListingSource interface {
	Name() string
	List(ctx context.Context, cursor string) (ListingPage, error)
	Fetch(ctx context.Context, name, version string) (Manifest, error)
}

// Verification is what an ArtifactVerifier established about a listing version.
type Verification struct {
	DigestVerified bool   `json:"digest_verified"`
	Signed         bool   `json:"signed"`
	Identity       string `json:"identity,omitempty"`
}

// ArtifactVerifier verifies the digest and, when present, the signature of a listing version.
type ArtifactVerifier interface {
	Verify(ctx context.Context, m Manifest, artifact []byte) (Verification, error)
}

// RuntimeCapabilities is what a runtime adapter states.
type RuntimeCapabilities struct {
	Kinds         []Kind `json:"kinds"`
	Isolation     string `json:"isolation"`
	Egress        string `json:"egress"`
	SecretVisible bool   `json:"secret_visible"`
	Processes     int    `json:"processes"`
	UnaryOnly     bool   `json:"unary_only,omitempty"`
}

// SecretsRef names the credential a runtime injects through the CredentialInjector; never its plaintext.
type SecretsRef struct {
	Tenant     TenantID     `json:"tenant,omitempty"`
	Credential CredentialID `json:"credential,omitempty"`
	Owner      OwnerKind    `json:"owner,omitempty"`
}

// Handle names one started connector instance.
type Handle string

// RawDefinition keeps a tool definition as received, so a hash sees an absent field and an explicit false apart.
type RawDefinition struct {
	Name string          `json:"name"`
	Raw  json.RawMessage `json:"raw"`
}

// ListMeta is what a listing of functions said besides the definitions.
type ListMeta struct {
	TTL             time.Duration     `json:"ttl,omitempty"`
	Instructions    string            `json:"instructions,omitempty"`
	ProtocolVersion string            `json:"protocol_version,omitempty"`
	UIResources     map[string][]byte `json:"ui_resources,omitempty"`
}

// Result is a function's answer: content blocks and structured content as received.
type Result struct {
	Content    json.RawMessage `json:"content,omitempty"`
	Structured json.RawMessage `json:"structured_content,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
}

// ConnectorRuntime starts a connector instance, lists its functions and calls one.
type ConnectorRuntime interface {
	Describe() RuntimeCapabilities
	Start(ctx context.Context, c Connector, m Manifest, secrets SecretsRef) (Handle, error)
	ListFunctions(ctx context.Context, h Handle) ([]RawDefinition, ListMeta, error)
	Call(ctx context.Context, h Handle, function string, arguments json.RawMessage, deadline time.Time) (Result, error)
	Stop(ctx context.Context, h Handle) error
}

// ContentRuntime is the optional part of a ConnectorRuntime that lists a connector's prompts, resources and templates as items and serves them.
type ContentRuntime interface {
	ListContent(ctx context.Context, h Handle) ([]RawDefinition, error)
	Content(ctx context.Context, h Handle, item string, arguments json.RawMessage, deadline time.Time) (Result, error)
	Complete(ctx context.Context, h Handle, item string, request json.RawMessage) (json.RawMessage, error)
}

// OAuthClient is the client of one issuer, however it was registered.
type OAuthClient struct {
	Issuer       string       `json:"issuer"`
	ClientID     string       `json:"client_id"`
	ClientSecret []byte       `json:"-"`
	Registration Registration `json:"registration"`
	RedirectURL  string       `json:"redirect_url,omitempty"`
}

// AuthorizationRequest begins an authorization for one bound person.
type AuthorizationRequest struct {
	Client   OAuthClient `json:"client"`
	Resource string      `json:"resource,omitempty"`
	Scopes   []string    `json:"scopes,omitempty"`
	State    string      `json:"state"`
	Verifier string      `json:"-"`
}

// TokenSet is the material an authorization or a refresh returns.
type TokenSet struct {
	AccessToken  []byte    `json:"-"`
	RefreshToken []byte    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	Scopes       []string  `json:"scopes,omitempty"`
}

// OAuthBroker begins and completes an authorization, refreshes and revokes.
type OAuthBroker interface {
	Begin(ctx context.Context, r AuthorizationRequest) (string, error)
	Complete(ctx context.Context, r AuthorizationRequest, code string) (TokenSet, error)
	Refresh(ctx context.Context, c OAuthClient, refreshToken []byte) (TokenSet, error)
	Revoke(ctx context.Context, c OAuthClient, token []byte) error
}

// ClientRegistrar resolves the OAuth client for an issuer: pre-registered, CIMD, DCR, then manual.
type ClientRegistrar interface {
	Resolve(ctx context.Context, tenant TenantID, issuer string) (OAuthClient, error)
}

// KeyScope is what Rotate re-wraps: the root key, or one tenant's key-encryption key.
type KeyScope struct {
	Root   bool     `json:"root"`
	Tenant TenantID `json:"tenant,omitempty"`
}

// SecretStore seals records under a per-tenant key-encryption key and destroys that key for crypto-shredding.
type SecretStore interface {
	Seal(ctx context.Context, tenant TenantID, plaintext []byte) (Sealed, error)
	Open(ctx context.Context, tenant TenantID, s Sealed) ([]byte, error)
	Rotate(ctx context.Context, scope KeyScope) error
	Destroy(ctx context.Context, tenant TenantID) error
}

// Outbound is the neutral form of an outbound request a credential is applied to.
type Outbound struct {
	Headers map[string][]string `json:"headers,omitempty"`
	Env     map[string]string   `json:"env,omitempty"`
}

// CredentialInjector applies an opened credential to an outbound request.
type CredentialInjector interface {
	Inject(ctx context.Context, kind CredentialKind, secret []byte, out *Outbound) error
}

// ApprovalChannel optionally delivers an approval prompt directly; events are always published.
type ApprovalChannel interface {
	Deliver(ctx context.Context, a Approval) error
}

// Finding is one result of a DefinitionScanner or a ResultFilter.
type Finding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// DefinitionScanner inspects a definition at pin time; findings never clear a quarantine.
type DefinitionScanner interface {
	Scan(ctx context.Context, d RawDefinition) ([]Finding, error)
}

// ToolDoc is one function a ToolIndex can return, keyed by its exposed name.
type ToolDoc struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ToolIndex searches the functions granted to an agent; it returns only names in allowed.
type ToolIndex interface {
	Put(ctx context.Context, tenant TenantID, docs []ToolDoc) error
	Search(ctx context.Context, tenant TenantID, query string, allowed []string, limit int) ([]ToolDoc, error)
}

// ResultFilter redacts a result and returns a count of findings per kind, never their values.
type ResultFilter interface {
	Filter(ctx context.Context, r Result) (Result, map[string]int, error)
}

// Suggestion is a proposed title and description; it is untrusted until a person accepts it.
type Suggestion struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
}

// DescriptionSuggester proposes a description overlay for a function; it is never applied by itself.
type DescriptionSuggester interface {
	Suggest(ctx context.Context, d RawDefinition) (Suggestion, error)
}
