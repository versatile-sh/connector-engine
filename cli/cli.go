// Package cli is the command tree of vsh that every build has: admin over HTTP, tools and skills over MCP.
package cli

import (
	"context"

	"github.com/versatile-sh/connector-engine/cli/admin"
	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/cli/skills"
	"github.com/versatile-sh/connector-engine/cli/tools"
)

// Admin is the vsh admin group, the engine's management API over HTTP.
func Admin() *command.Command {
	return &command.Command{Name: "admin", Summary: "Call the engine's management API over HTTP (operators).",
		Raw: func(ctx context.Context, x *command.Exec) error {
			if code := admin.Command(x.Stdout, x.Stderr, x.Environ).Run(ctx, x.Args); code != 0 {
				return command.Silent(code)
			}
			return nil
		}}
}

// Tools is the vsh tools group, the agent-side list, search and call.
func Tools() *command.Command {
	return &command.Command{Name: "tools", Summary: "List, search and call functions as an agent.", Subs: tools.Commands()}
}

// Skills is the vsh skills group with export, the agent-side commands; a build may add more leaves.
func Skills(more ...*command.Command) *command.Command {
	return &command.Command{Name: "skills", Summary: "Work with skills.", Subs: append(more, skills.ExportCommand())}
}

// Root returns the vsh tree with subs under it.
func Root(summary string, subs ...*command.Command) *command.Command {
	return &command.Command{Name: "vsh", Summary: summary, Subs: subs}
}

// Core returns the tree of the headless vsh: admin, tools and skills.
func Core() *command.Command {
	return Root("vsh is the command line of connector-engine.", Admin(), Skills(), Tools())
}
