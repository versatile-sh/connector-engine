// Package config holds connector-engine.yaml with every key and its default, and adapters.yaml through plugkit/adapters.
package config

import (
	"slices"
	"time"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"

	"github.com/versatile-sh/connector-engine/filter/builtin"
)

// Modes of the engine; production refuses unsafe settings unless acknowledged.
const (
	ModeProduction = "production"
	ModeDev        = "dev"
)

// Config is connector-engine.yaml: every number of the engine, so no figure is a constant in code.
type Config struct {
	Mode         string       `yaml:"mode"`
	Server       Server       `yaml:"server"`
	Auth         Auth         `yaml:"auth"`
	Bootstrap    Bootstrap    `yaml:"bootstrap"`
	Gateway      Gateway      `yaml:"gateway"`
	Apps         Apps         `yaml:"apps"`
	Manage       Manage       `yaml:"manage"`
	Catalog      Catalog      `yaml:"catalog"`
	Permission   Permission   `yaml:"permission"`
	Approval     Approval     `yaml:"approval"`
	Pinning      Pinning      `yaml:"pinning"`
	Exposure     Exposure     `yaml:"exposure"`
	Search       Search       `yaml:"search"`
	Projection   Projection   `yaml:"projection"`
	ResultFilter ResultFilter `yaml:"resultFilter"`
	Overlay      Overlay      `yaml:"overlay"`
	CLI          CLI          `yaml:"cli"`
	GRPC         GRPC         `yaml:"grpc"`
	Sources      Sources      `yaml:"sources"`
	Listing      Listing      `yaml:"listing"`
	Credentials  Credentials  `yaml:"credentials"`
	Secrets      Secrets      `yaml:"secrets"`
	Runtime      Runtime      `yaml:"runtime"`
	Audit        Audit        `yaml:"audit"`
	Outbox       Outbox       `yaml:"outbox"`
	Retention    Retention    `yaml:"retention"`
}

// Server holds the addresses the binary listens on; it is not a key.
type Server struct {
	Listen         string        `yaml:"listen"`
	GRPCListen     string        `yaml:"grpcListen"`
	OperatorListen string        `yaml:"operatorListen"`
	PublicURL      string        `yaml:"publicURL"`
	MaxBodyBytes   int64         `yaml:"maxBodyBytes"`
	ShutdownGrace  time.Duration `yaml:"shutdownGrace"`
}

// Issuer is a token issuer and where its public keys are published.
type Issuer struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// LocalIssuer is the quickstart token issuer; enabling it needs the acknowledgement AckLocalIssuer.
type LocalIssuer struct {
	Enabled     bool          `yaml:"enabled"`
	Name        string        `yaml:"name"`
	RotateEvery time.Duration `yaml:"rotateEvery"`
}

// TrustedProxy is the listener that accepts the identity headers of a fronting proxy; it is off by default and needs AckTrustedProxy.
type TrustedProxy struct {
	Listen string   `yaml:"listen"`
	CIDRs  []string `yaml:"cidrs"`
}

// Operator is the token of the operator listener, read from a reference such as env:NAME or file:/path.
type Operator struct {
	TokenRef string `yaml:"tokenRef"`
}

// Auth is auth.*: issuers, audiences, token lifetime and the opt-ins.
type Auth struct {
	Issuers               []Issuer      `yaml:"issuers"`
	Audience              string        `yaml:"audience"`
	ManageAudience        string        `yaml:"manageAudience"`
	LocalIssuer           LocalIssuer   `yaml:"localIssuer"`
	AgentTokenMaxLifetime time.Duration `yaml:"agentTokenMaxLifetime"`
	SenderConstraint      string        `yaml:"senderConstraint"`
	TrustedProxy          TrustedProxy  `yaml:"trustedProxy"`
	Operator              Operator      `yaml:"operator"`
}

// RoleRow is a role written at boot, the way an installation names its first operator and tenant admins.
type RoleRow struct {
	Tenant string `yaml:"tenant"`
	Issuer string `yaml:"issuer"`
	Person string `yaml:"person"`
	Role   string `yaml:"role"`
	Scope  string `yaml:"scope"`
}

// Bootstrap lists the role rows written at boot; a row already there is left alone.
type Bootstrap struct {
	Roles []RoleRow `yaml:"roles"`
}

// Gateway is gateway.*, read by the agent endpoint.
type Gateway struct {
	ProtocolVersions      []string      `yaml:"protocolVersions"`
	RelayInstructions     string        `yaml:"relayInstructions"`
	UpstreamContent       string        `yaml:"upstreamContent"`
	ScanArgumentsForToken bool          `yaml:"scanArgumentsForToken"`
	ListTTL               time.Duration `yaml:"listTtl"`
	ToolNameMax           int           `yaml:"toolNameMax"`
	Separator             string        `yaml:"separator"`
	DenyAs                string        `yaml:"denyAs"`
	Skills                string        `yaml:"skills"`
}

// Apps is apps.*.
type Apps struct {
	AppOnlyFunctions string `yaml:"appOnlyFunctions"`
}

// MCP is manage.mcp: the mutations named here join the reading operations of /mcp/manage and need AckManageMutations.
type MCP struct {
	Operations []string `yaml:"operations"`
}

// Manage is manage.*.
type Manage struct {
	MCP MCP `yaml:"mcp"`
}

// Catalog is catalog.*.
type Catalog struct {
	TenantDefault   string `yaml:"tenantDefault"`
	OfferDefault    string `yaml:"offerDefault"`
	DefaultCeiling  string `yaml:"defaultCeiling"`
	PrivateListings bool   `yaml:"privateListings"`
}

// LevelWrite is permission.levelWrite: the modes level write expands to.
type LevelWrite struct {
	Write       string `yaml:"write"`
	Destructive string `yaml:"destructive"`
}

// PermissionInstall is permission.install.
type PermissionInstall struct {
	LevelReviewed string `yaml:"levelReviewed"`
	LevelOther    string `yaml:"levelOther"`
}

// External is permission.external: an AuthZEN endpoint that may only narrow; none by default.
type External struct {
	Endpoint string `yaml:"endpoint"`
}

// Permission is permission.*.
type Permission struct {
	DefaultMode       string            `yaml:"defaultMode"`
	LevelWrite        LevelWrite        `yaml:"levelWrite"`
	DestructiveMax    string            `yaml:"destructiveMax"`
	SessionGrantMax   time.Duration     `yaml:"sessionGrantMax"`
	Unclassified      string            `yaml:"unclassified"`
	Install           PermissionInstall `yaml:"install"`
	AlwaysMaxDuration time.Duration     `yaml:"alwaysMaxDuration"`
	External          External          `yaml:"external"`
}

// DenialBrake is approval.denialBrake.
type DenialBrake struct {
	Consecutive int `yaml:"consecutive"`
	PerSession  int `yaml:"perSession"`
}

// InChat is approval.inChat.
type InChat struct {
	Classes []string `yaml:"classes"`
}

// Approval is approval.*.
type Approval struct {
	HoldWindow           time.Duration `yaml:"holdWindow"`
	Timeout              time.Duration `yaml:"timeout"`
	PollAfter            time.Duration `yaml:"pollAfter"`
	ResultRetention      time.Duration `yaml:"resultRetention"`
	DenialBrake          DenialBrake   `yaml:"denialBrake"`
	MaxPendingPerSession int           `yaml:"maxPendingPerSession"`
	ExecuteWithin        time.Duration `yaml:"executeWithin"`
	SweepInterval        time.Duration `yaml:"sweepInterval"`
	ExecuteGrace         time.Duration `yaml:"executeGrace"`
	ProofOfReading       string        `yaml:"proofOfReading"`
	InChat               InChat        `yaml:"inChat"`
	NoteMaxChars         int           `yaml:"noteMaxChars"`
	ChangedWarningDays   int           `yaml:"changedWarningDays"`
}

// Pinning is pinning.*.
type Pinning struct {
	RelistMin          time.Duration `yaml:"relistMin"`
	RelistMax          time.Duration `yaml:"relistMax"`
	HashIcons          bool          `yaml:"hashIcons"`
	AutoAcceptReviewed string        `yaml:"autoAcceptReviewed"`
	RelistBeforeCall   string        `yaml:"relistBeforeCall"`
}

// Exposure is exposure.*.
type Exposure struct {
	OnTrifecta string `yaml:"onTrifecta"`
}

// Search is search.*.
type Search struct {
	ThresholdTokens    int `yaml:"thresholdTokens"`
	ThresholdFunctions int `yaml:"thresholdFunctions"`
	ResultMax          int `yaml:"resultMax"`
}

// Projection is projection.*.
type Projection struct {
	Default  string `yaml:"default"`
	MaxPaths int    `yaml:"maxPaths"`
}

// ResultFilter is resultFilter.*: none or builtin, and the locale pattern sets the builtin filter masks besides secrets, emails, cards and phones.
type ResultFilter struct {
	Adapter string   `yaml:"adapter"`
	Locales []string `yaml:"locales"`
}

// Overlay is overlay.*: no suggester ships in this build.
type Overlay struct {
	Suggester string `yaml:"suggester"`
}

// CLI is cli.*, read by the agent runtime's recipe.
type CLI struct {
	DefaultForCodingAgents bool `yaml:"defaultForCodingAgents"`
}

// GRPC is grpc.*.
type GRPC struct {
	UnaryOnly bool `yaml:"unaryOnly"`
	Insecure  bool `yaml:"insecure"`
}

// Official is the official registry source.
type Official struct {
	Enabled  bool   `yaml:"enabled"`
	URL      string `yaml:"url"`
	TokenRef string `yaml:"tokenRef"`
}

// BSRSource is the Buf Schema Registry source: the owners to list, the gRPC target of each module and the token reference.
type BSRSource struct {
	Enabled  bool              `yaml:"enabled"`
	URL      string            `yaml:"url"`
	Owners   []string          `yaml:"owners"`
	Targets  map[string]string `yaml:"targets"`
	TokenRef string            `yaml:"tokenRef"`
}

// Directory is a source read from a directory of files.
type Directory struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

// OpenAPISource is the source over a directory of OpenAPI documents and a list of addresses.
type OpenAPISource struct {
	Enabled bool     `yaml:"enabled"`
	Dir     string   `yaml:"dir"`
	URLs    []string `yaml:"urls"`
}

// SkillCatalog is the source over skill catalog entries at https addresses; the engine names no catalog itself.
type SkillCatalog struct {
	Enabled bool     `yaml:"enabled"`
	URLs    []string `yaml:"urls"`
}

// ReflectionSource is the source over gRPC server reflection.
type ReflectionSource struct {
	Enabled  bool     `yaml:"enabled"`
	Targets  []string `yaml:"targets"`
	Insecure bool     `yaml:"insecure"`
}

// Sources is sources.*.
type Sources struct {
	Official     Official         `yaml:"official"`
	BSR          BSRSource        `yaml:"bsr"`
	Docker       Directory        `yaml:"docker"`
	Files        Directory        `yaml:"files"`
	OpenAPI      OpenAPISource    `yaml:"openapi"`
	Reflection   ReflectionSource `yaml:"reflection"`
	SkillCatalog SkillCatalog     `yaml:"skillCatalog"`
	SyncInterval time.Duration    `yaml:"syncInterval"`
}

// Listing is listing.*.
type Listing struct {
	RequireDigest    bool `yaml:"requireDigest"`
	RequireSignature bool `yaml:"requireSignature"`
}

// OAuthClient is a pre-registered OAuth client; an empty tenant makes it the default for the issuer.
type OAuthClient struct {
	Tenant      string `yaml:"tenant"`
	Issuer      string `yaml:"issuer"`
	ClientID    string `yaml:"clientId"`
	SecretRef   string `yaml:"secretRef"`
	RedirectURL string `yaml:"redirectUrl"`
}

// Credentials is credentials.*.
type Credentials struct {
	RefreshMargin     time.Duration `yaml:"refreshMargin"`
	RegistrationOrder []string      `yaml:"registrationOrder"`
	Fallback          string        `yaml:"fallback"`
	Clients           []OAuthClient `yaml:"clients"`
}

// Secrets is secrets.*; Dir is where the file-backed adapters keep their root key, UnlockSocket where hostkey waits for the host's unlock key.
type Secrets struct {
	Adapter      string        `yaml:"adapter"`
	DataKeyCache time.Duration `yaml:"dataKeyCache"`
	Dir          string        `yaml:"dir"`
	UnlockSocket string        `yaml:"unlockSocket"`
	OnePassword  OnePassword   `yaml:"onePassword"`
}

// OnePassword is secrets.onePassword: the Connect server, a reference to its token, and the vault id, item id and field label of the unlock key.
type OnePassword struct {
	ConnectURL string `yaml:"connectUrl"`
	TokenRef   string `yaml:"tokenRef"`
	Vault      string `yaml:"vault"`
	Item       string `yaml:"item"`
	Field      string `yaml:"field"`
}

// Container is runtime.container; no container runtime ships in this build.
type Container struct {
	CPU      int           `yaml:"cpu"`
	Memory   string        `yaml:"memory"`
	IdleStop time.Duration `yaml:"idleStop"`
	Network  string        `yaml:"network"`
}

// Runtime is runtime.*.
type Runtime struct {
	Adapters          []string      `yaml:"adapters"`
	CallTimeout       time.Duration `yaml:"callTimeout"`
	AllowPrivateHosts bool          `yaml:"allowPrivateHosts"`
	Container         Container     `yaml:"container"`
}

// Audit is audit.*.
type Audit struct {
	Arguments string `yaml:"arguments"`
	HashChain bool   `yaml:"hashChain"`
}

// Outbox is outbox.*.
type Outbox struct {
	Signing string `yaml:"signing"`
}

// Retention is retention.*: how often the worker prunes history and how long it keeps delivered events, idempotency keys, audit rows and finished calls; 0s keeps them forever.
type Retention struct {
	Every           time.Duration `yaml:"every"`
	DeliveredEvents time.Duration `yaml:"deliveredEvents"`
	IdempotencyKeys time.Duration `yaml:"idempotencyKeys"`
	Audit           time.Duration `yaml:"audit"`
	Calls           time.Duration `yaml:"calls"`
}

// Default returns every key at its default, and the keys of the binary at theirs.
func Default() Config {
	return Config{
		Mode:   ModeProduction,
		Server: Server{Listen: ":8080", GRPCListen: ":9090", MaxBodyBytes: 1 << 20, ShutdownGrace: 15 * time.Second},
		Auth: Auth{LocalIssuer: LocalIssuer{Name: "connector-engine-local", RotateEvery: 30 * 24 * time.Hour}, AgentTokenMaxLifetime: time.Hour, SenderConstraint: "none",
			Issuers: []Issuer{}, TrustedProxy: TrustedProxy{CIDRs: []string{}}},
		Bootstrap: Bootstrap{Roles: []RoleRow{}},
		Gateway: Gateway{ProtocolVersions: []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26"}, RelayInstructions: "never", UpstreamContent: "refuse", ScanArgumentsForToken: true,
			ListTTL: 60 * time.Second, ToolNameMax: 64, Separator: "__", DenyAs: "unknown_tool", Skills: "off"},
		Apps:    Apps{AppOnlyFunctions: "refuse"},
		Manage:  Manage{MCP: MCP{Operations: []string{ReadOnly}}},
		Catalog: Catalog{TenantDefault: "none", OfferDefault: "enabled_sources", DefaultCeiling: "write", PrivateListings: true},
		Permission: Permission{DefaultMode: "deny", LevelWrite: LevelWrite{Write: "ask_once", Destructive: "ask"}, DestructiveMax: "ask_once",
			SessionGrantMax: 28800 * time.Second, Unclassified: "destructive", Install: PermissionInstall{LevelReviewed: "write", LevelOther: "off"},
			AlwaysMaxDuration: 90 * 24 * time.Hour},
		Approval: Approval{HoldWindow: 20 * time.Second, Timeout: 600 * time.Second, PollAfter: 15 * time.Second, ResultRetention: 3600 * time.Second,
			DenialBrake: DenialBrake{Consecutive: 3, PerSession: 10}, MaxPendingPerSession: 5, ExecuteWithin: 60 * time.Second, SweepInterval: 5 * time.Second,
			ExecuteGrace: 10 * time.Second, ProofOfReading: "off", InChat: InChat{Classes: []string{"write"}}, NoteMaxChars: 500, ChangedWarningDays: 7},
		Pinning:      Pinning{RelistMin: 30 * time.Second, RelistMax: 300 * time.Second, AutoAcceptReviewed: "none", RelistBeforeCall: "held"},
		Exposure:     Exposure{OnTrifecta: "warn"},
		Search:       Search{ThresholdTokens: 10000, ThresholdFunctions: 30, ResultMax: 5},
		Projection:   Projection{Default: "off", MaxPaths: 20},
		ResultFilter: ResultFilter{Adapter: "none", Locales: slices.Clone(builtin.DefaultLocales)},
		Overlay:      Overlay{Suggester: "none"},
		CLI:          CLI{DefaultForCodingAgents: true},
		GRPC:         GRPC{UnaryOnly: true},
		Sources: Sources{Official: Official{URL: "https://registry.modelcontextprotocol.io"}, OpenAPI: OpenAPISource{URLs: []string{}}, SkillCatalog: SkillCatalog{URLs: []string{}},
			Reflection: ReflectionSource{Targets: []string{}}, BSR: BSRSource{Owners: []string{}, Targets: map[string]string{}}, SyncInterval: 6 * time.Hour},
		Listing:     Listing{RequireDigest: true},
		Credentials: Credentials{RefreshMargin: 300 * time.Second, RegistrationOrder: []string{"pre_registered", "cimd", "dcr", "manual"}, Fallback: "none", Clients: []OAuthClient{}},
		Secrets:     Secrets{Adapter: "localfile", DataKeyCache: 300 * time.Second, Dir: "/var/lib/connector-engine/secrets"},
		Runtime:     Runtime{Adapters: []string{"remote", "declarative"}, CallTimeout: 60 * time.Second, Container: Container{CPU: 1, Memory: "2GB", IdleStop: 60 * time.Second, Network: "none"}},
		Audit:       Audit{Arguments: "reference"},
		Outbox:      Outbox{Signing: "standard-webhooks"},
		Retention:   Retention{Every: time.Hour},
	}
}

// ReadOnly is the entry of manage.mcp.operations that offers the reading operations alone.
const ReadOnly = "read_only"

func (c Config) inert() adapters.Findings {
	d := Default()
	var fs adapters.Findings
	for _, k := range []struct {
		key string
		ok  bool
	}{
		{"gateway.denyAs", c.Gateway.DenyAs == d.Gateway.DenyAs},
		{"audit.arguments", c.Audit.Arguments == d.Audit.Arguments},
		{"audit.hashChain", c.Audit.HashChain == d.Audit.HashChain},
		{"pinning.hashIcons", c.Pinning.HashIcons == d.Pinning.HashIcons},
		{"approval.inChat.classes", slices.Equal(c.Approval.InChat.Classes, d.Approval.InChat.Classes)},
		{"listing.requireSignature", c.Listing.RequireSignature == d.Listing.RequireSignature},
		{"secrets.dataKeyCache", c.Secrets.DataKeyCache == d.Secrets.DataKeyCache},
		{"credentials.registrationOrder", slices.Equal(c.Credentials.RegistrationOrder, d.Credentials.RegistrationOrder)},
		{"runtime.container", c.Runtime.Container == d.Runtime.Container},
		{"grpc.unaryOnly", c.GRPC.UnaryOnly == d.GRPC.UnaryOnly},
	} {
		if !k.ok {
			fs = append(fs, adapters.Finding{Level: adapters.LevelError, Where: k.key, Message: "this build does not act on the key; only its default is accepted"})
		}
	}
	return fs
}
