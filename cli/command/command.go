// Package command is the command tree of vsh: named groups and leaves with one help format, interspersed flags and documented exit codes.
package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Exit codes of every vsh command.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnreachable = 3
	ExitAuth        = 4
)

// Command is one vsh command: a group of Subs, a leaf whose Setup declares its flags and returns its runner, or both.
type Command struct {
	Name    string
	Summary string
	Args    string
	Long    string
	Default string
	Subs    []*Command
	Setup   func(fs *flag.FlagSet) Runner
	Raw     func(ctx context.Context, x *Exec) error
}

// Runner runs a parsed leaf.
type Runner func(ctx context.Context, x *Exec) error

// Exec is one run of a command: its path, positional arguments, streams and environment.
type Exec struct {
	Path    string
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Environ []string
}

// Getenv reads a variable from the run's environment.
func (x *Exec) Getenv(key string) string { return Getenv(x.Environ)(key) }

// Error is a failure with its exit code and a suggested next step.
type Error struct {
	Code int
	Err  error
	Next string
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

// Silent returns an error that exits with code and prints nothing, for a command that reported its failure itself.
func Silent(code int) error { return &Error{Code: code} }

func (e *Error) Unwrap() error { return e.Err }

// Fail returns an error with code and the next step to suggest.
func Fail(code int, next string, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...), Next: next}
}

// Usage returns a usage error that points at the help of path.
func Usage(path, format string, args ...any) error {
	return &Error{Code: ExitUsage, Err: fmt.Errorf(format, args...), Next: "run '" + path + " --help'"}
}

var errHelp = errors.New("help")

// Main runs args against root, printing errors with their next step, and returns the exit code.
func Main(ctx context.Context, root *Command, args []string, stdin io.Reader, stdout, stderr io.Writer, environ []string) int {
	err := root.run(ctx, root.Name, args, &Exec{Stdin: stdin, Stdout: stdout, Stderr: stderr, Environ: environ})
	if err == nil || errors.Is(err, errHelp) {
		return ExitOK
	}
	code := ExitFailure
	var e *Error
	if errors.As(err, &e) {
		code = e.Code
		if e.Err == nil {
			return code
		}
	}
	fmt.Fprintf(stderr, "%s: %v\n", root.Name, err)
	if e != nil && e.Next != "" {
		fmt.Fprintf(stderr, "next: %s\n", e.Next)
	}
	return code
}

func help(arg string) bool { return arg == "-h" || arg == "--help" || arg == "-help" || arg == "help" }

func (c *Command) find(name string) *Command {
	for _, s := range c.Subs {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (c *Command) run(ctx context.Context, path string, args []string, x *Exec) error {
	if len(c.Subs) > 0 {
		if len(args) > 0 {
			if s := c.find(args[0]); s != nil {
				return s.run(ctx, path+" "+s.Name, args[1:], x)
			}
			if help(args[0]) {
				c.groupHelp(path, x.Stdout)
				return errHelp
			}
		}
		if c.Default != "" && (len(args) == 0 || strings.HasPrefix(args[0], "-")) {
			s := c.find(c.Default)
			return s.run(ctx, path+" "+s.Name, args, x)
		}
		if c.Setup == nil && c.Raw == nil {
			if len(args) == 0 {
				c.groupHelp(path, x.Stderr)
				return &Error{Code: ExitUsage, Err: errors.New("a command is required")}
			}
			return Usage(path, "unknown command %q", args[0])
		}
	}
	x.Path = path
	if c.Raw != nil {
		x.Args = args
		return c.Raw(ctx, x)
	}
	fs := flag.NewFlagSet(path, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runner := c.Setup(fs)
	pos, err := Parse(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		c.leafHelp(path, fs, x.Stdout)
		return errHelp
	}
	if err != nil {
		return Usage(path, "%v", err)
	}
	x.Args = pos
	return runner(ctx, x)
}

// Parse parses fs over args with flags allowed between positional arguments, and returns the positional arguments; "--" ends the flags.
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if len(args) > len(rest) && args[len(args)-len(rest)-1] == "--" {
			return append(pos, rest...), nil
		}
		pos, args = append(pos, rest[0]), rest[1:]
	}
}

func (c *Command) synopsis(path string) string {
	s := "Usage: " + path
	if c.Args != "" {
		s += " " + c.Args
	}
	return s
}

func (c *Command) groupHelp(path string, w io.Writer) {
	fmt.Fprintln(w, c.synopsis(path)+" <command>")
	if c.Summary != "" {
		fmt.Fprintln(w, "\n"+c.Summary)
	}
	if c.Long != "" {
		fmt.Fprintln(w, "\n"+strings.TrimRight(c.Long, "\n"))
	}
	fmt.Fprintln(w, "\nCommands:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range c.Subs {
		name := s.Name
		if s.Name == c.Default {
			name += " (default)"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", name, s.Summary)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nRun '%s <command> --help' for details.\n", path)
}

func (c *Command) leafHelp(path string, fs *flag.FlagSet, w io.Writer) {
	syn := c.synopsis(path)
	n := 0
	fs.VisitAll(func(*flag.Flag) { n++ })
	if n > 0 {
		syn += " [flags]"
	}
	fmt.Fprintln(w, syn)
	if c.Summary != "" {
		fmt.Fprintln(w, "\n"+c.Summary)
	}
	if c.Long != "" {
		fmt.Fprintln(w, "\n"+strings.TrimRight(c.Long, "\n"))
	}
	if n == 0 {
		return
	}
	fmt.Fprintln(w, "\nFlags:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if name != "" {
			left += " " + name
		}
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			usage += " (default " + f.DefValue + ")"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", left, usage)
	})
	tw.Flush()
}

// Getenv reads a variable from environ, a KEY=VALUE list.
func Getenv(environ []string) func(string) string {
	return func(key string) string {
		for _, kv := range environ {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
}
