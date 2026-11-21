package manage

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

type batchResult struct {
	Items []struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	} `json:"items"`
	Keys []struct {
		Agent string `json:"agent"`
		Error string `json:"error"`
	} `json:"keys"`
}

func (b batchResult) err() error {
	var failed []string
	for _, it := range b.Items {
		if it.Error != "" {
			failed = append(failed, it.Name+": "+it.Error)
		}
	}
	for _, k := range b.Keys {
		if k.Error != "" {
			failed = append(failed, k.Agent+": "+k.Error)
		}
	}
	if len(failed) > 0 {
		return command.Fail(command.ExitFailure, "", "the console refused %s", strings.Join(failed, "; "))
	}
	return nil
}

// SetAccess sets names of connector k to access, as the connector's own access when key is empty or as key's profile otherwise, in one request.
func (c *Client) SetAccess(ctx context.Context, k console.ConnectorSummary, key string, names []string, access console.Access) error {
	var out batchResult
	switch {
	case key != "":
		if err := c.Do(ctx, http.MethodPut, "/api/connectors/"+esc(k.ID)+"/profiles/"+esc(key), map[string]any{"names": names, "access": access}, &out); err != nil {
			return err
		}
	case len(names) == 1 && !isFunction(k, names[0]):
		return c.Do(ctx, http.MethodPut, "/api/connectors/"+esc(k.ID)+"/items", map[string]any{"name": names[0], "access": access}, nil)
	case len(names) == 1:
		return c.Do(ctx, http.MethodPut, "/api/connectors/"+esc(k.ID)+"/functions/"+esc(names[0]), map[string]any{"access": access}, nil)
	default:
		if err := c.Do(ctx, http.MethodPut, "/api/connectors/"+esc(k.ID)+"/functions", map[string]any{"names": names, "access": access}, &out); err != nil {
			return err
		}
	}
	return out.err()
}

// SetMode makes key follow connector k, or keep its own custom profile, or reach it not at all.
func (c *Client) SetMode(ctx context.Context, k console.ConnectorSummary, key string, mode console.ProfileMode) error {
	var out batchResult
	if err := c.Do(ctx, http.MethodPut, "/api/connectors/"+esc(k.ID)+"/profiles", map[string]any{"mode": mode, "keys": []string{key}}, &out); err != nil {
		return err
	}
	return out.err()
}

func isFunction(k console.ConnectorSummary, name string) bool {
	return slices.ContainsFunc(k.Functions, func(f console.FunctionView) bool { return f.Name == name })
}

// FunctionsOf returns the names of k's functions of kind, or of every kind when kind is empty.
func FunctionsOf(k console.ConnectorSummary, kind port.Class) []string {
	var out []string
	for _, f := range k.Functions {
		if kind == "" || f.Kind == kind {
			out = append(out, f.Name)
		}
	}
	return out
}

func accessCommand() *command.Command {
	var key, kind string
	return leaf("access", "<connector> [<function>] on|off|ask|follow|custom", "Set who may call what: a connector's access, or one key's override with --key.", func(fs *flag.FlagSet) {
		fs.StringVar(&key, "key", "", "change this access key's profile instead of the connector's own access")
		fs.StringVar(&kind, "kind", "", "without a function, change only the functions of this kind: read, write, destructive or undeclared")
	}, func(ctx context.Context, e *Env) error {
		args := e.X.Args
		if len(args) < 2 || len(args) > 3 {
			return command.Usage(e.X.Path, "give the connector, an optional function, and on, off, ask, follow or custom")
		}
		value := args[len(args)-1]
		k, err := e.Client.Connector(ctx, args[0])
		if err != nil {
			return err
		}
		what := k.Alias
		switch value {
		case "follow", "custom":
			if key == "" || len(args) == 3 || kind != "" {
				return command.Usage(e.X.Path, "%s sets a key's profile on the whole connector: give --key and no function", value)
			}
			if err := e.Client.SetMode(ctx, k, key, console.ProfileMode(value)); err != nil {
				return err
			}
		case "on", "off", "ask":
			names := FunctionsOf(k, port.Class(kind))
			if kind != "" && !slices.Contains(Kinds, port.Class(kind)) {
				return command.Usage(e.X.Path, "--kind is read, write, destructive or undeclared")
			}
			if len(args) == 3 {
				if kind != "" {
					return command.Usage(e.X.Path, "give a function or --kind, not both")
				}
				names, what = []string{args[1]}, k.Alias+" "+args[1]
			}
			if len(names) == 0 {
				return command.Fail(command.ExitFailure, "run 'vsh connectors show "+k.Alias+"'", "%s has no such functions", k.Alias)
			}
			if err := e.Client.SetAccess(ctx, k, key, names, console.Access(value)); err != nil {
				return err
			}
		default:
			return command.Usage(e.X.Path, "%q is not on, off, ask, follow or custom", value)
		}
		if key != "" {
			what += " for " + key
		}
		return e.print(map[string]any{"connector": k.Alias, "key": key, "set": value}, func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", what, value) })
	}, opts{})
}
