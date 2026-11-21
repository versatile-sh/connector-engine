package manage

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
	"golang.org/x/term"
)

func loginCommand() *command.Command {
	var user string
	var fromStdin bool
	return leaf("login", "", "Sign in to the console with the username and password of the web console.", func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "the username; asked when empty")
		fs.BoolVar(&fromStdin, "password-stdin", false, "read the password from the first line of stdin instead of asking")
	}, func(ctx context.Context, e *Env) error {
		if err := e.args(0, ""); err != nil {
			return err
		}
		tty, isTTY := e.tty()
		if !fromStdin && !isTTY {
			return command.Usage(e.X.Path, "no terminal to ask the password in: pipe it with --password-stdin")
		}
		if user == "" {
			if !isTTY {
				return command.Usage(e.X.Path, "give --user when stdin is not a terminal")
			}
			fmt.Fprint(e.X.Stderr, "Username: ")
			line, err := e.line()
			if err != nil {
				return err
			}
			user = strings.TrimSpace(line)
		}
		var password string
		if fromStdin {
			line, err := e.line()
			if err != nil {
				return command.Fail(command.ExitUsage, "", "read the password from stdin: %v", err)
			}
			password = line
		} else {
			fmt.Fprint(e.X.Stderr, "Password: ")
			b, err := term.ReadPassword(int(tty.Fd()))
			fmt.Fprintln(e.X.Stderr)
			if err != nil {
				return err
			}
			password = string(b)
		}
		s, err := e.Client.Login(ctx, user, password)
		if err != nil {
			return err
		}
		e.Client.Session = s
		var acct struct {
			Username string `json:"username"`
		}
		if err := e.Client.Do(ctx, http.MethodGet, "/api/account", nil, &acct); err == nil && acct.Username != "" {
			s.User = acct.Username
		}
		where, err := e.Store.Save(ctx, e.Client.Base, s)
		if err != nil {
			return err
		}
		return e.print(map[string]string{"url": e.Client.Base, "user": s.User, "kept_in": where}, func(w io.Writer) {
			fmt.Fprintf(w, "signed in to %s as %s; the session is kept in %s\n", e.Client.Base, s.User, where)
		})
	}, opts{anonymous: true})
}

func logoutCommand() *command.Command {
	return leaf("logout", "", "Sign out of the console and forget the session kept here.", nil, func(ctx context.Context, e *Env) error {
		if err := e.args(0, ""); err != nil {
			return err
		}
		s, err := e.Store.Load(ctx, e.Client.Base)
		if err == nil {
			e.Client.Session = s
			var ce *command.Error
			if err := e.Client.Do(ctx, http.MethodPost, "/api/logout", nil, nil); err != nil && (!errors.As(err, &ce) || ce.Code != command.ExitAuth) {
				return err
			}
		}
		if err := e.Store.Delete(ctx, e.Client.Base); err != nil {
			return err
		}
		return e.print(map[string]string{"url": e.Client.Base}, func(w io.Writer) { fmt.Fprintf(w, "signed out of %s\n", e.Client.Base) })
	}, opts{anonymous: true})
}

func statusCommand() *command.Command {
	var data string
	return leaf("status", "", "Show the install running here, the console and who is signed in.", func(fs *flag.FlagSet) {
		fs.StringVar(&data, "data", "", "the data directory of an install without Docker; empty takes $CN_DATA_DIR or the default one")
	}, func(ctx context.Context, e *Env) error {
		if err := e.args(0, ""); err != nil {
			return err
		}
		out := map[string]any{"url": e.Client.Base}
		if data == "" {
			data, _ = standalone.DataDir(e.X.Getenv)
		}
		abs, _ := filepath.Abs(data)
		local, running := standalone.Running(ctx, abs)
		if running {
			out["install"] = local
			if e.Client.Base == defaultURL {
				e.Client.Base = strings.TrimRight(local.URL, "/")
				out["url"] = e.Client.Base
			}
		}
		var session struct {
			OwnerExists bool `json:"owner_exists"`
		}
		reachErr := e.Client.Do(ctx, http.MethodGet, "/api/session", nil, &session)
		out["reachable"] = reachErr == nil
		if reachErr == nil {
			out["owner_exists"] = session.OwnerExists
			if s, err := e.Store.Load(ctx, e.Client.Base); err == nil {
				e.Client.Session = s
				var acct struct {
					Username string `json:"username"`
				}
				if e.Client.Do(ctx, http.MethodGet, "/api/account", nil, &acct) == nil {
					out["user"] = acct.Username
				}
			}
		}
		if err := e.print(out, func(w io.Writer) {
			if running {
				fmt.Fprintf(w, "install\trunning   pid: %d   version: %s   data: %s\n", local.PID, local.Version, local.Data)
			}
			switch {
			case reachErr != nil:
				fmt.Fprintf(w, "console\t%s is not answering\n", e.Client.Base)
			default:
				fmt.Fprintf(w, "console\t%s   agent endpoint: %s/mcp\n", e.Client.Base, e.Client.Base)
				switch u, _ := out["user"].(string); {
				case !session.OwnerExists:
					fmt.Fprintln(w, "user\tno owner account yet: open the console in a browser to create it")
				case u != "":
					fmt.Fprintf(w, "user\t%s\n", u)
				default:
					fmt.Fprintln(w, "user\tnot signed in (vsh login)")
				}
			}
		}); err != nil {
			return err
		}
		if reachErr != nil {
			return command.Fail(command.ExitUnreachable, "start it with 'vsh up', or set --url or $VSH_URL to the console's address", "cannot reach the console at %s", e.Client.Base)
		}
		return nil
	}, opts{anonymous: true})
}
