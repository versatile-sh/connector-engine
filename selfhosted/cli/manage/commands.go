package manage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/versatile-sh/connector-engine/cli/command"
	"golang.org/x/term"
)

// Env is one run of a management command: the client signed in to the console, the output mode and the run's streams.
type Env struct {
	X      *command.Exec
	Client *Client
	Store  Store
	JSON   bool
	in     *bufio.Reader
}

type opts struct {
	anonymous bool
	noJSON    bool
}

func leaf(name, args, summary string, extra func(fs *flag.FlagSet), run func(ctx context.Context, e *Env) error, o opts) *command.Command {
	return &command.Command{Name: name, Args: args, Summary: summary, Setup: func(fs *flag.FlagSet) command.Runner {
		base := fs.String("url", "", "the console address; empty takes $VSH_URL, the install running here, or "+defaultURL)
		asJSON := new(bool)
		if !o.noJSON {
			fs.BoolVar(asJSON, "json", false, "print JSON")
		}
		if extra != nil {
			extra(fs)
		}
		return func(ctx context.Context, x *command.Exec) error {
			e := &Env{X: x, Client: &Client{Base: Locate(ctx, *base, x.Getenv)}, Store: NewStore(x.Getenv), JSON: *asJSON}
			if !o.anonymous {
				c, err := Open(ctx, *base, x.Getenv)
				if err != nil {
					return err
				}
				e.Client = c
			}
			return run(ctx, e)
		}
	}}
}

// Open returns a client signed in to the console that Locate finds, or an error that says to run vsh login.
func Open(ctx context.Context, base string, getenv func(string) string) (*Client, error) {
	c := &Client{Base: Locate(ctx, base, getenv)}
	s, err := NewStore(getenv).Load(ctx, c.Base)
	if errors.Is(err, errNoSession) {
		return nil, command.Fail(command.ExitAuth, "run 'vsh login'", "not signed in to %s", c.Base)
	}
	if err != nil {
		return nil, err
	}
	c.Session = s
	return c, nil
}

func (e *Env) print(v any, human func(w io.Writer)) error {
	if e.JSON {
		enc := json.NewEncoder(e.X.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	tw := tabwriter.NewWriter(e.X.Stdout, 0, 0, 2, ' ', 0)
	human(tw)
	return tw.Flush()
}

func (e *Env) args(n int, what string) error {
	if len(e.X.Args) != n {
		if n == 0 {
			return command.Usage(e.X.Path, "%s takes no arguments", e.X.Path)
		}
		return command.Usage(e.X.Path, "give %s", what)
	}
	return nil
}

func (e *Env) tty() (*os.File, bool) {
	f, ok := e.X.Stdin.(*os.File)
	return f, ok && term.IsTerminal(int(f.Fd()))
}

func (e *Env) line() (string, error) {
	if e.in == nil {
		e.in = bufio.NewReader(e.X.Stdin)
	}
	s, err := e.in.ReadString('\n')
	if err == io.EOF && s != "" {
		err = nil
	}
	return strings.TrimRight(s, "\r\n"), err
}

func (e *Env) confirm(question string, yes bool) error {
	if yes {
		return nil
	}
	if _, ok := e.tty(); !ok {
		return command.Usage(e.X.Path, "this asks for a confirmation: run it in a terminal or add --yes")
	}
	fmt.Fprint(e.X.Stderr, question+" [y/N] ")
	answer, err := e.line()
	if err != nil {
		return err
	}
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
		return command.Fail(command.ExitFailure, "", "cancelled")
	}
	return nil
}

// Commands returns the management leaves of vsh: status, login, logout, connectors, keys, access, approvals and activity.
func Commands() []*command.Command {
	return []*command.Command{statusCommand(), loginCommand(), logoutCommand(), connectorsCommand(), keysCommand(), accessCommand(), approvalsCommand(), activityCommand()}
}

// SkillsList is the vsh skills list leaf: the installed skills and each key's access.
func SkillsList() *command.Command { return skillsList() }
