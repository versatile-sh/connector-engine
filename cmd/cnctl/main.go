// Command cnctl is the command line of connector-engine: admin commands over HTTP, agent tools over MCP, the unlock key in the host's keychain and the host notification helper of the local console.
package main

import (
	"context"
	"maps"
	"os"
	"os/signal"

	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/selfhosted"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	commands := cli.Core()
	maps.Copy(commands, selfhosted.Commands())
	code := cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ(), commands)
	stop()
	os.Exit(code)
}
