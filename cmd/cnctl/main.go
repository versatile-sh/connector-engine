// Command cnctl is the command line of connector-engine: the admin commands over HTTP.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"

	"github.com/agents-control/connector-engine/cli/admin"
)

type command func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int

var commands = map[string]command{
	"admin": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
		return admin.Command(stdout, stderr, environ).Run(ctx, args)
	},
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
