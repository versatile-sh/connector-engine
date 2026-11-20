// Command connector-console serves the local web console of a connector-engine person install.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
	"github.com/versatile-sh/connector-engine/selfhosted/console/web"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "unlock" {
		return hostkey.Main(args[1:], getenv("CONSOLE_UNLOCK_SOCKET"), os.Stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "host" {
		return host(args[1:], getenv("CONSOLE_HOST_SOCKET"), os.Stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	return serve(args, getenv, stderr)
}

func or(getenv func(string) string, name, def string) string {
	if v := getenv(name); v != "" {
		return v
	}
	return def
}

func serve(args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("connector-console", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := or(getenv, "CONSOLE_PORT", "7465")
	listen := fs.String("listen", or(getenv, "CONSOLE_LISTEN", "127.0.0.1:"+port), "address the console listens on")
	publicURL := fs.String("public-url", getenv("CONSOLE_PUBLIC_URL"), "address the browser and agents use; empty takes http://127.0.0.1 at the port of --listen")
	engineURL := fs.String("engine", or(getenv, "CONSOLE_ENGINE_URL", "http://127.0.0.1:8080"), "the engine's public listener")
	operatorURL := fs.String("engine-operator", or(getenv, "CONSOLE_ENGINE_OPERATOR_URL", "http://127.0.0.1:8081"), "the engine's operator listener")
	tokenFile := fs.String("operator-token-file", or(getenv, "CONSOLE_OPERATOR_TOKEN_FILE", "/run/connector-engine/operator-token"), "file holding the operator token")
	statePath := fs.String("state", or(getenv, "CONSOLE_STATE", "/var/lib/connector-console/state.json"), "the console's state file")
	secretsDir := fs.String("secrets", or(getenv, "CONSOLE_SECRETS", "/var/lib/connector-console/secrets"), "key store that seals the console's own secrets")
	unlockSocket := fs.String("unlock-socket", getenv("CONSOLE_UNLOCK_SOCKET"), "socket the host delivers the unlock key on; empty keeps a plaintext root key in --secrets")
	hostSocket := fs.String("host-socket", getenv("CONSOLE_HOST_SOCKET"), "socket for the requests only the local machine makes, such as a password reset; empty serves none")
	tenant := fs.String("tenant", or(getenv, "CONSOLE_TENANT", "local"), "tenant of the local install")
	workspace := fs.String("workspace", or(getenv, "CONSOLE_WORKSPACE", "default"), "workspace connectors are installed in")
	person := fs.String("person", or(getenv, "CONSOLE_PERSON", "owner"), "engine person the console acts as; a tenant admin in bootstrap.roles")
	loopbackHost := fs.String("loopback-host", getenv("CONSOLE_LOOPBACK_HOST"), "host a local server at localhost is dialled as, such as host.docker.internal in a container; empty dials loopback itself")
	agentTTL := fs.Duration("agent-ttl", 720*time.Hour, "longest agent token; at most auth.agentTokenMaxLifetime")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	if *publicURL == "" {
		_, p, err := net.SplitHostPort(*listen)
		if err != nil {
			fmt.Fprintln(stderr, "--listen is host:port")
			return 2
		}
		*publicURL = "http://127.0.0.1:" + p
	}
	public, err := url.Parse(strings.TrimRight(*publicURL, "/"))
	if err != nil || public.Host == "" || public.Port() == "" {
		fmt.Fprintln(stderr, "--public-url is an absolute URL with a port")
		return 2
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil || strings.TrimSpace(string(token)) == "" {
		fmt.Fprintf(stderr, "operator token: %v\n", err)
		return 1
	}
	state, err := console.OpenState(*statePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	activity, err := console.OpenActivity(filepath.Join(filepath.Dir(*statePath), "activity"), log)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer activity.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	box := &sealed.Box{}
	if *unlockSocket == "" {
		store, err := secretslocal.Open(*secretsDir)
		if err != nil {
			fmt.Fprintf(stderr, "secrets: %v\n", err)
			return 1
		}
		box.Unlock(store)
	} else {
		go unlockBox(ctx, box, *secretsDir, *unlockSocket, log)
	}
	eng := &console.Engine{PublicURL: *engineURL, OperatorURL: *operatorURL, Operator: strings.TrimSpace(string(token)), Tenant: *tenant, Workspace: *workspace, Person: *person}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: console.LoopbackHosts(public)}, Web: web.App(), Secrets: box,
		MCPURL: public.String() + "/mcp", OAuthRedirect: public.String() + "/oauth/callback", MaxAgentTTL: *agentTTL, Log: log, Activity: activity, Restart: stop, LoopbackHost: *loopbackHost}
	go ensureTenant(ctx, eng, log)
	go c.RunNotifications(ctx)
	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	srv := &http.Server{Handler: c.Handler(), ReadHeaderTimeout: 10 * time.Second}
	servers := []*http.Server{srv}
	if *hostSocket != "" {
		hl, err := hostkey.Listen(*hostSocket)
		if err != nil {
			fmt.Fprintf(stderr, "host socket: %v\n", err)
			return 1
		}
		hs := &http.Server{Handler: c.HostHandler(), ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, hs)
		go func() { _ = hs.Serve(hl) }()
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, s := range servers {
			_ = s.Shutdown(shut)
		}
	}()
	log.Info("console listening", "listen", lis.Addr().String(), "url", public.String())
	if err := srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func unlockBox(ctx context.Context, box *sealed.Box, dir, socket string, log *slog.Logger) {
	store, err := hostkey.Open(ctx, dir, socket, log)
	if err != nil {
		log.Error("the console's secrets stay locked", "err", err)
		return
	}
	box.Unlock(store)
	log.Info("the console's secrets are unlocked", "protection", store.Form())
}

func ensureTenant(ctx context.Context, eng *console.Engine, log *slog.Logger) {
	for {
		err := eng.EnsureTenant(ctx)
		if err == nil {
			log.Info("tenant ready", "tenant", eng.Tenant)
			return
		}
		log.Warn("waiting for the engine", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func host(args []string, socket string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 2 || socket == "" {
		fmt.Fprintln(stderr, "usage: connector-console host METHOD PATH, with CONSOLE_HOST_SOCKET set; the body is read from stdin")
		return 2
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	req, err := http.NewRequest(args[0], "http://console"+args[1], stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if json.Unmarshal(raw, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		fmt.Fprintln(stderr, e.Error)
		return 1
	}
	if _, err := io.Copy(stdout, resp.Body); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
