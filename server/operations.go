// Package server holds the operation table every inbound surface (HTTP, gRPC, OpenAPI, management MCP) is built from.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/internal/plugkit/discovery"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

// WatchPoll is how long a watch sleeps between two reads of an empty stream.
const WatchPoll = time.Second

// Env is what the operations run on: the service and the adapter document of GET /v1/adapters.
type Env struct {
	Svc      *service.Service
	Adapters func() discovery.Document
}

// Operation is one API operation: its HTTP route, its RPC, its service call and its types.
type Operation struct {
	ID     string
	Method string
	Path   string
	RPC    string
	Status int
	Public bool
	Live   bool
	Never  bool
	In     func() any
	Out    any
	call   func(ctx context.Context, e *Env, p port.Principal, in any) (any, error)
}

type option func(*Operation)

func public() option { return func(o *Operation) { o.Public = true } }

func never() option { return func(o *Operation) { o.Never = true } }

func op[I, O any](id, method, path, rpc string, status int, fn func(context.Context, *Env, port.Principal, I) (O, error), opts ...option) Operation {
	o := Operation{
		ID: id, Method: method, Path: path, RPC: rpc, Status: status,
		In: func() any { return new(I) }, Out: *new(O),
		call: func(ctx context.Context, e *Env, p port.Principal, in any) (any, error) {
			return fn(ctx, e, p, *in.(*I))
		},
	}
	for _, f := range opts {
		f(&o)
	}
	return o
}

func watch[I any](id, path, rpc string, fn func(context.Context, *Env, port.Principal, I) ([]port.Event, error)) Operation {
	return Operation{
		ID: id, Method: http.MethodGet, Path: path, RPC: rpc, Status: http.StatusOK, Live: true,
		In: func() any { return new(I) }, Out: port.Event{},
		call: func(ctx context.Context, e *Env, p port.Principal, in any) (any, error) {
			return fn(ctx, e, p, *in.(*I))
		},
	}
}

// Operations returns the operation table.
func Operations() []Operation { return operations }

// Reads reports whether the operation only reads: a list, get or explain, or lintListing.
func (o Operation) Reads() bool {
	return o.ID == "lintListing" || strings.HasPrefix(o.ID, "list") || strings.HasPrefix(o.ID, "get") || strings.HasPrefix(o.ID, "explain")
}

// QueryParams lists the fields a bodiless request carries in its query string: every field that is not in the path.
func (o Operation) QueryParams() []string {
	if o.HasBody() {
		return nil
	}
	var out []string
	for _, f := range Fields(reflect.TypeOf(o.In()).Elem()) {
		if !slices.Contains(o.PathParams(), f.Name) {
			out = append(out, f.Name)
		}
	}
	return out
}

// HasBody reports whether the HTTP request carries a JSON body.
func (o Operation) HasBody() bool {
	return o.Method == http.MethodPost || o.Method == http.MethodPut || o.Method == http.MethodPatch
}

// PathParams lists the wildcard names of the path, each also the JSON field it fills.
func (o Operation) PathParams() []string {
	var out []string
	for seg := range strings.SplitSeq(o.Path, "/") {
		if name, ok := strings.CutPrefix(seg, "{"); ok {
			out = append(out, strings.SplitN(name, "}", 2)[0])
		}
	}
	return out
}

// Match reports whether path is this operation's route and returns its wildcard values.
func (o Operation) Match(path string) (map[string]string, bool) {
	want, got := strings.Split(o.Path, "/"), strings.Split(path, "/")
	if len(want) != len(got) {
		return nil, false
	}
	params := map[string]string{}
	for i, tpl := range want {
		name, ok := strings.CutPrefix(tpl, "{")
		if !ok {
			if tpl != got[i] {
				return nil, false
			}
			continue
		}
		name, suffix, _ := strings.Cut(name, "}")
		value, found := strings.CutSuffix(got[i], suffix)
		if !found || value == "" || strings.Contains(value, ":") {
			return nil, false
		}
		params[name] = value
	}
	return params, true
}

// Fill sets the JSON fields of in named by values, parsing each from its string form.
func Fill(in any, values map[string]string) error {
	v := reflect.ValueOf(in).Elem()
	fields := Fields(v.Type())
	for name, raw := range values {
		i := indexOf(fields, name)
		if i < 0 {
			return port.Errorf(port.CodeInvalidArgument, "unknown parameter %q", name)
		}
		if err := parse(v.FieldByIndex(fields[i].Index), raw); err != nil {
			return port.Errorf(port.CodeInvalidArgument, "parameter %q: %v", name, err)
		}
	}
	return nil
}

func indexOf(fields []Field, name string) int {
	for i, f := range fields {
		if f.Name == name {
			return i
		}
	}
	return -1
}

func parse(dst reflect.Value, raw string) error {
	if dst.Type() == TimeType {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil {
			dst.Set(reflect.ValueOf(t))
		}
		return err
	}
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(raw)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		dst.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		dst.SetBool(b)
	default:
		return port.Errorf(port.CodeInvalidArgument, "cannot be set from a string")
	}
	return nil
}

// Invoke runs a unary operation for the caller; in is the pointer o.In returned.
func (o Operation) Invoke(ctx context.Context, e *Env, p port.Principal, idempotencyKey string, in any) (any, error) {
	if idempotencyKey == "" {
		return o.call(ctx, e, p, in)
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append([]byte(o.ID+"\x00"), body...))
	ctx = service.WithIdempotencyKey(ctx, idempotencyKey, hex.EncodeToString(sum[:]))
	out, err := o.call(ctx, e, p, in)
	if stored, replayed, rerr := service.Replay(ctx); replayed {
		if rerr != nil {
			return nil, rerr
		}
		return o.decodeStored(stored, out, err)
	}
	if err == nil {
		if rerr := e.Svc.Remember(ctx, out); rerr != nil {
			return out, rerr
		}
	}
	return out, err
}

func (o Operation) decodeStored(stored []byte, out any, err error) (any, error) {
	if len(stored) == 0 || o.Out == nil {
		return out, err
	}
	v := reflect.New(reflect.TypeOf(o.Out))
	if json.Unmarshal(stored, v.Interface()) != nil {
		return out, err
	}
	return v.Elem().Interface(), nil
}

// Watch streams a live operation until ctx ends: it sends every event after the cursor and sleeps WatchPoll when there is none.
func (o Operation) Watch(ctx context.Context, e *Env, p port.Principal, in any, ready func(), send func(port.Event) error) error {
	first := true
	for ctx.Err() == nil {
		out, err := o.call(ctx, e, p, in)
		if err != nil {
			return err
		}
		if first {
			ready()
			first = false
		}
		events := out.([]port.Event)
		for _, ev := range events {
			if err := send(ev); err != nil {
				return err
			}
			in.(Cursor).Advance(ev.Sequence)
		}
		if len(events) == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(WatchPoll):
			}
		}
	}
	return nil
}

// Cursor is the input of a live operation: it remembers the last sequence sent.
type Cursor interface{ Advance(sequence int64) }
