// Package memory is a scripted ConnectorRuntime for tests of the layers above it.
package memory

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	deadlines "github.com/agents-control/connector-engine/internal/deadline"
	"github.com/agents-control/connector-engine/internal/handles"
	"github.com/agents-control/connector-engine/port"
)

// Handler answers one call of a scripted connector.
type Handler func(ctx context.Context, function string, arguments json.RawMessage) (port.Result, error)

// Script is what a scripted connector lists and how it answers.
type Script struct {
	Definitions []port.RawDefinition
	Meta        port.ListMeta
	Handler     Handler
}

// Call is one call the runtime received.
type Call struct {
	Connector port.ConnectorID
	Function  string
	Arguments json.RawMessage
	Deadline  time.Time
	Secrets   port.SecretsRef
}

type instance struct {
	connector port.ConnectorID
	secrets   port.SecretsRef
}

// Runtime is a ConnectorRuntime that serves scripts registered per connector.
type Runtime struct {
	mu      sync.Mutex
	scripts map[port.ConnectorID]Script
	calls   []Call
	started *handles.Registry[instance]
}

// New returns a runtime with no scripts.
func New() *Runtime {
	return &Runtime{scripts: map[port.ConnectorID]Script{}, started: handles.New[instance]("memory")}
}

// Script sets what the connector lists and answers, taking effect on the next list or call.
func (r *Runtime) Script(id port.ConnectorID, s Script) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[id] = s
}

// Calls returns the calls received so far, oldest first.
func (r *Runtime) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *Runtime) script(id port.ConnectorID) (Script, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scripts[id]
	if !ok {
		return s, port.Errorf(port.CodeNotFound, "no script for connector %q", id)
	}
	return s, nil
}

// Describe states a runtime that serves every kind with no isolation.
func (r *Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{
		Kinds:     []port.Kind{port.KindMCPRemote, port.KindMCPPackage, port.KindRESTDeclarative, port.KindGRPCDeclarative},
		Isolation: "none",
		Egress:    "none",
	}
}

// Start registers an instance of the connector's script.
func (r *Runtime) Start(_ context.Context, c port.Connector, _ port.Manifest, secrets port.SecretsRef) (port.Handle, error) {
	if _, err := r.script(c.ID); err != nil {
		return "", err
	}
	return r.started.Add(instance{connector: c.ID, secrets: secrets}), nil
}

// ListFunctions returns the script's current definitions.
func (r *Runtime) ListFunctions(_ context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	s, err := r.script(in.connector)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	return slices.Clone(s.Definitions), s.Meta, nil
}

// Call records the call and runs the script's handler under the deadline.
func (r *Runtime) Call(ctx context.Context, h port.Handle, function string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	s, err := r.script(in.connector)
	if err != nil {
		return port.Result{}, err
	}
	r.mu.Lock()
	r.calls = append(r.calls, Call{Connector: in.connector, Function: function, Arguments: slices.Clone(arguments), Deadline: deadline, Secrets: in.secrets})
	r.mu.Unlock()
	if s.Handler == nil {
		return port.Result{}, port.Errorf(port.CodeUnavailable, "the script of %q has no handler", in.connector)
	}
	ctx, cancel := deadlines.Apply(ctx, deadline)
	defer cancel()
	return s.Handler(ctx, function, arguments)
}

// Stop forgets the instance.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	if _, ok := r.started.Remove(h); !ok {
		return port.Errorf(port.CodeNotFound, "no started instance %q", h)
	}
	return nil
}
