// Command cnctl is the command line of connector-engine: admin commands over HTTP and agent tools and skills over MCP.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/versatile-sh/connector-engine/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ(), cli.Core())
	stop()
	os.Exit(code)
}
