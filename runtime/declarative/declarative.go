// Package declarative is the in-process ConnectorRuntime that executes the rest block of a rest_declarative listing.
package declarative

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/credential"
	deadlines "github.com/agents-control/connector-engine/internal/deadline"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/internal/handles"
	"github.com/agents-control/connector-engine/internal/hints"
	"github.com/agents-control/connector-engine/port"
)

const maxResponse = 8 << 20

var pathParam = regexp.MustCompile(`\{([^{}/]+)\}`)

type instance struct {
	base   string
	client *http.Client
	order  []port.RESTFunction
	byName map[string]port.RESTFunction
}

// Runtime executes declared REST functions against the hosts of egress.allow_hosts only.
type Runtime struct {
	applier credential.Applier
	base    http.RoundTripper
	guard   egress.Guard
	started *handles.Registry[*instance]
}

// New returns a runtime that injects credentials through applier; a nil base dials through guard.
func New(applier credential.Applier, base http.RoundTripper, guard egress.Guard) *Runtime {
	return &Runtime{applier: applier, base: base, guard: guard, started: handles.New[*instance]("rest")}
}

// Describe states a runtime with no processes, whose connectors are data, reaching the allowed hosts only.
func (*Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{Kinds: []port.Kind{port.KindRESTDeclarative}, Isolation: "none", Egress: "egress.allow_hosts"}
}

// Start prepares a client limited to the manifest's allowed hosts.
func (r *Runtime) Start(_ context.Context, _ port.Connector, m port.Manifest, secrets port.SecretsRef) (port.Handle, error) {
	if m.Kind != port.KindRESTDeclarative || m.REST == nil {
		return "", port.Errorf(port.CodeInvalidManifest, "the declarative runtime serves rest_declarative listings with a rest block")
	}
	if m.Egress == nil || len(m.Egress.AllowHosts) == 0 {
		return "", port.Errorf(port.CodeInvalidManifest, "a rest_declarative listing carries egress.allow_hosts")
	}
	base, err := url.Parse(m.REST.BaseURL)
	if err != nil || base.Host == "" {
		return "", port.Errorf(port.CodeInvalidManifest, "rest.base_url %q is not an address", m.REST.BaseURL)
	}
	if !credential.HostAllowed(m.Egress.AllowHosts, base.Host, base.Hostname()) {
		return "", port.Errorf(port.CodeInvalidManifest, "rest.base_url host %s is not in egress.allow_hosts", base.Host)
	}
	transport := r.base
	if transport == nil {
		transport = r.guard.Transport(m.Egress.AllowHosts...)
	}
	in := &instance{
		base:   strings.TrimRight(m.REST.BaseURL, "/"),
		client: &http.Client{Transport: credential.Transport{Base: transport, Applier: r.applier, Ref: secrets, AllowHosts: m.Egress.AllowHosts}},
		order:  m.REST.Functions,
		byName: map[string]port.RESTFunction{},
	}
	for _, f := range m.REST.Functions {
		in.byName[f.Name] = f
	}
	return r.started.Add(in), nil
}

// ListFunctions returns the declared functions as definitions.
func (r *Runtime) ListFunctions(_ context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	defs := make([]port.RawDefinition, len(in.order))
	for i, f := range in.order {
		def := map[string]any{"name": f.Name, "inputSchema": schemaOr(f.InputSchema), "_meta": hints.Meta("", f.Method)}
		if f.Description != "" {
			def["description"] = f.Description
		}
		if len(f.OutputSchema) > 0 {
			def["outputSchema"] = f.OutputSchema
		}
		raw, err := json.Marshal(def)
		if err != nil {
			return nil, port.ListMeta{}, port.Errorf(port.CodeInvalidManifest, "function %s: %v", f.Name, err)
		}
		defs[i] = port.RawDefinition{Name: f.Name, Raw: raw}
	}
	return defs, port.ListMeta{}, nil
}

func schemaOr(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

// Call substitutes path parameters, sends the rest as the query or a JSON body and maps the response to content and structured content.
func (r *Runtime) Call(ctx context.Context, h port.Handle, function string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	f, ok := in.byName[function]
	if !ok {
		return port.Result{}, port.Errorf(port.CodeNotFound, "no declared function %q", function)
	}
	req, err := in.request(ctx, f, arguments)
	if err != nil {
		return port.Result{}, err
	}
	ctx, cancel := deadlines.Apply(ctx, deadline)
	defer cancel()
	resp, err := in.client.Do(req.WithContext(ctx))
	if err != nil {
		if ctx.Err() != nil {
			return port.Result{}, ctx.Err()
		}
		return port.Result{}, port.Errorf(port.CodeUnavailable, "%s %s: %v", f.Method, f.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(body) > maxResponse {
		return port.Result{}, port.Errorf(port.CodeUnavailable, "%s %s: the response is unreadable or over %d bytes", f.Method, f.Path, maxResponse)
	}
	return result(resp.StatusCode, body), nil
}

func (in *instance) request(ctx context.Context, f port.RESTFunction, arguments json.RawMessage) (*http.Request, error) {
	args := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(arguments)) > 0 && json.Unmarshal(arguments, &args) != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "the arguments of %s are a JSON object", f.Name)
	}
	path, err := expandPath(f.Path, args)
	if err != nil {
		return nil, err
	}
	query, body := splitArguments(f, args)
	target := in.base + path
	if len(query) > 0 {
		values := url.Values{}
		for k, raw := range query {
			vs, err := scalars(raw)
			if err != nil {
				return nil, port.Errorf(port.CodeInvalidArgument, "query parameter %s: %v", k, err)
			}
			values[k] = vs
		}
		target += "?" + values.Encode()
	}
	var reader io.Reader
	if len(body) > 0 {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(f.Method), target, reader)
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidManifest, "%s %s: %v", f.Method, f.Path, err)
	}
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func expandPath(path string, args map[string]json.RawMessage) (string, error) {
	var missing error
	out := pathParam.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1 : len(m)-1]
		raw, ok := args[name]
		if !ok {
			missing = port.Errorf(port.CodeInvalidArgument, "path parameter %s is missing", name)
			return m
		}
		delete(args, name)
		vs, err := scalars(raw)
		if err != nil || len(vs) != 1 {
			missing = port.Errorf(port.CodeInvalidArgument, "path parameter %s is one scalar", name)
			return m
		}
		return url.PathEscape(vs[0])
	})
	return out, missing
}

func splitArguments(f port.RESTFunction, args map[string]json.RawMessage) (query, body map[string]json.RawMessage) {
	query, body = map[string]json.RawMessage{}, map[string]json.RawMessage{}
	var schema struct {
		Properties map[string]struct {
			In string `json:"x-in"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(f.InputSchema, &schema)
	bodyless := strings.EqualFold(f.Method, "GET") || strings.EqualFold(f.Method, "HEAD") || strings.EqualFold(f.Method, "DELETE")
	for k, v := range args {
		if schema.Properties[k].In == "query" || bodyless {
			query[k] = v
		} else {
			body[k] = v
		}
	}
	return query, body
}

func scalars(raw json.RawMessage) ([]string, error) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		list = []json.RawMessage{raw}
	}
	out := make([]string, len(list))
	for i, item := range list {
		var s string
		var v any
		switch {
		case json.Unmarshal(item, &s) == nil:
			out[i] = s
		case json.Unmarshal(item, &v) == nil && v != nil && !isContainer(item):
			out[i] = string(item)
		default:
			return nil, fmt.Errorf("a scalar or a list of scalars is expected")
		}
	}
	return out, nil
}

func isContainer(raw json.RawMessage) bool {
	return len(raw) > 0 && (raw[0] == '{' || raw[0] == '[')
}

func result(status int, body []byte) port.Result {
	text, _ := json.Marshal([]map[string]string{{"type": "text", "text": string(body)}})
	res := port.Result{Content: text, IsError: status < 200 || status > 299}
	if len(bytes.TrimSpace(body)) == 0 || !json.Valid(body) {
		return res
	}
	if trimmed := bytes.TrimSpace(body); trimmed[0] == '{' {
		res.Structured = bytes.Clone(trimmed)
	} else {
		res.Structured, _ = json.Marshal(map[string]json.RawMessage{"result": trimmed})
	}
	return res
}

// Stop forgets the instance.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	if _, ok := r.started.Remove(h); !ok {
		return port.Errorf(port.CodeNotFound, "no started instance %q", h)
	}
	return nil
}
