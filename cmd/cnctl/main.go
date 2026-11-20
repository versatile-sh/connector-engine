// Command cnctl is the command line of connector-engine: admin commands over HTTP, agent tools over MCP, the unlock key in the host's keychain and the host notification helper of the local console.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/versatile-sh/connector-engine/cli/admin"
	"github.com/versatile-sh/connector-engine/cli/keychain"
	"github.com/versatile-sh/connector-engine/cli/notify"
	"github.com/versatile-sh/connector-engine/cli/skills"
	"github.com/versatile-sh/connector-engine/cli/tools"
)

type command func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int

var commands = map[string]command{
	"admin": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
		return admin.Command(stdout, stderr, environ).Run(ctx, args)
	},
	"skills": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
		if err := skills.New(getenv(environ)).Run(ctx, args, stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "cnctl:", err)
			return 1
		}
		return 0
	},
	"keychain": func(ctx context.Context, args []string, stdout, stderr io.Writer, _ []string) int {
		return keychain.Run(ctx, keychain.System(), args, os.Stdin, stdout, stderr)
	},
	"notify": func(ctx context.Context, args []string, _, stderr io.Writer, environ []string) int {
		return notify.Command(ctx, args, getenv(environ), stderr)
	},
	"tools": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
		if err := tools.New(getenv(environ)).Run(ctx, args, stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "cnctl:", err)
			return 1
		}
		return 0
	},
}

func getenv(environ []string) func(string) string {
	return func(key string) string {
		for _, kv := range environ {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ())
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
	if len(args) > 0 {
		if c, ok := commands[args[0]]; ok {
			return c(ctx, args[1:], stdout, stderr, environ)
		}
	}
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(stderr, "usage: cnctl <command> [arguments]\ncommands: %v\n", names)
	return 2
}
