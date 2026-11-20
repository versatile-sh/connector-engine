// Package gateway is the agent endpoint: an MCP server over Streamable HTTP that lists and calls the functions the engine grants to one agent.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

const (
	newEraVersion    = "2026-07-28"
	versionHeader    = "MCP-Protocol-Version"
	instructions     = "Functions reach connectors through an approval gateway. A call that needs a person's approval answers pending_approval; poll approval_status."
	serverName       = "connector-engine"
	changedSentinel  = "list-changed-sentinel"
	principalExtra   = "connector-engine/principal"
	deliveryExtra    = "connector-engine/deliveries"
	fieldsMetaKey    = "connector-engine/fields"
	defaultHoldPoll  = 250 * time.Millisecond
	defaultSession   = 30 * time.Minute
	tokenBytesPerTok = 4
)

// Core is what the gateway asks of the use cases; *service.Service is one.
type Core interface {
	ListFunctions(ctx context.Context, p port.Principal) (service.FunctionList, error)
	Call(ctx context.Context, p port.Principal, req service.CallRequest) (service.Outcome, error)
	ApprovalStatus(ctx context.Context, p port.Principal, id port.ApprovalID) (service.Outcome, error)
	ApprovalCancelByAgent(ctx context.Context, p port.Principal, id port.ApprovalID) (service.Outcome, error)
	Complete(ctx context.Context, p port.Principal, req service.CallRequest) (json.RawMessage, bool, error)
	MarkDelivered(ctx context.Context, p port.Principal, id port.ApprovalID) error
	TenantActive(ctx context.Context, tenant port.TenantID) error
}

// Config holds the keys the gateway reads; DefaultConfig is the defaults of that table.
type Config struct {
	Audience            string
	ResourceMetadataURL string
	ViewTokens          bool
	HoldWindow          time.Duration
	HoldPoll            time.Duration
	ThresholdTokens     int
	ThresholdFunctions  int
	SearchResultMax     int
	SessionTimeout      time.Duration
	Skills              bool
	Instructions        bool
	Content             bool
	SkillPrompts        bool
	Logger              *slog.Logger
}

// DefaultConfig returns the defaults for an endpoint with the given audience.
func DefaultConfig(audience string) Config {
	return Config{Audience: audience, HoldWindow: 20 * time.Second, HoldPoll: defaultHoldPoll, ThresholdTokens: 10000, ThresholdFunctions: 30,
		SearchResultMax: 5, SessionTimeout: defaultSession}
}

// Gateway serves the agent endpoint.
type Gateway struct {
	core     Core
	verifier port.PrincipalVerifier
	index    port.ToolIndex
	cfg      Config
	server   *mcp.Server
	handler  http.Handler
}

// New builds a Gateway; the index may be nil, and then a large list is served whole instead of through find and call.
func New(core Core, verifier port.PrincipalVerifier, index port.ToolIndex, cfg Config) (*Gateway, error) {
	if core == nil || verifier == nil || cfg.Audience == "" {
		return nil, port.Errorf(port.CodeInvalidArgument, "core, verifier and audience are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	if cfg.HoldPoll <= 0 {
		cfg.HoldPoll = defaultHoldPoll
	}
	g := &Gateway{core: core, verifier: verifier, index: index, cfg: cfg}
	g.server = mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "1"}, &mcp.ServerOptions{Capabilities: cfg.capabilities(), Instructions: instructions})
	g.server.AddReceivingMiddleware(g.middleware)
	sessionless := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return g.server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	sessions := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return g.server }, &mcp.StreamableHTTPOptions{SessionTimeout: cfg.SessionTimeout})
	eras := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(versionHeader) >= newEraVersion {
			sessionless.ServeHTTP(w, r)
			return
		}
		sessions.ServeHTTP(w, r)
	})
	g.handler = auth.RequireBearerToken(g.verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: cfg.ResourceMetadataURL, AllowMissingExpiration: true})(g.delivering(eras))
	return g, nil
}

// Handler serves /mcp: requests of protocol 2026-07-28 and later run without a session, older ones in an SDK session, and each request carries its own identity.
func (g *Gateway) Handler() http.Handler { return g.handler }

func (c Config) prompts() bool { return c.Content || c.SkillPrompts }

func (c Config) capabilities() *mcp.ServerCapabilities {
	caps := &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{ListChanged: true}}
	if c.prompts() {
		caps.Prompts = &mcp.PromptCapabilities{ListChanged: true}
	}
	if c.Content {
		caps.Resources, caps.Completions = &mcp.ResourceCapabilities{ListChanged: true}, &mcp.CompletionCapabilities{}
	}
	return caps
}

// ToolsChanged sends the list_changed notifications of tools, and of the prompts and resources it serves, to the sessions and listen streams that can receive them.
func (g *Gateway) ToolsChanged() {
	g.server.AddTool(&mcp.Tool{Name: changedSentinel, InputSchema: map[string]any{"type": "object"}}, nil)
	g.server.RemoveTools(changedSentinel)
	if g.cfg.prompts() {
		g.server.AddPrompt(&mcp.Prompt{Name: changedSentinel}, nil)
		g.server.RemovePrompts(changedSentinel)
	}
	if g.cfg.Content {
		g.server.AddResource(&mcp.Resource{Name: changedSentinel, URI: "sentinel:" + changedSentinel}, nil)
		g.server.RemoveResources("sentinel:" + changedSentinel)
	}
}

func (g *Gateway) verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	p, err := g.verifier.Verify(ctx, token, g.cfg.Audience)
	switch {
	case port.IsCode(err, port.CodeUnauthenticated):
		return nil, fmt.Errorf("%w: %w", auth.ErrInvalidToken, err)
	case err != nil:
		return nil, err
	case p.Kind != port.PrincipalAgent && p.Kind != port.PrincipalView:
		return nil, fmt.Errorf("%w: the agent endpoint takes an agent or view token", auth.ErrInvalidToken)
	case p.Kind == port.PrincipalView && !g.cfg.ViewTokens:
		return nil, fmt.Errorf("%w: view tokens are refused (apps.appOnlyFunctions = refuse)", auth.ErrInvalidToken)
	}
	if err := g.core.TenantActive(ctx, p.Claims.Tenant); port.IsCode(err, port.CodeTenantRemoved) {
		return nil, fmt.Errorf("%w: %w", auth.ErrInvalidToken, err)
	} else if err != nil {
		return nil, err
	}
	return &auth.TokenInfo{UserID: string(p.Claims.Tenant) + "/" + string(p.Claims.Agent) + "/" + string(p.Claims.Session), Extra: map[string]any{principalExtra: p, deliveryExtra: &deliveries{}}}, nil
}
