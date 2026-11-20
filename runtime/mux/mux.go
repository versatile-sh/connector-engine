// Package mux is the ConnectorRuntime that sends each listing to the runtime serving its kind.
package mux

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/port"
)

// Runtime routes Start by manifest kind and every later call by the runtime that started the handle.
type Runtime struct {
	byKind  map[port.Kind]port.ConnectorRuntime
	mu      sync.Mutex
	started map[port.Handle]port.ConnectorRuntime
	caps    port.RuntimeCapabilities
}

var (
	_ port.ConnectorRuntime = (*Runtime)(nil)
	_ port.ContentRuntime   = (*Runtime)(nil)
)

// New returns a Runtime over runtimes; a kind served by two of them goes to the first, and no runtime at all is refused.
func New(runtimes ...port.ConnectorRuntime) (*Runtime, error) {
	if len(runtimes) == 0 {
		return nil, port.Errorf(port.CodeInvalidArgument, "at least one runtime is required")
	}
	r := &Runtime{byKind: map[port.Kind]port.ConnectorRuntime{}, started: map[port.Handle]port.ConnectorRuntime{}, caps: port.RuntimeCapabilities{Isolation: "none", UnaryOnly: true}}
	for _, rt := range runtimes {
		c := rt.Describe()
		for _, k := range c.Kinds {
			if _, taken := r.byKind[k]; !taken {
				r.byKind[k] = rt
				r.caps.Kinds = append(r.caps.Kinds, k)
			}
		}
		r.caps.SecretVisible = r.caps.SecretVisible || c.SecretVisible
		r.caps.UnaryOnly = r.caps.UnaryOnly && c.UnaryOnly
		r.caps.Processes += c.Processes
	}
	slices.Sort(r.caps.Kinds)
	r.caps.Egress = "per kind"
	return r, nil
}

// Describe states the union of the kinds served; its isolation is the weakest of its runtimes, none.
func (r *Runtime) Describe() port.RuntimeCapabilities { return r.caps }

// Start starts the manifest on the runtime of its kind.
func (r *Runtime) Start(ctx context.Context, c port.Connector, m port.Manifest, s port.SecretsRef) (port.Handle, error) {
	rt, ok := r.byKind[m.Kind]
	if !ok {
		return "", port.Errorf(port.CodeInvalidManifest, "no runtime serves kind %q", m.Kind)
	}
	h, err := rt.Start(ctx, c, m, s)
	if err == nil {
		r.mu.Lock()
		r.started[h] = rt
		r.mu.Unlock()
	}
	return h, err
}

func (r *Runtime) of(h port.Handle) (port.ConnectorRuntime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.started[h]
	if !ok {
		return nil, port.Errorf(port.CodeNotFound, "handle %s is not started", h)
	}
	return rt, nil
}

// ListFunctions lists the functions of a started instance.
func (r *Runtime) ListFunctions(ctx context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	rt, err := r.of(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	return rt.ListFunctions(ctx, h)
}

// Call calls a function of a started instance.
func (r *Runtime) Call(ctx context.Context, h port.Handle, function string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	rt, err := r.of(h)
	if err != nil {
		return port.Result{}, err
	}
	return rt.Call(ctx, h, function, arguments, deadline)
}

// Stop stops an instance and forgets its handle.
func (r *Runtime) Stop(ctx context.Context, h port.Handle) error {
	rt, err := r.of(h)
	if err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.started, h)
	r.mu.Unlock()
	return rt.Stop(ctx, h)
}

var errNoContent = port.Errorf(port.CodeInvalidArgument, "the runtime of this handle serves no prompts or resources")

func (r *Runtime) content(h port.Handle) (port.ContentRuntime, error) {
	rt, err := r.of(h)
	if err != nil {
		return nil, err
	}
	c, ok := rt.(port.ContentRuntime)
	if !ok {
		return nil, errNoContent
	}
	return c, nil
}

// ListContent lists the prompts and resources of a started instance; a runtime that serves none lists nothing.
func (r *Runtime) ListContent(ctx context.Context, h port.Handle) ([]port.RawDefinition, error) {
	c, err := r.content(h)
	if errors.Is(err, errNoContent) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c.ListContent(ctx, h)
}

// Content gets a prompt or reads a resource of a started instance.
func (r *Runtime) Content(ctx context.Context, h port.Handle, item string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	c, err := r.content(h)
	if err != nil {
		return port.Result{}, err
	}
	return c.Content(ctx, h, item, arguments, deadline)
}

// Complete asks a started instance to complete an argument of a prompt or resource template.
func (r *Runtime) Complete(ctx context.Context, h port.Handle, item string, request json.RawMessage) (json.RawMessage, error) {
	c, err := r.content(h)
	if err != nil {
		return nil, err
	}
	return c.Complete(ctx, h, item, request)
}
