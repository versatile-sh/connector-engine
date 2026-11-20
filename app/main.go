package app

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

	"github.com/versatile-sh/connector-engine/config"
)

// Command is a subcommand of the engine binary; it runs once connector-engine.yaml has loaded.
type Command func(cfg config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int

// Build is what one engine binary composes: its version, its adapters (nil is Core) and the subcommands it adds.
type Build struct {
	Version  string
	Adapters *Registry
	Commands map[string]Command
}

// Main runs the engine binary: it serves, checks or plans an adapters file with "adapters", or runs a subcommand of b.
func Main(b Build, args, environ []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if b.Adapters == nil {
		b.Adapters = Core()
	}
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
	cfg = cfg.With(b.Adapters.Builtins()...)
	if rest := fs.Args(); len(rest) > 0 {
		if rest[0] == "adapters" {
			return cfg.AdaptersMain(append(rest[1:], "--file", *adaptersPath), environ, stdout, stderr)
		}
		if c, ok := b.Commands[rest[0]]; ok {
			return c(cfg, rest[1:], stdin, stdout, stderr)
		}
	}
	return serve(b, cfg, *adaptersPath, environ, stdout, stderr)
}

func serve(b Build, cfg config.Config, adaptersPath string, environ []string, stdout, stderr io.Writer) int {
	file, err := cfg.ReadAdapters(adaptersPath, environ)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	a, err := New(ctx, cfg, file, Options{Lookup: func(name string) string { return env(environ, name) }, Log: log, Build: b.Version, Adapters: b.Adapters})
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
	fmt.Fprintf(stdout, "connector-engine %s mode=%s http=%s grpc=%s operator=%s\n", b.Version, cfg.Mode, addr(l.HTTP), addr(l.GRPC), addr(l.Operator))
	if err := a.Run(ctx, l); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

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
