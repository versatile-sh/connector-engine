// Command vsh is the command line of a person install: its lifecycle without Docker, its management as a signed-in user, the agent-side tools and skills, admin, the keychain and the notification helper.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/manage"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/tui"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	subs := append([]*command.Command{tui.Command()}, manage.Commands()...)
	subs = append(subs, cli.Skills(manage.SkillsList()))
	for _, c := range standalone.Commands(version) {
		if c.Name != "status" {
			subs = append(subs, c)
		}
	}
	subs = append(subs, cli.Tools(), cli.Admin())
	root := cli.Root("vsh runs and manages a connector-engine install. With no command in a terminal it opens the full-screen vsh.", append(subs, selfhosted.Commands()...)...)
	root.Setup = tui.Setup
	code := command.Main(ctx, root, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ())
	stop()
	os.Exit(code)
}
