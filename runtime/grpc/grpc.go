// Package grpc is the ConnectorRuntime for grpc_declarative listings: descriptors from reflection or a descriptor set, calls through dynamicpb and protojson.
package grpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/versatile-sh/connector-engine/credential"
	deadlines "github.com/versatile-sh/connector-engine/internal/deadline"
	"github.com/versatile-sh/connector-engine/internal/digest"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/fetch"
	"github.com/versatile-sh/connector-engine/internal/grpcdesc"
	"github.com/versatile-sh/connector-engine/internal/handles"
	"github.com/versatile-sh/connector-engine/internal/hints"
	"github.com/versatile-sh/connector-engine/internal/outcome"
	"github.com/versatile-sh/connector-engine/port"
)

// Fetcher returns the bytes of a FileDescriptorSet a manifest points to.
type Fetcher func(ctx context.Context, d port.Descriptor) ([]byte, error)

// Options configure a Runtime; the zero value reaches servers over TLS and reads descriptor sets from paths and http(s) addresses.
type Options struct {
	Applier  credential.Applier
	Insecure bool
	Fetch    Fetcher
	Dial     []grpc.DialOption
	Guard    egress.Guard
}

type described struct {
	types   *dynamicpb.Types
	order   []string
	methods map[string]grpcdesc.Method
}

type instance struct {
	conn *grpc.ClientConn
	spec *port.GRPC
	desc atomic.Pointer[described]
}

// Runtime calls unary gRPC methods declared by a listing; streaming methods are refused (grpc.unaryOnly).
type Runtime struct {
	opts    Options
	started *handles.Registry[*instance]
}

// New returns a runtime with the given options.
func New(opts Options) *Runtime {
	if opts.Fetch == nil {
		opts.Fetch = DefaultFetcher(opts.Guard.Client())
	}
	return &Runtime{opts: opts, started: handles.New[*instance]("grpc")}
}

// DefaultFetcher reads a file_descriptor_set from a path or an http(s) address; a bsr descriptor needs a Fetcher of its own.
func DefaultFetcher(c *http.Client) Fetcher {
	return func(ctx context.Context, d port.Descriptor) ([]byte, error) {
		switch {
		case d.From != port.DescriptorFileSet:
			return nil, port.Errorf(port.CodeInvalidManifest, "no fetcher is configured for %s descriptors", d.From)
		case strings.HasPrefix(d.Ref, "http://") || strings.HasPrefix(d.Ref, "https://"):
			return fetch.Do(ctx, c, http.MethodGet, d.Ref, nil, nil)
		}
		b, err := os.ReadFile(d.Ref)
		if err != nil {
			return nil, port.Errorf(port.CodeNotFound, "descriptor set %s: %v", d.Ref, err)
		}
		return b, nil
	}
}

// Describe states a runtime with no processes that reaches one address and serves unary methods only.
func (*Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{Kinds: []port.Kind{port.KindGRPCDeclarative}, Isolation: "none", Egress: "grpc.target", UnaryOnly: true}
}

type perRPC struct {
	applier credential.Applier
	ref     port.SecretsRef
	secure  bool
}

func (p perRPC) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	headers, err := p.applier.Headers(ctx, p.ref)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range headers {
		out[strings.ToLower(k)] = strings.Join(v, ",")
	}
	return out, nil
}

func (p perRPC) RequireTransportSecurity() bool { return p.secure }

func (r *Runtime) descriptors(ctx context.Context, conn *grpc.ClientConn, d port.Descriptor) (*protoregistry.Files, error) {
	if d.From == port.DescriptorReflection {
		return grpcdesc.FromReflection(ctx, conn)
	}
	if d.Digest == "" {
		return nil, port.Errorf(port.CodeInvalidManifest, "a %s descriptor carries a digest", d.From)
	}
	b, err := r.opts.Fetch(ctx, d)
	if err != nil {
		return nil, err
	}
	if !digest.Matches(d.Digest, b) {
		return nil, port.Errorf(port.CodeInvalidManifest, "the descriptor set %s does not match its digest", d.Ref)
	}
	return grpcdesc.FromSet(b)
}

// Start dials the target, loads the descriptors and resolves every declared method; a streaming or unknown method fails the start.
func (r *Runtime) Start(ctx context.Context, _ port.Connector, m port.Manifest, secrets port.SecretsRef) (port.Handle, error) {
	if m.Kind != port.KindGRPCDeclarative || m.GRPC == nil || m.GRPC.Target == "" {
		return "", port.Errorf(port.CodeInvalidManifest, "the grpc runtime serves grpc_declarative listings with a target")
	}
	transport := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	if r.opts.Insecure {
		transport = insecure.NewCredentials()
	}
	var allow []string
	if m.Egress != nil {
		allow = m.Egress.AllowHosts
	}
	dial := append([]grpc.DialOption{
		grpc.WithTransportCredentials(transport),
		r.opts.Guard.GRPC(allow...),
		grpc.WithPerRPCCredentials(perRPC{applier: r.opts.Applier, ref: secrets, secure: !r.opts.Insecure}),
	}, r.opts.Dial...)
	conn, err := grpc.NewClient(m.GRPC.Target, dial...)
	if err != nil {
		return "", port.Errorf(port.CodeInvalidManifest, "grpc target %q: %v", m.GRPC.Target, err)
	}
	in := &instance{conn: conn, spec: m.GRPC}
	if err := r.load(ctx, in); err != nil {
		conn.Close()
		return "", err
	}
	return r.started.Add(in), nil
}

func (r *Runtime) load(ctx context.Context, in *instance) error {
	files, err := r.descriptors(ctx, in.conn, in.spec.Descriptor)
	if err != nil {
		return err
	}
	d := &described{types: dynamicpb.NewTypes(files), methods: map[string]grpcdesc.Method{}}
	for _, f := range in.spec.Methods {
		md, err := grpcdesc.Unary(files, f.FullMethod)
		if err != nil {
			return err
		}
		m := grpcdesc.Describe(md)
		m.Function.Name = f.Name
		d.order = append(d.order, f.Name)
		d.methods[f.Name] = m
	}
	in.desc.Store(d)
	return nil
}

// ListFunctions loads the descriptors again and returns one definition per declared method, with the class hints in _meta.
func (r *Runtime) ListFunctions(ctx context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	if err := r.load(ctx, in); err != nil {
		return nil, port.ListMeta{}, err
	}
	desc := in.desc.Load()
	out := make([]port.RawDefinition, len(desc.order))
	for i, name := range desc.order {
		d := desc.methods[name]
		raw, err := json.Marshal(map[string]any{
			"name":         name,
			"description":  "gRPC " + d.Function.FullMethod,
			"inputSchema":  d.Function.InputSchema,
			"outputSchema": d.Function.OutputSchema,
			"_meta":        hints.Meta(d.Idempotency, d.HTTPVerb),
		})
		if err != nil {
			return nil, port.ListMeta{}, port.Errorf(port.CodeInternal, "definition %s: %v", name, err)
		}
		out[i] = port.RawDefinition{Name: name, Raw: raw}
	}
	return out, port.ListMeta{}, nil
}

// Call builds the request message from the JSON arguments, invokes the method and returns the response as JSON; a status other than unavailable or deadline_exceeded is an error result.
func (r *Runtime) Call(ctx context.Context, h port.Handle, function string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	desc := in.desc.Load()
	m, ok := desc.methods[function]
	if !ok {
		return port.Result{}, port.Errorf(port.CodeNotFound, "no declared method %q", function)
	}
	req := dynamicpb.NewMessage(m.Descriptor.Input())
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if err := (protojson.UnmarshalOptions{Resolver: desc.types}).Unmarshal(arguments, req); err != nil {
		return port.Result{}, port.Errorf(port.CodeInvalidArgument, "arguments of %s: %v", function, err)
	}
	ctx, cancel := deadlines.Apply(ctx, deadline)
	defer cancel()
	resp := dynamicpb.NewMessage(m.Descriptor.Output())
	if err := in.conn.Invoke(ctx, m.Function.FullMethod, req, resp); err != nil {
		return failure(ctx, function, err)
	}
	body, err := (protojson.MarshalOptions{Resolver: desc.types}).Marshal(resp)
	if err != nil {
		return port.Result{}, port.Errorf(port.CodeInternal, "response of %s: %v", function, err)
	}
	return port.Result{Content: textContent(string(body)), Structured: body}, nil
}

func failure(ctx context.Context, function string, err error) (port.Result, error) {
	st := status.Convert(err)
	switch st.Code() {
	case codes.DeadlineExceeded, codes.Canceled:
		return port.Result{}, outcome.Of(ctx, err, "%s: %v", function)
	case codes.Unavailable:
		if ctx.Err() != nil {
			return port.Result{}, ctx.Err()
		}
		return port.Result{}, port.Errorf(port.CodeUnavailable, "%s: %v", function, err)
	}
	return port.Result{Content: textContent(st.Code().String() + ": " + st.Message()), IsError: true}, nil
}

func textContent(s string) json.RawMessage {
	b, _ := json.Marshal([]map[string]string{{"type": "text", "text": s}})
	return b
}

// Stop closes the connection.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	in, ok := r.started.Remove(h)
	if !ok {
		return port.Errorf(port.CodeNotFound, "no started instance %q", h)
	}
	return in.conn.Close()
}
