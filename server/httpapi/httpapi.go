// Package httpapi serves the operation table over net/http: JSON routes under /v1 and server-sent events for the live ones.
package httpapi

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/server"
)

// Options configure the handler; Env and Authn are required.
type Options struct {
	Env          *server.Env
	Authn        server.Authenticator
	MaxBodyBytes int64
	Log          *slog.Logger
}

type api struct{ Options }

// New builds the handler of every operation of the table; it fails without an environment or an authenticator.
func New(o Options) (http.Handler, error) {
	if o.Env == nil || o.Env.Svc == nil || o.Authn == nil {
		return nil, errors.New("httpapi: an environment and an authenticator are required")
	}
	o.MaxBodyBytes = cmp.Or(o.MaxBodyBytes, 1<<20)
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return &api{o}, nil
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	known := false
	for _, op := range server.Operations() {
		params, ok := op.Match(r.URL.Path)
		switch {
		case !ok:
		case op.Method == r.Method:
			a.serve(w, r, op, params)
			return
		default:
			known = true
		}
	}
	if known {
		a.write(w, server.Problem{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Detail: "method not allowed"})
		return
	}
	a.fail(w, r, port.Errorf(port.CodeNotFound, "no route %s", r.URL.Path))
}

func (a *api) serve(w http.ResponseWriter, r *http.Request, op server.Operation, params map[string]string) {
	p, err := server.Authenticate(r.Context(), op, a.Authn, server.Bearer(r.Header.Get(server.HeaderAuthorization)))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	in := op.In()
	if op.HasBody() {
		err = a.decode(w, r, in)
	}
	values := params
	for _, name := range op.QueryParams() {
		if q := r.URL.Query(); q.Has(name) {
			values[name] = q.Get(name)
		}
	}
	if last := r.Header.Get("Last-Event-ID"); op.Live && last != "" && !r.URL.Query().Has("after") {
		values["after"] = last
	}
	if err == nil {
		err = server.Fill(in, values)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if op.Live {
		a.watch(w, r, op, p, in)
		return
	}
	out, err := op.Invoke(r.Context(), a.Env, p, r.Header.Get(server.HeaderIdempotencyKey), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if op.Status == http.StatusNoContent {
		w.WriteHeader(op.Status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(op.Status)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		a.Log.WarnContext(r.Context(), "response write failed", "operation", op.ID, "error", err)
	}
}

func (a *api) decode(w http.ResponseWriter, r *http.Request, into any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.MaxBodyBytes))
	if err != nil {
		return port.Errorf(port.CodeInvalidArgument, "request body: %v", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return port.Errorf(port.CodeInvalidArgument, "request body: %v", err)
	}
	return nil
}

func (a *api) watch(w http.ResponseWriter, r *http.Request, op server.Operation, p port.Principal, in any) {
	rc, started := http.NewResponseController(w), false
	ready := func() {
		started = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_ = rc.Flush()
	}
	send := func(e port.Event) error {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Sequence, e.Type, data); err != nil {
			return err
		}
		return rc.Flush()
	}
	if err := op.Watch(r.Context(), a.Env, p, in, ready, send); err != nil && !started {
		a.fail(w, r, err)
	}
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	p := server.ProblemOf(err)
	if p.Code == port.CodeInternal {
		a.Log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}
	a.write(w, p)
}

func (a *api) write(w http.ResponseWriter, p server.Problem) {
	w.Header().Set("Content-Type", server.ProblemMediaType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
