// Package servicetest holds fakes of the ports a Service runs on and the Env that wires them over the memory store.
package servicetest

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/port"
)

// Clock is a controllable port.Clock.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a clock set to at.
func NewClock(at time.Time) *Clock { return &Clock{now: at} }

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the fake time forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Secrets is a port.SecretStore that masks bytes and refuses Open after Destroy.
type Secrets struct {
	mu        sync.Mutex
	destroyed map[port.TenantID]bool
}

// NewSecrets returns an empty fake.
func NewSecrets() *Secrets { return &Secrets{destroyed: map[port.TenantID]bool{}} }

func mask(b []byte) []byte {
	out := make([]byte, len(b))
	for i, x := range b {
		out[i] = x ^ 0x5a
	}
	return out
}

// Seal masks the plaintext under the key reference of the tenant.
func (s *Secrets) Seal(_ context.Context, t port.TenantID, plaintext []byte) (port.Sealed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed[t] {
		return port.Sealed{}, port.Errorf(port.CodeUnavailable, "key of %s is destroyed", t)
	}
	return port.Sealed{Ciphertext: mask(plaintext), KeyRef: "kek-" + string(t)}, nil
}

// Open unmasks a sealed record unless the tenant key was destroyed.
func (s *Secrets) Open(_ context.Context, t port.TenantID, sealed port.Sealed) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed[t] {
		return nil, port.Errorf(port.CodeUnavailable, "key of %s is destroyed", t)
	}
	return mask(sealed.Ciphertext), nil
}

// Rotate does nothing.
func (s *Secrets) Rotate(context.Context, port.KeyScope) error { return nil }

// Destroy makes every sealed record of the tenant unreadable.
func (s *Secrets) Destroy(_ context.Context, t port.TenantID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed[t] = true
	return nil
}

// Destroyed reports whether Destroy ran for the tenant.
func (s *Secrets) Destroyed(t port.TenantID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.destroyed[t]
}

// Invocation is one call the fake runtime received.
type Invocation struct {
	Function   string
	Arguments  json.RawMessage
	Credential port.CredentialID
}

// Runtime is a scripted port.ConnectorRuntime: definitions per listing name, handlers per function, a log of calls.
type Runtime struct {
	mu       sync.Mutex
	defs     map[string][]port.RawDefinition
	meta     map[string]port.ListMeta
	handlers map[string]func(json.RawMessage) (port.Result, error)
	handles  map[port.Handle]startInfo
	Calls    []Invocation
	Starts   int
	Stops    int
	OnStart  func(port.Connector) error
	OnList   func()
}

type startInfo struct {
	listing string
	secrets port.SecretsRef
}

// NewRuntime returns a fake with no listings.
func NewRuntime() *Runtime {
	return &Runtime{defs: map[string][]port.RawDefinition{}, meta: map[string]port.ListMeta{}, handlers: map[string]func(json.RawMessage) (port.Result, error){}, handles: map[port.Handle]startInfo{}}
}

// SetDefinitions sets what a listing's connectors capture and re-list.
func (r *Runtime) SetDefinitions(listing string, defs ...port.RawDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs[listing] = defs
}

// SetMeta sets the list metadata of a listing.
func (r *Runtime) SetMeta(listing string, m port.ListMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta[listing] = m
}

// Handle sets the answer of a function; the default echoes the arguments as structured content.
func (r *Runtime) Handle(function string, fn func(json.RawMessage) (port.Result, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[function] = fn
}

// Calls returns the number of calls to a function.
func (r *Runtime) CallsTo(function string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.Calls {
		if c.Function == function {
			n++
		}
	}
	return n
}

// Describe states the fake's capabilities.
func (r *Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{Kinds: []port.Kind{port.KindMCPRemote}}
}

// Start runs OnStart, then returns a handle bound to the listing of the manifest.
func (r *Runtime) Start(_ context.Context, c port.Connector, m port.Manifest, s port.SecretsRef) (port.Handle, error) {
	if r.OnStart != nil {
		if err := r.OnStart(c); err != nil {
			return "", err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Starts++
	h := port.Handle(string(c.ID) + "@" + m.Version + "/" + string(s.Credential))
	r.handles[h] = startInfo{listing: m.Name, secrets: s}
	return h, nil
}

// ListFunctions runs OnList, then returns the scripted definitions of the handle's listing.
func (r *Runtime) ListFunctions(_ context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	if r.OnList != nil {
		r.OnList()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	in := r.handles[h]
	return append([]port.RawDefinition(nil), r.defs[in.listing]...), r.meta[in.listing], nil
}

// Call runs the handler of the function and logs the call.
func (r *Runtime) Call(_ context.Context, h port.Handle, function string, args json.RawMessage, _ time.Time) (port.Result, error) {
	r.mu.Lock()
	r.Calls = append(r.Calls, Invocation{Function: function, Arguments: args, Credential: r.handles[h].secrets.Credential})
	fn := r.handlers[function]
	r.mu.Unlock()
	if fn != nil {
		return fn(args)
	}
	return port.Result{Content: json.RawMessage(`[{"type":"text","text":"ok"}]`), Structured: args}, nil
}

// Stop forgets the handle.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Stops++
	delete(r.handles, h)
	return nil
}

// Filter is a ResultFilter that masks the word "secret" and counts it.
type Filter struct{}

// Filter replaces "secret" with "***" in content and structured content.
func (Filter) Filter(_ context.Context, r port.Result) (port.Result, map[string]int, error) {
	n := bytes.Count(r.Content, []byte("secret")) + bytes.Count(r.Structured, []byte("secret"))
	if n == 0 {
		return r, nil, nil
	}
	r.Content = bytes.ReplaceAll(r.Content, []byte("secret"), []byte("***"))
	r.Structured = bytes.ReplaceAll(r.Structured, []byte("secret"), []byte("***"))
	return r, map[string]int{"secret": n}, nil
}

// Definition builds the raw MCP definition of a function and its hash.
func Definition(name, description, extra string) (port.RawDefinition, port.Hash) {
	body := `{"name":"` + name + `","description":"` + description + `","inputSchema":{"type":"object","properties":{"q":{"type":"string"}}}`
	if extra != "" {
		body += "," + extra
	}
	raw := json.RawMessage(body + "}")
	h, _ := pinning.DefinitionHash(raw)
	return port.RawDefinition{Name: name, Raw: raw}, h
}

// Manifest builds a valid mcp_remote manifest.
func Manifest(name, version string) port.Manifest {
	return port.Manifest{Name: name, Version: version, Title: strings.ToUpper(name[:1]) + name[1:], Kind: port.KindMCPRemote,
		Remote: &port.Remote{URL: "https://example.test/mcp", Transport: "streamable-http"}, Publisher: port.Publisher{Name: "Acme"}}
}
