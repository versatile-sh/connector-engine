// Command connector-console serves the local web console of a connector-engine person install, or writes its first-boot secrets.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/agents-control/connector-engine/console"
	"github.com/agents-control/connector-engine/console/web"
)

func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "init" {
		return initSecrets(args[1:], stdout, stderr)
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

func secretHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeOnce(path, value string, mode os.FileMode) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(value); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

func initSecrets(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "/run/connector-engine", "directory of the secrets volume")
	dbHost := fs.String("db-host", "postgres:5432", "host:port of the Postgres service")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	appPassword := secretHex(24)
	files := []struct {
		name, value string
		mode        os.FileMode
	}{
		{"operator-token", secretHex(32), 0o600},
		{"pg-admin-password", secretHex(24), 0o644},
		{"pg-app-password", appPassword, 0o644},
		{"database-url", "", 0o600},
	}
	for _, f := range files {
		value := f.value
		if f.name == "database-url" {
			raw, err := os.ReadFile(filepath.Join(*dir, "pg-app-password"))
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			value = (&url.URL{Scheme: "postgres", User: url.UserPassword("cn", strings.TrimSpace(string(raw))), Host: *dbHost, Path: "/connector_engine", RawQuery: "sslmode=disable"}).String()
		}
		made, err := writeOnce(filepath.Join(*dir, f.name), value, f.mode)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if made {
			fmt.Fprintf(stdout, "wrote %s\n", f.name)
		}
	}
	return 0
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
	tenant := fs.String("tenant", or(getenv, "CONSOLE_TENANT", "local"), "tenant of the local install")
	workspace := fs.String("workspace", or(getenv, "CONSOLE_WORKSPACE", "default"), "workspace connectors are installed in")
	person := fs.String("person", or(getenv, "CONSOLE_PERSON", "owner"), "engine person the console acts as; a tenant admin in bootstrap.roles")
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
	eng := &console.Engine{PublicURL: *engineURL, OperatorURL: *operatorURL, Operator: strings.TrimSpace(string(token)), Tenant: *tenant, Workspace: *workspace, Person: *person}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: console.LoopbackHosts(public)}, Web: web.App(),
		MCPURL: public.String() + "/mcp", OAuthRedirect: public.String() + "/oauth/callback", MaxAgentTTL: *agentTTL, Log: log}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go ensureTenant(ctx, eng, log)
	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	srv := &http.Server{Handler: c.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Info("console listening", "listen", lis.Addr().String(), "url", public.String())
	if err := srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
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
