package service

import (
	"time"

	"github.com/agents-control/connector-engine/core/approval"
	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/port"
)

// Config holds the keys of §14 that the use cases read; DefaultConfig is the defaults of that table.
type Config struct {
	Approval           approval.Limits
	PollAfter          time.Duration
	RelistMin          time.Duration
	RelistMax          time.Duration
	RelistBeforeCall   string
	AutoAcceptReviewed string
	TenantDefault      string
	DefaultCeiling     port.Ceiling
	PrivateListings    bool
	SessionGrantMax    time.Duration
	AlwaysMax          time.Duration
	AgentTokenMax      time.Duration
	LevelModes         grant.LevelModes
	DestructiveMax     port.Mode
	ForceAsk           bool
	CallTimeout        time.Duration
	ListTTL            time.Duration
	Separator          string
	NameMax            int
	ProjectionDefault  bool
	ProjectionMaxPaths int
	RequireDigest      bool
	ScanForToken       bool
	ChangedWarningDays int
	RelayInstructions  string
	KeyOutcome         string
	RefreshMargin      time.Duration
}

// DefaultConfig returns the defaults of §14.
func DefaultConfig() Config {
	return Config{
		Approval: approval.Defaults, PollAfter: 15 * time.Second, RelistMin: 30 * time.Second, RelistMax: 300 * time.Second,
		RelistBeforeCall: "held", AutoAcceptReviewed: "none", TenantDefault: "none", DefaultCeiling: port.CeilingWrite, PrivateListings: true,
		SessionGrantMax: 28800 * time.Second, AlwaysMax: 90 * 24 * time.Hour, AgentTokenMax: time.Hour, LevelModes: grant.DefaultLevelModes,
		DestructiveMax: port.ModeAskOnce, CallTimeout: 60 * time.Second, ListTTL: 60 * time.Second, Separator: names.Separator, NameMax: names.MaxLength,
		ProjectionMaxPaths: 20, RequireDigest: true, ScanForToken: true, ChangedWarningDays: 7, RelayInstructions: "never", KeyOutcome: "wrapped_key_deleted", RefreshMargin: 300 * time.Second,
	}
}
