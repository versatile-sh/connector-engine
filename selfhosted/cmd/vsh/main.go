// Command vsh is the command line of a person install: the install's lifecycle without Docker, the agent-side tools and skills, admin, the keychain and the notification helper.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	subs := append(standalone.Commands(version), cli.Admin(), cli.Skills(), cli.Tools())
	root := cli.Root("vsh runs and manages a connector-engine install.", append(subs, selfhosted.Commands()...)...)
	code := command.Main(ctx, root, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ())
	stop()
	os.Exit(code)
}
