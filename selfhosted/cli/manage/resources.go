package manage

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

// Connectors lists the installed connectors as GET /api/connectors answers.
func (c *Client) Connectors(ctx context.Context) ([]console.ConnectorSummary, error) {
	var out struct {
		Connectors []console.ConnectorSummary `json:"connectors"`
	}
	err := c.Do(ctx, http.MethodGet, "/api/connectors", nil, &out)
	return out.Connectors, err
}

// Connector returns the installed connector whose alias or id is name.
func (c *Client) Connector(ctx context.Context, name string) (console.ConnectorSummary, error) {
	list, err := c.Connectors(ctx)
	if err != nil {
		return console.ConnectorSummary{}, err
	}
	for _, k := range list {
		if k.Alias == name || k.ID == name {
			return k, nil
		}
	}
	for _, k := range list {
		if strings.EqualFold(k.Alias, name) {
			return k, nil
		}
	}
	return console.ConnectorSummary{}, command.Fail(command.ExitFailure, "run 'vsh connectors' for the names", "no connector named %q", name)
}

func connectorsCommand() *command.Command {
	var mcpURL, local, yes bool
	return &command.Command{Name: "connectors", Summary: "List, show, import and remove connectors.", Default: "list", Subs: []*command.Command{
		leaf("list", "", "List the installed connectors.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(0, ""); err != nil {
				return err
			}
			list, err := e.Client.Connectors(ctx)
			if err != nil {
				return err
			}
			return e.print(list, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "no connectors yet: vsh connectors import <listing-url>")
					return
				}
				fmt.Fprintln(w, "NAME\tTITLE\tFUNCTIONS\tKEYS\tSTATE")
				for _, k := range list {
					fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", k.Alias, k.Title, len(k.Functions), len(k.Profiles), connectorState(k))
				}
			})
		}, opts{}),
		leaf("show", "<name>", "Show a connector's functions by kind and each key's profile.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the connector's name"); err != nil {
				return err
			}
			k, err := e.Client.Connector(ctx, e.X.Args[0])
			if err != nil {
				return err
			}
			return e.print(k, func(w io.Writer) { showConnector(w, k) })
		}, opts{}),
		leaf("import", "<listing-url>", "Install a connector or a skill from its listing, or an MCP server with --mcp.", func(fs *flag.FlagSet) {
			fs.BoolVar(&mcpURL, "mcp", false, "the address is an MCP server, not a listing")
			fs.BoolVar(&local, "local", false, "allow an MCP server on this machine or its network")
			fs.BoolVar(&yes, "yes", false, "skip the confirmation")
		}, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the listing or MCP address"); err != nil {
				return err
			}
			req := map[string]any{"listing": e.X.Args[0]}
			if mcpURL {
				req = map[string]any{"mcp": e.X.Args[0], "local": local}
			}
			var p console.Preview
			if err := e.Client.Do(ctx, http.MethodPost, "/api/preview", req, &p); err != nil {
				return err
			}
			if !e.JSON {
				showPreview(e.X.Stderr, p)
			}
			if err := e.confirm("Install "+p.Title+" with these choices?", yes || e.JSON); err != nil {
				return err
			}
			if p.Kind == "skill" && p.Skill != nil {
				req["access"], req["digest"] = console.AccessOn, p.Skill.Digest
			}
			var out map[string]string
			if err := e.Client.Do(ctx, http.MethodPost, "/api/import", req, &out); err != nil {
				return err
			}
			return e.print(out, func(w io.Writer) {
				fmt.Fprintf(w, "installed %s\n", p.Title)
				if len(p.Auth) > 0 {
					fmt.Fprintln(w, "sign in to it from the console's Connectors page before an agent calls it")
				}
			})
		}, opts{}),
		leaf("remove", "<name>", "Remove a connector with its functions and credentials.", func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "skip the confirmation")
		}, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the connector's name"); err != nil {
				return err
			}
			k, err := e.Client.Connector(ctx, e.X.Args[0])
			if err != nil {
				return err
			}
			if err := e.confirm("Remove "+k.Alias+" and its credentials?", yes); err != nil {
				return err
			}
			if err := e.Client.Do(ctx, http.MethodDelete, "/api/connectors/"+esc(k.ID), nil, nil); err != nil {
				return err
			}
			return e.print(map[string]string{"removed": k.Alias}, func(w io.Writer) { fmt.Fprintf(w, "removed %s\n", k.Alias) })
		}, opts{}),
	}}
}

func connectorState(k console.ConnectorSummary) string {
	switch {
	case k.Blocked != "":
		return "blocked: " + k.Blocked
	case !k.Enabled:
		return "disabled"
	}
	return "enabled"
}

// Kinds are the kinds of function in the order the console shows them.
var Kinds = []port.Class{port.ClassRead, port.ClassWrite, port.ClassDestructive}

func showConnector(w io.Writer, k console.ConnectorSummary) {
	fmt.Fprintf(w, "%s\t%s   %s\n", k.Alias, k.Title, connectorState(k))
	for _, kind := range Kinds {
		first := true
		for _, f := range k.Functions {
			if f.Kind != kind {
				continue
			}
			if first {
				fmt.Fprintf(w, "\n%s\n", strings.ToUpper(string(kind)))
				first = false
			}
			fmt.Fprintf(w, "  %s\t%s\n", f.Name, f.Access)
		}
	}
	if len(k.Profiles) > 0 {
		fmt.Fprintln(w, "\nKEYS")
		for _, p := range k.Profiles {
			fmt.Fprintf(w, "  %s\t%s", p.Agent, p.Mode)
			for _, name := range slices.Sorted(maps.Keys(p.Overrides)) {
				fmt.Fprintf(w, "  %s=%s", name, p.Overrides[name])
			}
			fmt.Fprintln(w)
		}
	}
}

func showPreview(w io.Writer, p console.Preview) {
	fmt.Fprintf(w, "%s (%s) from %s\n", p.Title, p.Kind, p.Host)
	if p.Warning != "" {
		fmt.Fprintln(w, "warning:", p.Warning)
	}
	for _, t := range p.Tools {
		fmt.Fprintf(w, "  %-12s %-5s %s\n", t.Kind, t.Proposed, t.Name)
	}
}

func keysCommand() *command.Command {
	var ttl time.Duration
	var yes bool
	return &command.Command{Name: "keys", Summary: "List, create and revoke the access keys agents connect with.", Default: "list", Subs: []*command.Command{
		leaf("list", "", "List the access keys.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(0, ""); err != nil {
				return err
			}
			var out struct {
				Agents []console.AgentView `json:"agents"`
			}
			if err := e.Client.Do(ctx, http.MethodGet, "/api/agents", nil, &out); err != nil {
				return err
			}
			return e.print(out.Agents, func(w io.Writer) {
				if len(out.Agents) == 0 {
					fmt.Fprintln(w, "no access keys yet: vsh keys create <name>")
					return
				}
				fmt.Fprintln(w, "NAME\tEXPIRES\tVIA")
				for _, a := range out.Agents {
					via := "key"
					if a.ViaBrowser {
						via = "browser sign-in"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.ExpiresAt.Local().Format("2006-01-02 15:04"), via)
				}
			})
		}, opts{}),
		leaf("create", "<name>", "Create an access key; it is printed once.", func(fs *flag.FlagSet) {
			fs.DurationVar(&ttl, "ttl", 0, "how long the key lives, such as 720h; 0 takes the longest the console allows")
		}, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "a name for the key, such as the agent's"); err != nil {
				return err
			}
			hours := int(ttl.Hours())
			if hours == 0 {
				var keys struct {
					Max int `json:"max_ttl_hours"`
				}
				if err := e.Client.Do(ctx, http.MethodGet, "/api/agents", nil, &keys); err != nil {
					return err
				}
				hours = keys.Max
			}
			var tok console.AgentToken
			if err := e.Client.Do(ctx, http.MethodPost, "/api/agents", map[string]any{"name": e.X.Args[0], "ttl_hours": hours}, &tok); err != nil {
				return err
			}
			return e.print(tok, func(w io.Writer) {
				fmt.Fprintln(e.X.Stderr, "access key of "+tok.Agent+", shown this once; give it to the agent as CN_AGENT_TOKEN with CN_ENDPOINT="+e.Client.Base+"/mcp")
				fmt.Fprintln(w, tok.Token)
			})
		}, opts{}),
		leaf("revoke", "<name>", "Revoke an access key for good.", func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "skip the confirmation")
		}, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the key's name"); err != nil {
				return err
			}
			if err := e.confirm("Revoke "+e.X.Args[0]+"? Agents using it lose access at once.", yes); err != nil {
				return err
			}
			if err := e.Client.Do(ctx, http.MethodDelete, "/api/agents/"+esc(e.X.Args[0]), nil, nil); err != nil {
				return err
			}
			return e.print(map[string]string{"revoked": e.X.Args[0]}, func(w io.Writer) { fmt.Fprintf(w, "revoked %s\n", e.X.Args[0]) })
		}, opts{}),
	}}
}

func approvalsCommand() *command.Command {
	decide := func(name, done, summary string, approve bool) *command.Command {
		return leaf(name, "<id>", summary, nil, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the approval's id, from 'vsh approvals'"); err != nil {
				return err
			}
			if err := e.Client.Do(ctx, http.MethodPost, "/api/approvals/"+esc(e.X.Args[0]), map[string]bool{"approve": approve}, nil); err != nil {
				return err
			}
			return e.print(map[string]any{"id": e.X.Args[0], "approved": approve}, func(w io.Writer) { fmt.Fprintf(w, "%s %s\n", done, e.X.Args[0]) })
		}, opts{})
	}
	return &command.Command{Name: "approvals", Summary: "List the calls waiting for approval and approve or deny them.", Default: "list", Subs: []*command.Command{
		leaf("list", "", "List the calls waiting for approval.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(0, ""); err != nil {
				return err
			}
			list, err := e.Client.Approvals(ctx)
			if err != nil {
				return err
			}
			names := map[port.ConnectorID]string{}
			if conns, err := e.Client.Connectors(ctx); err == nil {
				for _, k := range conns {
					names[port.ConnectorID(k.ID)] = k.Alias
				}
			}
			return e.print(list, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "nothing waits for approval")
					return
				}
				fmt.Fprintln(w, "ID\tKEY\tCONNECTOR\tFUNCTION\tEXPIRES")
				for _, a := range list {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.ID, a.Agent, cmp.Or(names[a.Connector], string(a.Connector)), a.Function, a.ExpiresAt.Local().Format("15:04:05"))
				}
			})
		}, opts{}),
		decide("approve", "approved", "Let one waiting call run.", true),
		decide("deny", "denied", "Refuse one waiting call.", false),
	}}
}

// Approvals lists the calls waiting for approval.
func (c *Client) Approvals(ctx context.Context) ([]port.Approval, error) {
	var out struct {
		Approvals []port.Approval `json:"approvals"`
	}
	err := c.Do(ctx, http.MethodGet, "/api/approvals", nil, &out)
	return out.Approvals, err
}

// Activity returns the calls from since to now, narrowed to a connector id and a key when given, oldest first.
func (c *Client) Activity(ctx context.Context, since time.Time, connector, key string) ([]console.ActivityCall, error) {
	q := url.Values{"from": {since.UTC().Format(time.RFC3339Nano)}, "to": {time.Now().UTC().Format(time.RFC3339Nano)}}
	if connector != "" {
		q.Set("connector", connector)
	}
	if key != "" {
		q.Set("agent", key)
	}
	var page console.ActivityPage
	if err := c.Do(ctx, http.MethodGet, "/api/activity?"+q.Encode(), nil, &page); err != nil {
		return nil, err
	}
	slices.Reverse(page.Calls)
	return page.Calls, nil
}

func activityCommand() *command.Command {
	var connector, key string
	var since, every time.Duration
	var follow bool
	return leaf("activity", "", "Show the calls agents made, or follow them live with --follow.", func(fs *flag.FlagSet) {
		fs.StringVar(&connector, "connector", "", "only the calls of this connector")
		fs.StringVar(&key, "key", "", "only the calls of this access key")
		fs.DurationVar(&since, "since", time.Hour, "how far back to start, such as 30m or 24h")
		fs.BoolVar(&follow, "follow", false, "keep printing new calls until interrupted")
		fs.DurationVar(&every, "every", 2*time.Second, "how often --follow asks the console")
	}, func(ctx context.Context, e *Env) error {
		if err := e.args(0, ""); err != nil {
			return err
		}
		id := ""
		if connector != "" {
			k, err := e.Client.Connector(ctx, connector)
			if err != nil {
				return err
			}
			id = k.ID
		}
		from, seen := time.Now().Add(-since), map[string]string{}
		for {
			calls, err := e.Client.Activity(ctx, from, id, key)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if !follow {
				return e.print(calls, func(w io.Writer) {
					if len(calls) == 0 {
						fmt.Fprintln(w, "no calls in that time")
					}
					for _, c := range calls {
						callLine(w, c)
					}
				})
			}
			for _, c := range calls {
				if seen[c.ID] == string(c.Outcome) {
					continue
				}
				seen[c.ID] = string(c.Outcome)
				if err := e.print(c, func(w io.Writer) { callLine(w, c) }); err != nil {
					return err
				}
			}
			if len(calls) > 0 {
				from = latest(from, calls[len(calls)-1].At.Add(-followOverlap))
				for id := range seen {
					if !slices.ContainsFunc(calls, func(c console.ActivityCall) bool { return c.ID == id }) {
						delete(seen, id)
					}
				}
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(every):
			}
		}
	}, opts{})
}

const followOverlap = time.Minute

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func callLine(w io.Writer, c console.ActivityCall) {
	where := c.Alias
	if where == "" {
		where = c.Connector
	}
	d := ""
	if c.DurationMS != nil {
		d = fmt.Sprintf("%dms", *c.DurationMS)
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", c.At.Local().Format("2006-01-02 15:04:05"), c.Agent, where, c.Function, c.Outcome, d)
}

func skillsList() *command.Command {
	return leaf("list", "", "List the installed skills and each key's access.", nil, func(ctx context.Context, e *Env) error {
		if err := e.args(0, ""); err != nil {
			return err
		}
		var out struct {
			Skills []console.SkillSummary `json:"skills"`
		}
		if err := e.Client.Do(ctx, http.MethodGet, "/api/skills", nil, &out); err != nil {
			return err
		}
		return e.print(out.Skills, func(w io.Writer) {
			if len(out.Skills) == 0 {
				fmt.Fprintln(w, "no skills yet: vsh connectors import <skill-listing-url>")
				return
			}
			fmt.Fprintln(w, "NAME\tTITLE\tSTATE\tKEYS")
			for _, s := range out.Skills {
				var keys []string
				for _, a := range s.Access {
					keys = append(keys, a.Agent+"="+string(a.Access))
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Alias, s.Title, s.State, strings.Join(keys, " "))
			}
		})
	}, opts{})
}
