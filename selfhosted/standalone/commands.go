package standalone

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/atomicfile"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
)

const credentialsWarning = `A full backup holds every credential of this install:
  - the provider sign-ins and API keys of your connectors,
  - the access keys of your agents and the root keys that open everything else,
  - your account and its activity.
Anyone who has the file and its passphrase gets full access. Keep it somewhere safe and never share it.
The passphrase cannot be recovered: without it the file opens for nobody, you included.
`

type command struct {
	fs   *flag.FlagSet
	data *string
	io   prompt
	out  io.Writer
}

func newCommand(name string, args []string, stdin io.Reader, stdout, stderr io.Writer, run func(c *command) error, extra func(fs *flag.FlagSet)) int {
	c := &command{fs: flag.NewFlagSet(name, flag.ContinueOnError), io: newPrompt(stdin, stderr), out: stdout}
	c.fs.SetOutput(stderr)
	def, _ := DataDir(os.Getenv)
	c.data = c.fs.String("data", def, "the data directory of the install")
	if extra != nil {
		extra(c.fs)
	}
	if err := c.fs.Parse(args); err != nil {
		return 2
	}
	abs, err := filepath.Abs(*c.data)
	if err != nil {
		fmt.Fprintln(stderr, name+":", err)
		return 2
	}
	*c.data = abs
	err = run(c)
	var u usage
	switch {
	case errors.As(err, &u):
		fmt.Fprintln(stderr, err)
		return 2
	case errors.Is(err, errNotRunning):
		fmt.Fprintln(stderr, err)
		return 3
	case err != nil:
		fmt.Fprintln(stderr, name+":", err)
		return 1
	}
	return 0
}

type usage string

func (u usage) Error() string { return string(u) }

var errNotRunning = errors.New("no install runs on this data directory")

func (c *command) live(ctx context.Context) error {
	if _, ok := running(ctx, *c.data); !ok {
		return fmt.Errorf("%w (%s): start it with connector-engine-selfhosted up", errNotRunning, *c.data)
	}
	return nil
}

// Commands returns the subcommands of a no-Docker install: up, stop, status, export, import, reset-password and uninstall.
func Commands(version string) map[string]app.Command {
	return map[string]app.Command{
		"up": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			var port int
			var open, noKeychain bool
			return newCommand("up", args, stdin, stdout, stderr, func(c *command) error {
				return up(c, Options{Data: *c.data, Port: port, Version: version, Log: slog.New(slog.NewTextHandler(stderr, nil))}, open, noKeychain)
			}, func(fs *flag.FlagSet) {
				fs.IntVar(&port, "port", 7465, "the console port on 127.0.0.1")
				fs.BoolVar(&open, "open", false, "open the console in the browser")
				fs.BoolVar(&noKeychain, "no-keychain", false, "keep the root keys in plaintext files instead of the OS keychain")
			})
		},
		"stop": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			return newCommand("stop", args, stdin, stdout, stderr, func(c *command) error { return stop(c, true) }, nil)
		},
		"status": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			return newCommand("status", args, stdin, stdout, stderr, status, nil)
		},
		"export": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			var out string
			var setup, yes bool
			return newCommand("export", args, stdin, stdout, stderr, func(c *command) error { return export(c, out, setup, yes) }, func(fs *flag.FlagSet) {
				fs.StringVar(&out, "out", "", "the file to write; empty writes a dated file in $BACKUP_DIR or ~/connector-engine-backups")
				fs.BoolVar(&setup, "setup", false, "export a shareable setup without credentials instead of a full backup")
				fs.BoolVar(&yes, "yes", false, "skip the confirmation")
			})
		},
		"import": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			var yes bool
			return newCommand("import", args, stdin, stdout, stderr, func(c *command) error { return importFile(c, yes) }, func(fs *flag.FlagSet) {
				fs.BoolVar(&yes, "yes", false, "skip the confirmation")
			})
		},
		"reset-password": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			var user string
			return newCommand("reset-password", args, stdin, stdout, stderr, func(c *command) error { return resetPassword(c, user) }, func(fs *flag.FlagSet) {
				fs.StringVar(&user, "user", "", "the username whose password to set; may be left out while the console has one user")
			})
		},
		"uninstall": func(_ config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			var keep, purge, yes, noKeychain bool
			return newCommand("uninstall", args, stdin, stdout, stderr, func(c *command) error { return uninstall(c, keep, purge, yes, noKeychain) }, func(fs *flag.FlagSet) {
				fs.BoolVar(&keep, "keep-data", false, "stop the install and keep its data directory and unlock key")
				fs.BoolVar(&purge, "purge", false, "stop the install and delete its data directory and its unlock key in the keychain")
				fs.BoolVar(&yes, "yes", false, "skip the confirmation of --purge")
				fs.BoolVar(&noKeychain, "no-keychain", false, "leave the OS keychain alone, for an install started with --no-keychain")
			})
		},
	}
}

func up(c *command, o Options, open, noKeychain bool) error {
	if !noKeychain {
		o.Keychain = keychain.System()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	in, err := Start(ctx, o)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "console: %s   agent endpoint: %s/mcp   data: %s\n", in.URL, in.URL, o.Data)
	if open {
		if err := openBrowser(in.URL); err != nil {
			o.Log.Warn("cannot open the browser", "error", err)
		}
	}
	return in.Wait()
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

func stop(c *command, report bool) error {
	ctx := context.Background()
	if _, ok := running(ctx, *c.data); !ok {
		if report {
			fmt.Fprintln(c.out, "not running")
		}
		return nil
	}
	_ = request(ctx, *c.data, http.MethodPost, "/stop", nil, io.Discard)
	for range 900 {
		if _, ok := running(ctx, *c.data); !ok {
			fmt.Fprintln(c.out, "stopped")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the install did not stop within 90s")
}

func status(c *command) error {
	s, ok := running(context.Background(), *c.data)
	if !ok {
		return c.live(context.Background())
	}
	fmt.Fprintf(c.out, "running   pid: %d   version: %s   console: %s   agent endpoint: %s/mcp   data: %s\n", s.PID, s.Version, s.URL, s.URL, s.Data)
	return nil
}

func export(c *command, out string, setup, yes bool) error {
	ctx := context.Background()
	if err := c.live(ctx); err != nil {
		return err
	}
	path, kind, body := "/export?mode=setup", "setup", io.Reader(nil)
	if !setup {
		c.io.say(credentialsWarning)
		if err := c.io.confirm("Export a full backup with every credential?", yes); err != nil {
			return err
		}
		passphrase, err := c.io.twice("Backup passphrase (at least 12 characters)")
		if err != nil {
			return err
		}
		path, kind, body = "/export", "backup", bytes.NewReader(passphrase)
	}
	if out == "" {
		dir := os.Getenv("BACKUP_DIR")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir = filepath.Join(home, "connector-engine-backups")
		}
		ext := map[string]string{"setup": ".setup.yaml", "backup": ".cebak"}[kind]
		out = filepath.Join(dir, "connector-engine-"+kind+"-"+time.Now().UTC().Format("20060102T150405Z")+ext)
	}
	out = filepath.Clean(out)
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := request(ctx, *c.data, http.MethodPost, path, body, &buf); err != nil {
		return err
	}
	if err := atomicfile.Write(out, buf.Bytes()); err != nil {
		return err
	}
	fmt.Fprintln(c.out, "wrote", out)
	return nil
}

func importFile(c *command, yes bool) error {
	ctx := context.Background()
	if c.fs.NArg() != 1 {
		return usage("usage: connector-engine-selfhosted import [--data DIR] [--yes] FILE")
	}
	raw, err := os.ReadFile(c.fs.Arg(0))
	if err != nil {
		return err
	}
	if err := c.live(ctx); err != nil {
		return err
	}
	var passphrase []byte
	full := bytes.HasPrefix(raw, []byte("CEBAK\n"))
	if full {
		if passphrase, err = c.io.secret("Passphrase of " + filepath.Base(c.fs.Arg(0))); err != nil {
			return err
		}
	}
	body := func() io.Reader {
		return io.MultiReader(bytes.NewReader(passphrase), bytes.NewReader([]byte("\n")), bytes.NewReader(raw))
	}
	if err := request(ctx, *c.data, http.MethodPost, "/inspect", body(), c.out); err != nil {
		return err
	}
	fmt.Fprintln(c.out)
	question := "Add what this setup holds? Nothing already here changes."
	if full {
		question = "Replace everything on this install with this backup?"
	}
	if err := c.io.confirm(question, yes); err != nil {
		return err
	}
	if err := request(ctx, *c.data, http.MethodPost, "/import", body(), c.out); err != nil {
		return err
	}
	fmt.Fprintln(c.out)
	if full {
		fmt.Fprintln(c.out, "restored: the engine and the console restart on the restored data")
	} else {
		fmt.Fprintln(c.out, "sign in to each added connector from the console")
	}
	return nil
}

func resetPassword(c *command, user string) error {
	ctx := context.Background()
	if err := c.live(ctx); err != nil {
		return err
	}
	password, err := c.io.twice("New console password (at least 10 characters)")
	if err != nil {
		return err
	}
	body := password
	if user != "" {
		body = append([]byte(user+"\n"), password...)
	}
	if err := request(ctx, *c.data, http.MethodPost, "/password", bytes.NewReader(body), io.Discard); err != nil {
		return err
	}
	fmt.Fprintln(c.out, "password changed; every console session of that user is signed out")
	fmt.Fprintln(c.out, "access keys and apps signed in through the browser still work: revoke them on the Access page if you suspect a compromise")
	return nil
}

func uninstall(c *command, keep, purge, yes, noKeychain bool) error {
	if keep == purge {
		return usage("usage: connector-engine-selfhosted uninstall [--data DIR] --keep-data | --purge [--yes]")
	}
	if purge {
		if err := holdsInstall(*c.data); err != nil {
			return err
		}
		c.io.say("This deletes " + *c.data + " and the unlock key of this install: every connector, credential, access key and the account. Export a backup first if you may need any of it.\n")
		if err := c.io.confirm("Delete everything of this install?", yes); err != nil {
			return err
		}
	}
	if err := stop(c, false); err != nil {
		return err
	}
	if purge {
		if err := os.RemoveAll(*c.data); err != nil {
			return err
		}
		fmt.Fprintln(c.out, "deleted", *c.data)
		if kc := keychain.System(); kc != nil && !noKeychain {
			if err := kc.Delete(context.Background(), layout(*c.data).account); err != nil {
				c.io.say("the unlock key stays in the keychain (service " + keychain.Service + ", account " + layout(*c.data).account + "): " + err.Error() + "\n")
			}
		}
	} else {
		fmt.Fprintln(c.out, "stopped; the data stays in", *c.data)
	}
	if exe, err := os.Executable(); err == nil {
		fmt.Fprintln(c.out, "remove the binary to finish:", exe)
	}
	return nil
}
