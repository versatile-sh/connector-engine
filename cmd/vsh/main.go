// Command vsh is the headless command line of connector-engine: admin over HTTP, tools and skills over MCP.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/cli/command"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := command.Main(ctx, cli.Core(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ())
	stop()
	os.Exit(code)
}
