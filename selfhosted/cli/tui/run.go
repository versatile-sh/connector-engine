package tui

import (
	"context"
	"errors"
	"flag"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/manage"
)

// Run shows the TUI over api on the terminal until the user quits.
func Run(ctx context.Context, api API) error {
	_, err := tea.NewProgram(New(ctx, api), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	return err
}

// Terminal reports whether x reads from and writes to a terminal.
func Terminal(x *command.Exec) bool {
	in, ok := x.Stdin.(*os.File)
	out, ok2 := x.Stdout.(*os.File)
	return ok && ok2 && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

// Setup declares the flags of the TUI and returns its runner, for vsh ui and a bare vsh.
func Setup(fs *flag.FlagSet) command.Runner {
	base := fs.String("url", "", "the console address; empty takes $VSH_URL, the install running here, or http://127.0.0.1:7465")
	return func(ctx context.Context, x *command.Exec) error {
		if len(x.Args) > 0 {
			return command.Usage("vsh", "unknown command %q", x.Args[0])
		}
		if !Terminal(x) {
			return command.Usage("vsh", "the full-screen vsh needs a terminal; give a command instead")
		}
		c, err := manage.Open(ctx, *base, x.Getenv)
		if err != nil {
			return err
		}
		return Run(ctx, c)
	}
}

// Command is the vsh ui leaf.
func Command() *command.Command {
	return &command.Command{Name: "ui", Summary: "Open the full-screen vsh: connectors, keys, approvals and activity.", Setup: Setup}
}
