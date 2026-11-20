package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/relay"
	"github.com/versatile-sh/connector-engine/server"
	"github.com/versatile-sh/connector-engine/server/grpcapi"
	"github.com/versatile-sh/connector-engine/server/httpapi"
	"github.com/versatile-sh/connector-engine/server/mcpapi"
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
		meta := auth.ProtectedResourceMetadataHandler(a.resourceMetadata())
		mux.Handle("GET /.well-known/oauth-protected-resource", meta)
		mux.Handle("GET /.well-known/oauth-protected-resource/mcp", meta)
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

func (a *App) resourceMetadata() *oauthex.ProtectedResourceMetadata {
	m := &oauthex.ProtectedResourceMetadata{Resource: a.Config.Audience(), BearerMethodsSupported: []string{"header"}, ResourceName: "connector-engine"}
	for _, i := range a.Config.Auth.Issuers {
		m.AuthorizationServers = append(m.AuthorizationServers, i.Name)
	}
	return m
}

func (a *App) operatorHandler() (http.Handler, error) {
	mux := http.NewServeMux()
	api, err := a.api(a.operatorAuthn())
	if err != nil {
		return nil, err
	}
	mux.Handle("/v1/", api)
	a.snapshotRoutes(mux)
	mux.Handle("GET /metrics", a.metrics.Handler())
	a.probes(mux)
	return mux, nil
}

type tokenRequest struct {
	Subject    string           `json:"sub"`
	Audience   []string         `json:"aud"`
	Tenant     port.TenantID    `json:"tenant"`
	Workspace  port.WorkspaceID `json:"workspace"`
	Agent      port.AgentID     `json:"agent"`
	AgentOwner port.PersonID    `json:"agent_owner"`
	Session    port.SessionID   `json:"session"`
	OnBehalfOf port.PersonID    `json:"on_behalf_of"`
	Caller     port.Caller      `json:"caller"`
	TTL        string           `json:"ttl"`
}

func (a *App) tokenClaims(r *http.Request) (port.Claims, error) {
	bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if _, err := a.operatorAuthn()(r.Context(), bearer); err != nil || a.operator == "" {
		return port.Claims{}, port.Errorf(port.CodeUnauthenticated, "not the operator token")
	}
	var req tokenRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return port.Claims{}, port.Errorf(port.CodeInvalidArgument, "the body names only sub, aud, tenant, workspace, agent, agent_owner, session, on_behalf_of, caller and ttl: %v", err)
	}
	ttl, err := time.ParseDuration(cmp.Or(req.TTL, "1h"))
	if err != nil || ttl <= 0 || ttl > max(time.Hour, a.Config.Auth.AgentTokenMaxLifetime) {
		return port.Claims{}, port.Errorf(port.CodeInvalidArgument, "ttl %q is not a duration up to auth.agentTokenMaxLifetime", req.TTL)
	}
	c := port.Claims{Subject: req.Subject, Audience: req.Audience, Tenant: req.Tenant, Workspace: req.Workspace, Agent: req.Agent, AgentOwner: req.AgentOwner,
		Session: req.Session, OnBehalfOf: req.OnBehalfOf, Caller: req.Caller, ExpiresAt: a.Clock.Now().Add(ttl)}
	switch {
	case len(c.Audience) == 0 && c.Agent != "":
		c.Audience = []string{a.Config.Audience()}
	case len(c.Audience) == 0:
		c.Audience = []string{a.Config.ManageAudience()}
	}
	for _, aud := range c.Audience {
		if aud != a.Config.Audience() && aud != a.Config.ManageAudience() {
			return port.Claims{}, port.Errorf(port.CodeInvalidArgument, "audience %q is not one of this engine's", aud)
		}
	}
	return c, nil
}

func (a *App) localToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	c, err := a.tokenClaims(r)
	if port.IsCode(err, port.CodeUnauthenticated) {
		http.Error(w, "the local issuer takes the operator token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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

// Run serves until ctx ends or a snapshot is restored, then stops listeners within server.shutdownGrace; the worker, the relay and the source sync run beside them.
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
		g.Go(func() error {
			if err := srv.Serve(l.GRPC); !errors.Is(err, grpc.ErrServerStopped) {
				return err
			}
			return nil
		})
	}
	g.Go(func() error { return a.worker.Run(ctx) })
	g.Go(func() error {
		relay.Relay{Outbox: a.outbox, Sink: a.sink, Store: a.Store, Log: a.log, Now: a.Clock.Now}.Run(ctx)
		return nil
	})
	if len(a.sources) > 0 {
		g.Go(func() error { a.syncSources(ctx); return nil })
	}
	g.Go(func() error {
		select {
		case <-a.restored:
			return errRestored
		case <-ctx.Done():
			return nil
		}
	})
	if err := g.Wait(); !errors.Is(err, errRestored) {
		return err
	}
	return nil
}
