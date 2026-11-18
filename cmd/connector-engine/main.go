// Command connector-engine boots the engine from connector-engine.yaml and adapters.yaml, or checks an adapters file.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/agents-control/connector-engine/app"
	"github.com/agents-control/connector-engine/config"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Environ(), os.Stdout, os.Stderr)) }

func env(environ []string, name string) string {
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v
		}
	}
	return ""
}

func addr(l net.Listener) string {
	if l == nil {
		return "off"
	}
	return l.Addr().String()
}

func run(args, environ []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("connector-engine", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", env(environ, "CN_CONFIG"), "connector-engine.yaml; empty takes the defaults")
	adaptersPath := fs.String("adapters", env(environ, "CN_ADAPTERS_FILE"), "adapters.yaml; empty takes the defaults and the CN_* variables")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if rest := fs.Args(); len(rest) > 0 && rest[0] == "adapters" {
		return cfg.AdaptersMain(append(rest[1:], "--file", *adaptersPath), environ, stdout, stderr)
	}
	file, err := cfg.ReadAdapters(*adaptersPath, environ)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	a, err := app.New(ctx, cfg, file, app.Options{Lookup: func(name string) string { return env(environ, name) }, Log: log, Build: version})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer a.Close()
	l, err := a.Listen()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "connector-engine %s mode=%s http=%s grpc=%s operator=%s\n", version, cfg.Mode, addr(l.HTTP), addr(l.GRPC), addr(l.Operator))
	if err := a.Run(ctx, l); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
