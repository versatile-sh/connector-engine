// Command cnctl is the connector-engine command line; each group of subcommands registers in the root.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/agents-control/connector-engine/cli/tools"
)

type group interface {
	Name() string
	Summary() string
	Run(ctx context.Context, args []string, stdout, stderr io.Writer) error
}

func groups() []group { return []group{tools.New(nil)} }

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	all := groups()
	if len(args) > 0 {
		for _, g := range all {
			if g.Name() == args[0] {
				if err := g.Run(ctx, args[1:], stdout, stderr); err != nil {
					fmt.Fprintln(stderr, "cnctl:", err)
					return 1
				}
				return 0
			}
		}
	}
	fmt.Fprintln(stderr, "usage: cnctl GROUP [ARGS]")
	for _, g := range all {
		fmt.Fprintf(stderr, "  %-8s %s\n", g.Name(), g.Summary())
	}
	return 2
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
