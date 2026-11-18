package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/relay"
	"github.com/agents-control/connector-engine/server"
	"github.com/agents-control/connector-engine/server/grpcapi"
	"github.com/agents-control/connector-engine/server/httpapi"
	"github.com/agents-control/connector-engine/server/mcpapi"
)

func (a *App) api(authn server.Authenticator) (http.Handler, error) {
	return httpapi.New(httpapi.Options{Env: a.env, Authn: authn, MaxBodyBytes: a.Config.Server.MaxBodyBytes, Log: a.log})
}

func (a *App) probes(mux *http.ServeMux) {
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.Read(r.Context(), port.Scope{Installation: true}, func(port.Tx) error { return nil }); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// Handler is the public listener: /v1, /mcp/manage, /mcp when an agent endpoint is mounted, the probes and, for the local issuer, its keys and token endpoint.
func (a *App) Handler() (http.Handler, error) {
	mux := http.NewServeMux()
	api, err := a.api(a.personAuthn())
	if err != nil {
		return nil, err
	}
	manage, err := mcpapi.New(mcpapi.Options{Env: a.env, Authn: a.personAuthn(), Mutations: a.Config.Mutations(), Version: a.build})
	if err != nil {
		return nil, err
	}
	mux.Handle("/v1/", api)
	mux.Handle("/mcp/manage", manage)
	mux.Handle("/mcp/manage/", manage)
	if a.agent != nil {
		mux.Handle("/mcp", a.agent)
		mux.Handle("/mcp/", a.agent)
	}
	if a.issuer != nil {
		mux.Handle("GET /local/jwks", a.issuer.Handler())
		mux.HandleFunc("POST /local/token", a.localToken)
	}
	a.probes(mux)
	return mux, nil
}

func (a *App) operatorHandler() (http.Handler, error) {
	mux := http.NewServeMux()
	api, err := a.api(a.operatorAuthn())
	if err != nil {
		return nil, err
	}
	mux.Handle("/v1/", api)
	a.probes(mux)
	return mux, nil
}

type tokenRequest struct {
	port.Claims
	TTL string `json:"ttl,omitempty"`
}

func (a *App) localToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ttl, err := time.ParseDuration(req.TTL)
	if req.TTL == "" {
		ttl, err = time.Hour, nil
	}
	if err != nil {
		http.Error(w, "bad ttl", http.StatusBadRequest)
		return
	}
	c := req.Claims
	c.ExpiresAt = a.Clock.Now().Add(ttl)
	if len(c.Audience) == 0 && c.Agent != "" {
		c.Audience = []string{a.Config.Audience()}
	} else if len(c.Audience) == 0 {
		c.Audience = []string{a.Config.ManageAudience()}
	}
	token, err := a.issuer.Issue(c)
	if err != nil {
		http.Error(w, "cannot issue", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (a *App) syncSources(ctx context.Context) {
	t := time.NewTicker(a.Config.Sources.SyncInterval)
	defer t.Stop()
	system := port.Principal{Kind: port.PrincipalSystem}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for name := range a.sources {
				if _, err := a.Service.SyncSource(ctx, system, name); err != nil {
					a.log.WarnContext(ctx, "source sync failed", "source", name, "error", err)
				}
			}
		}
	}
}

func listen(addr string) (net.Listener, error) {
	if addr == "" {
		return nil, nil
	}
	return net.Listen("tcp", addr)
}

func (a *App) serveHTTP(g *errgroup.Group, ctx context.Context, lis net.Listener, h http.Handler) {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	g.Go(func() error {
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), a.Config.Server.ShutdownGrace)
		defer cancel()
		return srv.Shutdown(stop)
	})
	g.Go(func() error {
		if err := srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
}

// Listeners are the sockets Run serves on; a nil one is not served.
type Listeners struct {
	HTTP     net.Listener
	GRPC     net.Listener
	Operator net.Listener
}

// Listen opens the addresses of server.listen, server.grpcListen and server.operatorListen.
func (a *App) Listen() (l Listeners, err error) {
	s := a.Config.Server
	if s.OperatorListen != "" && a.operator == "" {
		return l, fmt.Errorf("server.operatorListen needs auth.operator.tokenRef")
	}
	for _, c := range []struct {
		addr string
		into *net.Listener
	}{{s.Listen, &l.HTTP}, {s.GRPCListen, &l.GRPC}, {s.OperatorListen, &l.Operator}} {
		if *c.into, err = listen(c.addr); err != nil {
			return l, err
		}
	}
	return l, nil
}

// Run serves until ctx ends, then stops listeners within server.shutdownGrace; the worker, the relay and the source sync run beside them.
func (a *App) Run(ctx context.Context, l Listeners) error {
	g, ctx := errgroup.WithContext(ctx)
	if l.HTTP != nil {
		h, err := a.Handler()
		if err != nil {
			return err
		}
		a.serveHTTP(g, ctx, l.HTTP, h)
	}
	if l.Operator != nil {
		h, err := a.operatorHandler()
		if err != nil {
			return err
		}
		a.serveHTTP(g, ctx, l.Operator, h)
	}
	if l.GRPC != nil {
		srv, err := grpcapi.New(a.env, a.personAuthn(), a.log)
		if err != nil {
			return err
		}
		g.Go(func() error {
			<-ctx.Done()
			done := make(chan struct{})
			go func() { srv.GracefulStop(); close(done) }()
			select {
			case <-done:
			case <-time.After(a.Config.Server.ShutdownGrace):
				srv.Stop()
			}
			return nil
		})
		g.Go(func() error { return srv.Serve(l.GRPC) })
	}
	g.Go(func() error { return a.worker.Run(ctx) })
	g.Go(func() error {
		relay.Relay{Outbox: a.outbox, Sink: a.sink, Store: a.Store, Log: a.log, Now: a.Clock.Now}.Run(ctx)
		return nil
	})
	if len(a.sources) > 0 {
		g.Go(func() error { a.syncSources(ctx); return nil })
	}
	return g.Wait()
}
