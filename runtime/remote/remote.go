// Package remote is the ConnectorRuntime for mcp_remote listings over Streamable HTTP, on the official Go SDK.
package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/credential"
	deadlines "github.com/agents-control/connector-engine/internal/deadline"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/internal/handles"
	"github.com/agents-control/connector-engine/port"
)

type page struct {
	Tools      []json.RawMessage `json:"tools"`
	NextCursor string            `json:"nextCursor"`
	TTLMs      int64             `json:"ttlMs"`
}

type instance struct {
	session *mcp.ClientSession
	capture *capture
	mu      sync.Mutex
	pages   map[string]page
}

// Runtime reaches a remote MCP server with the upstream credential and never the agent's token.
type Runtime struct {
	applier credential.Applier
	base    http.RoundTripper
	guard   egress.Guard
	started *handles.Registry[*instance]
}

// New returns a runtime that injects credentials through applier; a nil base dials through guard.
func New(applier credential.Applier, base http.RoundTripper, guard egress.Guard) *Runtime {
	return &Runtime{applier: applier, base: base, guard: guard, started: handles.New[*instance]("remote")}
}

// Describe states a runtime with no isolation that reaches only the listing's own host.
func (*Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{Kinds: []port.Kind{port.KindMCPRemote}, Isolation: "none", Egress: "listing host"}
}

// Start connects to the listing's remote address and negotiates the newest protocol era both sides speak.
func (r *Runtime) Start(ctx context.Context, _ port.Connector, m port.Manifest, secrets port.SecretsRef) (port.Handle, error) {
	if m.Kind != port.KindMCPRemote || m.Remote == nil {
		return "", port.Errorf(port.CodeInvalidManifest, "the remote runtime serves mcp_remote listings with a remote address")
	}
	u, err := url.Parse(m.Remote.URL)
	if err != nil || u.Host == "" {
		return "", port.Errorf(port.CodeInvalidManifest, "remote.url %q is not an address", m.Remote.URL)
	}
	if t := m.Remote.Transport; t != "" && t != "streamable-http" && t != "streamable_http" {
		return "", port.Errorf(port.CodeInvalidManifest, "remote transport %q is not Streamable HTTP", t)
	}
	base := r.base
	if base == nil {
		base = r.guard.Transport(allowedHosts(m)...)
	}
	capt := &capture{next: credential.Transport{Base: base, Applier: r.applier, Ref: secrets, AllowHosts: []string{u.Host}}}
	client := mcp.NewClient(&mcp.Implementation{Name: "connector-engine", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             m.Remote.URL,
		HTTPClient:           &http.Client{Transport: capt},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		return "", port.Errorf(port.CodeUnavailable, "connect %s: %v", u.Host, err)
	}
	return r.started.Add(&instance{session: session, capture: capt, pages: map[string]page{}}), nil
}

// ListFunctions returns every tool definition as the server sent it; a list served from the SDK's TTL cache repeats the last bytes seen.
func (r *Runtime) ListFunctions(ctx context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	in.capture.reset()
	var defs []port.RawDefinition
	var ttl int64
	for cursor := ""; ; {
		res, err := in.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, port.ListMeta{}, port.Errorf(port.CodeUnavailable, "tools/list: %v", err)
		}
		pg, ok := in.pageFor(cursor)
		if !ok {
			return nil, port.ListMeta{}, port.Errorf(port.CodeInternal, "tools/list: no raw page for cursor %q", cursor)
		}
		for _, raw := range pg.Tools {
			var name struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &name) != nil || name.Name == "" {
				return nil, port.ListMeta{}, port.Errorf(port.CodeInvalidManifest, "a tool definition without a name")
			}
			defs = append(defs, port.RawDefinition{Name: name.Name, Raw: raw})
		}
		ttl = max(ttl, pg.TTLMs)
		if cursor = res.NextCursor; cursor == "" {
			break
		}
	}
	init := in.session.InitializeResult()
	return defs, port.ListMeta{TTL: time.Duration(ttl) * time.Millisecond, Instructions: init.Instructions, ProtocolVersion: init.ProtocolVersion}, nil
}

func (in *instance) pageFor(cursor string) (page, bool) {
	raw, ok := in.capture.take()
	if !ok {
		pg, cached := in.pages[cursor]
		return pg, cached
	}
	var pg page
	if json.Unmarshal(raw, &pg) != nil {
		return pg, false
	}
	in.pages[cursor] = pg
	return pg, true
}

// Call forwards the call under the deadline and returns the content blocks and structured content the server answered.
func (r *Runtime) Call(ctx context.Context, h port.Handle, function string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	ctx, cancel := deadlines.Apply(ctx, deadline)
	defer cancel()
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	res, err := in.session.CallTool(ctx, &mcp.CallToolParams{Name: function, Arguments: arguments})
	if err != nil {
		if ctx.Err() != nil {
			return port.Result{}, ctx.Err()
		}
		return port.Result{}, port.Errorf(port.CodeUnavailable, "tools/call %s: %v", function, err)
	}
	out := port.Result{IsError: res.IsError}
	if out.Content, err = json.Marshal(res.Content); err != nil {
		return port.Result{}, port.Errorf(port.CodeInternal, "content: %v", err)
	}
	if res.StructuredContent != nil {
		if out.Structured, err = json.Marshal(res.StructuredContent); err != nil {
			return port.Result{}, port.Errorf(port.CodeInternal, "structured content: %v", err)
		}
	}
	return out, nil
}

// Stop closes the session.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	in, ok := r.started.Remove(h)
	if !ok {
		return port.Errorf(port.CodeNotFound, "no started instance %q", h)
	}
	return in.session.Close()
}

func allowedHosts(m port.Manifest) []string {
	if m.Egress == nil {
		return nil
	}
	return m.Egress.AllowHosts
}
