// Package tools is the agent CLI of list, search and call through the agent endpoint, results to a file and a short summary on stdout.
package tools

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/cli/command"
)

const (
	envEndpoint = "CN_ENDPOINT"
	envToken    = "CN_AGENT_TOKEN"
	fieldsKey   = "connector-engine/fields"
	toolFind    = "find"
	maxNames    = 10
)

// Command is the `tools` group of vsh; it is an MCP client of the agent endpoint, not a fourth surface.
type Command struct {
	getenv func(string) string
}

// New returns the command; a nil getenv reads the process environment.
func New(getenv func(string) string) *Command {
	if getenv == nil {
		getenv = os.Getenv
	}
	return &Command{getenv: getenv}
}

const usage = `usage: tools list [--out FILE]
       tools search QUERY [--limit N] [--out FILE]
       tools call NAME [--args JSON] [--fields /a,/b] [--out FILE]
environment: CN_ENDPOINT (the /mcp address), CN_AGENT_TOKEN (the token of this run)`

type options struct {
	out     string
	args    string
	fields  string
	limit   int
	timeout time.Duration
}

func flags(fs *flag.FlagSet, sub string) *options {
	var o options
	fs.StringVar(&o.out, "out", "", "file for the full answer; a temporary file when empty")
	if sub == "call" {
		fs.StringVar(&o.args, "args", "{}", "JSON arguments of the function")
		fs.StringVar(&o.fields, "fields", "", "comma-separated JSON Pointer paths of the result to keep")
	}
	if sub == "search" {
		fs.IntVar(&o.limit, "limit", 0, "most functions search returns")
	}
	fs.DurationVar(&o.timeout, "timeout", 2*time.Minute, "time allowed for the whole command")
	return &o
}

// Run executes one subcommand; the full answer goes to the --out file and a summary to stdout.
func (c *Command) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return fmt.Errorf("a subcommand is required")
	}
	sub, rest := args[0], args[1:]
	if _, known := needs[sub]; !known {
		fmt.Fprintln(stderr, usage)
		return fmt.Errorf("unknown subcommand %q", sub)
	}
	fs := flag.NewFlagSet("tools "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := flags(fs, sub)
	positional, err := parse(fs, rest)
	if err != nil {
		return err
	}
	return c.do(ctx, sub, positional, *o, stdout)
}

var needs = map[string]string{"list": "", "search": "query", "call": "function name"}

func (c *Command) do(ctx context.Context, sub, positional string, o options, stdout io.Writer) error {
	if o.args == "" {
		o.args = "{}"
	}
	if needs[sub] != "" && positional == "" {
		return fmt.Errorf("%s needs a %s", sub, needs[sub])
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	cs, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer cs.Close()
	switch sub {
	case "list":
		return list(ctx, cs, o, stdout)
	case "search":
		return search(ctx, cs, positional, o, stdout)
	}
	return call(ctx, cs, positional, o, stdout)
}

// Commands returns the list, search and call leaves of vsh tools.
func Commands() []*command.Command {
	leaf := func(sub, args, summary string) *command.Command {
		return &command.Command{Name: sub, Args: args, Summary: summary, Long: "Environment: CN_ENDPOINT (the /mcp address), CN_AGENT_TOKEN (the token of this agent).",
			Setup: func(fs *flag.FlagSet) command.Runner {
				o := flags(fs, sub)
				return func(ctx context.Context, x *command.Exec) error {
					if len(x.Args) > 1 {
						return command.Usage(x.Path, "unexpected argument %q", x.Args[1])
					}
					return New(x.Getenv).do(ctx, sub, strings.Join(x.Args, ""), *o, x.Stdout)
				}
			}}
	}
	return []*command.Command{
		leaf("list", "", "List the functions this agent can call."),
		leaf("search", "<query>", "Search the functions this agent can call."),
		leaf("call", "<function>", "Call a function and write its answer to a file."),
	}
}

func parse(fs *flag.FlagSet, args []string) (string, error) {
	positional := ""
	for {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		if positional != "" {
			return "", fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
		positional, args = fs.Arg(0), fs.Args()[1:]
	}
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (c *Command) connect(ctx context.Context) (*mcp.ClientSession, error) {
	return Dial(ctx, c.getenv)
}

// Dial opens an MCP session on the agent endpoint named by CN_ENDPOINT with the CN_AGENT_TOKEN bearer.
func Dial(ctx context.Context, getenv func(string) string) (*mcp.ClientSession, error) {
	endpoint, token := getenv(envEndpoint), getenv(envToken)
	if endpoint == "" || token == "" {
		return nil, fmt.Errorf("set %s and %s", envEndpoint, envToken)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "vsh", Version: "1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearer{token}}}, nil)
}

func write(o options, v any) (string, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	path := o.out
	if path == "" {
		f, err := os.CreateTemp("", "vsh-tools-*.json")
		if err != nil {
			return "", err
		}
		path = f.Name()
		_ = f.Close()
	}
	return path, os.WriteFile(path, raw, 0o600)
}

func list(ctx context.Context, cs *mcp.ClientSession, o options, stdout io.Writer) error {
	var all []*mcp.Tool
	for t, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		all = append(all, t)
	}
	path, err := write(o, all)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%d tools: %s\nfull definitions in %s\n", len(all), nameList(all), path)
	return nil
}

func nameList(tools []*mcp.Tool) string {
	var names []string
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return shorten(names)
}

func shorten(names []string) string {
	if len(names) > maxNames {
		return strings.Join(names[:maxNames], ", ") + fmt.Sprintf(", and %d more", len(names)-maxNames)
	}
	return strings.Join(names, ", ")
}

func search(ctx context.Context, cs *mcp.ClientSession, query string, o options, stdout io.Writer) error {
	args := map[string]any{"query": query}
	if o.limit > 0 {
		args["limit"] = o.limit
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: toolFind, Arguments: args})
	if err != nil {
		return err
	}
	path, err := write(o, res)
	if err != nil {
		return err
	}
	var found struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	raw, _ := json.Marshal(res.StructuredContent)
	_ = json.Unmarshal(raw, &found)
	var names []string
	for _, t := range found.Tools {
		names = append(names, t.Name)
	}
	fmt.Fprintf(stdout, "%d matches: %s\nfull definitions in %s\n", len(names), shorten(names), path)
	if res.IsError {
		return fmt.Errorf("search failed, see %s", path)
	}
	return nil
}

func call(ctx context.Context, cs *mcp.ClientSession, name string, o options, stdout io.Writer) error {
	if !json.Valid([]byte(o.args)) {
		return fmt.Errorf("--args is not valid JSON")
	}
	params := &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(o.args)}
	if o.fields != "" {
		params.Meta = mcp.Meta{fieldsKey: strings.Split(o.fields, ",")}
	}
	res, err := cs.CallTool(ctx, params)
	if err != nil {
		return err
	}
	path, err := write(o, res)
	if err != nil {
		return err
	}
	structured := map[string]any{}
	raw, _ := json.Marshal(res.StructuredContent)
	_ = json.Unmarshal(raw, &structured)
	switch {
	case res.IsError:
		fmt.Fprintf(stdout, "error %v: %v\nfull answer in %s\n", structured["code"], structured["detail"], path)
		return fmt.Errorf("the call failed, see %s", path)
	case structured["status"] == "pending_approval":
		fmt.Fprintf(stdout, "pending_approval approval_id=%v poll_after_seconds=%v\nask again with: tools call approval_status --args '{\"approval_id\":%q}'\nfull answer in %s\n",
			structured["approval_id"], structured["poll_after_seconds"], structured["approval_id"], path)
	default:
		fmt.Fprintf(stdout, "ok, %d bytes\nfull answer in %s\n", len(raw), path)
	}
	return nil
}
