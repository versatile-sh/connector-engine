package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"gopkg.in/yaml.v3"

	"github.com/versatile-sh/connector-engine/core/approval"
	"github.com/versatile-sh/connector-engine/core/grant"
	"github.com/versatile-sh/connector-engine/filter/builtin"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

// Acknowledgements an adapters file may list; production refuses the matching setting without them.
const (
	AckLocalIssuer       = "local_token_issuer"
	AckLongLivedTokens   = "long_lived_agent_tokens"
	AckTrustedProxy      = "trusted_proxy"
	AckManageMutations   = "manage_mcp_mutations"
	AckPlaintextRootKey  = "plaintext_root_key"
	AckPrivateEgress     = "private_egress"
	maxAgentTokenDefault = time.Hour
)

// Acknowledgements lists every name an adapters file may acknowledge.
var Acknowledgements = []string{AckLocalIssuer, AckLongLivedTokens, AckTrustedProxy, AckManageMutations, AckPlaintextRootKey, AckPrivateEgress}

// Parse reads connector-engine.yaml over the defaults, strictly: an unknown key is an error and a missing key keeps its default.
func Parse(data []byte) (Config, error) {
	c := Default()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("connector-engine.yaml: %w", err)
	}
	return c, nil
}

// Load reads the file at path; an empty path is the defaults.
func Load(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("connector-engine.yaml: %w", err)
	}
	return Parse(data)
}

// Marshal renders the configuration as connector-engine.yaml, every key written out.
func (c Config) Marshal() ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}

var oneOf = map[string][]string{
	"mode":                          {ModeProduction, ModeDev},
	"auth.senderConstraint":         {"none"},
	"catalog.tenantDefault":         {"none", "starter"},
	"pinning.autoAcceptReviewed":    {"none", "read"},
	"pinning.relistBeforeCall":      {"held", "write", "all"},
	"permission.destructiveMax":     {"ask", "ask_once"},
	"permission.defaultMode":        {"deny"},
	"resultFilter.adapter":          {"none", "builtin"},
	"projection.default":            {"off", "on"},
	"audit.arguments":               {"reference", "redacted"},
	"gateway.relayInstructions":     {"never", "pinned"},
	"gateway.upstreamContent":       {"refuse", "granted"},
	"gateway.skills":                {"off", "tools", "tools+prompts"},
	"approval.proofOfReading":       {"off"},
	"overlay.suggester":             {"none"},
	"outbox.signing":                {"standard-webhooks"},
	"secrets.adapter":               {"localfile", "memory", "hostkey", "onepassword"},
	"credentials.fallback":          {"none"},
	"exposure.onTrifecta":           {"warn", "force_ask"},
	"apps.appOnlyFunctions":         {"refuse", "view_token"},
	"permission.unclassified":       {"destructive"},
	"catalog.defaultCeiling":        {"read", "write", "destructive"},
	"catalog.offerDefault":          {"enabled_sources"},
	"runtime.container.network":     {"none"},
	"permission.install.levelOther": {"off"},
}

func (c Config) values() map[string]string {
	return map[string]string{
		"mode": c.Mode, "auth.senderConstraint": c.Auth.SenderConstraint, "catalog.tenantDefault": c.Catalog.TenantDefault,
		"pinning.autoAcceptReviewed": c.Pinning.AutoAcceptReviewed, "pinning.relistBeforeCall": c.Pinning.RelistBeforeCall,
		"permission.destructiveMax": c.Permission.DestructiveMax, "permission.defaultMode": c.Permission.DefaultMode,
		"resultFilter.adapter": c.ResultFilter.Adapter, "projection.default": c.Projection.Default, "audit.arguments": c.Audit.Arguments,
		"gateway.relayInstructions": c.Gateway.RelayInstructions, "gateway.upstreamContent": c.Gateway.UpstreamContent, "gateway.skills": c.Gateway.Skills, "approval.proofOfReading": c.Approval.ProofOfReading,
		"overlay.suggester": c.Overlay.Suggester, "outbox.signing": c.Outbox.Signing, "secrets.adapter": c.Secrets.Adapter,
		"credentials.fallback": c.Credentials.Fallback, "exposure.onTrifecta": c.Exposure.OnTrifecta, "apps.appOnlyFunctions": c.Apps.AppOnlyFunctions,
		"permission.unclassified": c.Permission.Unclassified, "catalog.defaultCeiling": c.Catalog.DefaultCeiling,
		"catalog.offerDefault": c.Catalog.OfferDefault, "runtime.container.network": c.Runtime.Container.Network,
		"permission.install.levelOther": c.Permission.Install.LevelOther,
	}
}

// Mutations returns the operation ids named in manage.mcp.operations besides read_only.
func (c Config) Mutations() []string {
	return slices.DeleteFunc(slices.Clone(c.Manage.MCP.Operations), func(s string) bool { return s == ReadOnly })
}

// Check validates the values and applies the refusals: in production each is an error unless acknowledged, in dev a warning.
func (c Config) Check(acknowledged func(string) bool) adapters.Findings {
	var fs adapters.Findings
	level := adapters.LevelWarning
	if c.Mode == ModeProduction {
		level = adapters.LevelError
	}
	add := func(level, where, format string, args ...any) {
		fs = append(fs, adapters.Finding{Level: level, Where: where, Message: fmt.Sprintf(format, args...)})
	}
	for key, allowed := range oneOf {
		if v := c.values()[key]; !slices.Contains(allowed, v) {
			add(adapters.LevelError, key, "%q is not one of %s", v, strings.Join(allowed, ", "))
		}
	}
	for _, l := range c.ResultFilter.Locales {
		if !slices.Contains(builtin.Locales(), l) {
			add(adapters.LevelError, "resultFilter.locales", "%q is not one of %s", l, strings.Join(builtin.Locales(), ", "))
		}
	}
	if c.Secrets.Adapter == "hostkey" && c.Secrets.UnlockSocket == "" {
		add(adapters.LevelError, "secrets.unlockSocket", "the hostkey adapter waits for the unlock key on this socket; it is empty")
	}
	if op := c.Secrets.OnePassword; c.Secrets.Adapter == "onepassword" && (op.ConnectURL == "" || op.TokenRef == "" || op.Vault == "" || op.Item == "" || op.Field == "") {
		add(adapters.LevelError, "secrets.onePassword", "the onepassword adapter needs connectUrl, tokenRef, vault, item and field")
	}
	if len(c.Auth.Issuers) == 0 && !c.Auth.LocalIssuer.Enabled {
		add(level, "auth.issuers", "the list is empty: no token can be verified")
	}
	if c.Auth.LocalIssuer.Enabled && c.Auth.Operator.TokenRef == "" {
		add(adapters.LevelError, "auth.localIssuer.enabled", "the local issuer mints tokens only for the operator token; set auth.operator.tokenRef")
	}
	if c.Auth.LocalIssuer.Enabled && !acknowledged(AckLocalIssuer) {
		add(level, "auth.localIssuer.enabled", "the local issuer signs the claims the operator token asks for; acknowledge %s in the adapters file to run it", AckLocalIssuer)
	}
	if c.Auth.AgentTokenMaxLifetime > maxAgentTokenDefault && !acknowledged(AckLongLivedTokens) {
		add(level, "auth.agentTokenMaxLifetime", "%s is above one hour; acknowledge %s to accept agent tokens that live longer", c.Auth.AgentTokenMaxLifetime, AckLongLivedTokens)
	}
	if c.Auth.TrustedProxy.Listen != "" && !acknowledged(AckTrustedProxy) {
		add(level, "auth.trustedProxy.listen", "identity headers of a proxy are trusted; acknowledge %s to open the listener", AckTrustedProxy)
	}
	if len(c.Mutations()) > 0 && !acknowledged(AckManageMutations) {
		add(level, "manage.mcp.operations", "mutations on the management MCP surface; acknowledge %s to offer them", AckManageMutations)
	}
	switch {
	case acknowledged(AckPlaintextRootKey), c.Secrets.Adapter == "onepassword":
	case c.Secrets.Adapter == "hostkey":
		add(level, "secrets.adapter", "hostkey keeps a plaintext root key on a host without a keychain and until it is migrated; acknowledge %s to run with it", AckPlaintextRootKey)
	default:
		add(level, "secrets.adapter", "%s keeps the root key in plaintext; acknowledge %s to run with it", c.Secrets.Adapter, AckPlaintextRootKey)
	}
	if c.Runtime.AllowPrivateHosts && !acknowledged(AckPrivateEgress) {
		add(level, "runtime.allowPrivateHosts", "runtimes and sources may dial loopback, private and metadata addresses; acknowledge %s to allow it", AckPrivateEgress)
	}
	if c.Mode == ModeProduction && c.Server.PublicURL == "" && (c.Auth.Audience == "" || c.Auth.ManageAudience == "") {
		add(adapters.LevelError, "auth.audience", "auth.audience and auth.manageAudience default to server.publicURL, which is empty")
	}
	if c.Approval.SweepInterval <= 0 || c.Server.ShutdownGrace < 0 {
		add(adapters.LevelError, "approval.sweepInterval", "durations are positive")
	}
	return append(fs, c.inert()...)
}

// Audience returns the audience of the agent endpoint, defaulting to the address of /mcp.
func (c Config) Audience() string {
	return orDefault(c.Auth.Audience, strings.TrimRight(c.Server.PublicURL, "/")+"/mcp")
}

// ResourceMetadataURL is where the agent endpoint publishes its protected-resource metadata (RFC 9728), empty without server.publicURL.
func (c Config) ResourceMetadataURL() string {
	if c.Server.PublicURL == "" {
		return ""
	}
	return strings.TrimRight(c.Server.PublicURL, "/") + "/.well-known/oauth-protected-resource"
}

// ManageAudience returns the audience of the management API, defaulting to the public address.
func (c Config) ManageAudience() string {
	return orDefault(c.Auth.ManageAudience, strings.TrimRight(c.Server.PublicURL, "/"))
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// Service returns the configuration of the use cases: the defaults of the service with every key of this file laid over them.
func (c Config) Service() service.Config {
	s := service.DefaultConfig()
	s.Approval = approval.Limits{Timeout: c.Approval.Timeout, ExecuteWithin: c.Approval.ExecuteWithin, ExecuteGrace: c.Approval.ExecuteGrace,
		ResultRetention: c.Approval.ResultRetention, MaxPending: c.Approval.MaxPendingPerSession, BrakeConsecutive: c.Approval.DenialBrake.Consecutive,
		BrakePerSession: c.Approval.DenialBrake.PerSession, NoteMax: c.Approval.NoteMaxChars}
	s.PollAfter, s.RelistMin, s.RelistMax = c.Approval.PollAfter, c.Pinning.RelistMin, c.Pinning.RelistMax
	s.RelistBeforeCall, s.AutoAcceptReviewed, s.TenantDefault = c.Pinning.RelistBeforeCall, c.Pinning.AutoAcceptReviewed, c.Catalog.TenantDefault
	s.DefaultCeiling, s.PrivateListings = port.Ceiling(c.Catalog.DefaultCeiling), c.Catalog.PrivateListings
	s.SessionGrantMax, s.AlwaysMax, s.AgentTokenMax = c.Permission.SessionGrantMax, c.Permission.AlwaysMaxDuration, c.Auth.AgentTokenMaxLifetime
	s.LevelModes = grant.LevelModes{Write: port.Mode(c.Permission.LevelWrite.Write), Destructive: port.Mode(c.Permission.LevelWrite.Destructive)}
	s.DestructiveMax, s.CallTimeout, s.ListTTL = port.Mode(c.Permission.DestructiveMax), c.Runtime.CallTimeout, c.Gateway.ListTTL
	s.ForceAsk = c.Exposure.OnTrifecta == "force_ask"
	s.Separator, s.NameMax = c.Gateway.Separator, c.Gateway.ToolNameMax
	s.ProjectionDefault, s.ProjectionMaxPaths = c.Projection.Default == "on", c.Projection.MaxPaths
	s.RelayInstructions, s.UpstreamContent = c.Gateway.RelayInstructions == "pinned", c.Gateway.UpstreamContent == "granted"
	s.RequireDigest, s.ScanForToken, s.ChangedWarningDays, s.RefreshMargin = c.Listing.RequireDigest, c.Gateway.ScanArgumentsForToken, c.Approval.ChangedWarningDays, c.Credentials.RefreshMargin
	return s
}
