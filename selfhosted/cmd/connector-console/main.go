// Command connector-console serves the local web console of a connector-engine person install.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
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
	consolePort := or(getenv, "CONSOLE_PORT", "7465")
	listen := fs.String("listen", or(getenv, "CONSOLE_LISTEN", "127.0.0.1:"+consolePort), "address the console listens on")
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
	token, err := os.ReadFile(*tokenFile)
	if err != nil || strings.TrimSpace(string(token)) == "" {
		fmt.Fprintf(stderr, "operator token: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	unlock := func(context.Context) (port.SecretStore, error) { return secretslocal.Open(*secretsDir) }
	if *unlockSocket != "" {
		unlock = func(ctx context.Context) (port.SecretStore, error) {
			return hostkey.Open(ctx, *secretsDir, *unlockSocket, log)
		}
	}
	c, closeConsole, err := console.Open(ctx, console.Options{PublicURL: *publicURL, EngineURL: *engineURL, OperatorURL: *operatorURL, Operator: strings.TrimSpace(string(token)),
		StatePath: *statePath, Unlock: unlock, Tenant: *tenant, Workspace: *workspace, Person: *person, LoopbackHost: *loopbackHost, AgentTTL: *agentTTL, Restart: stop, Log: log})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeConsole()
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
	log.Info("console listening", "listen", lis.Addr().String(), "url", *publicURL)
	if err := srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func host(args []string, socket string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 2 || socket == "" {
		fmt.Fprintln(stderr, "usage: connector-console host METHOD PATH, with CONSOLE_HOST_SOCKET set; the body is read from stdin")
		return 2
	}
	if err := console.HostRequest(context.Background(), socket, args[0], args[1], stdin, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
