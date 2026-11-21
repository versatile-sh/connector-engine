// Package admin is "vsh admin": the management operations of the operation table, called over HTTP.
package admin

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/server"
)

// Alias is a "group verb" shorthand for one operation; Positional names the fields the arguments after the path parameters fill.
type Alias struct {
	Group, Verb, Operation string
	Positional             []string
}

// Aliases are the commands of "vsh admin"; any other operation is reached with "vsh admin call <operationId>".
var Aliases = []Alias{
	{"listings", "list", "listListings", nil}, {"listings", "get", "getListing", nil}, {"listings", "create", "createListing", nil},
	{"listings", "lint", "lintListing", nil}, {"listings", "review", "reviewListing", nil}, {"listings", "quarantine", "quarantineListing", nil},
	{"listings", "deprecate", "deprecateListing", nil}, {"listings", "withdraw", "withdrawListing", nil}, {"listings", "restore", "restoreListing", nil},
	{"listings", "offered", "listOfferedListings", nil}, {"listings", "starter", "setListingStarter", nil},
	{"sources", "sync", "syncSource", nil}, {"sources", "list", "listSources", nil},
	{"calls", "list", "listCalls", nil}, {"secrets", "rotate", "rotateKey", nil}, {"issuers", "client", "putClientRegistration", nil},
	{"connectors", "list", "listConnectors", nil}, {"connectors", "get", "getConnector", nil}, {"connectors", "install", "installConnector", nil},
	{"connectors", "enable", "enableConnector", nil}, {"connectors", "disable", "disableConnector", nil}, {"connectors", "update", "updateConnector", nil},
	{"connectors", "level", "setConnectorLevel", nil}, {"connectors", "remove", "removeConnector", nil},
	{"functions", "list", "listFunctions", nil}, {"functions", "accept", "acceptFunction", nil}, {"functions", "classify", "classifyFunction", nil},
	{"functions", "enable", "enableFunction", nil}, {"functions", "disable", "disableFunction", nil}, {"functions", "projection", "setProjection", nil},
	{"bindings", "list", "listBindings", nil}, {"bindings", "put", "putBinding", nil}, {"bindings", "delete", "deleteBinding", nil}, {"bindings", "level", "setAgentLevel", nil},
	{"grants", "list", "listGrants", nil}, {"grants", "create", "createGrant", nil}, {"grants", "delete", "deleteGrant", nil}, {"grants", "confirm", "confirmGrant", nil},
	{"approvals", "list", "listApprovals", nil}, {"approvals", "get", "getApproval", nil}, {"approvals", "decide", "decideApproval", []string{"decision"}},
	{"approvals", "cancel", "cancelApproval", nil},
	{"overlays", "list", "listOverlays", nil}, {"overlays", "suggest", "suggestOverlay", nil}, {"overlays", "propose", "proposeOverlay", nil},
	{"overlays", "accept", "acceptOverlay", nil}, {"overlays", "restore", "restoreOverlay", nil}, {"overlays", "clear", "clearOverlay", nil},
	{"tenants", "create", "createTenant", nil}, {"tenants", "remove", "removeTenant", nil},
	{"roles", "list", "listRoles", nil}, {"roles", "put", "putRole", nil}, {"roles", "delete", "deleteRole", nil},
	{"groups", "list", "listGroupMembers", nil}, {"groups", "put", "putGroupMember", nil}, {"groups", "delete", "deleteGroupMember", nil},
	{"operations", "get", "getOperation", nil}, {"adapters", "list", "listAdapters", nil}, {"events", "list", "listEventTypes", nil}, {"events", "get", "getEventType", nil},
}

// Cmd is the admin command bound to its output and environment.
type Cmd struct {
	out, err io.Writer
	environ  []string
	client   *http.Client
}

// Command returns "vsh admin" writing to out and err; VSH_ADMIN_URL and VSH_ADMIN_TOKEN give the engine and the bearer token.
func Command(out, err io.Writer, environ []string) *Cmd {
	return &Cmd{out: out, err: err, environ: environ, client: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Cmd) env(name string) string {
	for _, kv := range c.environ {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v
		}
	}
	return ""
}

func (c *Cmd) usage() int {
	c.help(c.err)
	return 2
}

func (c *Cmd) help(w io.Writer) {
	fmt.Fprintln(w, "Usage: vsh admin [--url URL] [--token TOKEN] <group> <verb> [path values] [--field value] [--json BODY]\n       vsh admin call <operationId> [path values] [--field value] [--json BODY]")
	fmt.Fprintln(w, "\nCall the engine's management API over HTTP; $VSH_ADMIN_URL and $VSH_ADMIN_TOKEN give the engine and its bearer token.\n\nCommands:")
	var seen []string
	for _, a := range Aliases {
		if !slices.Contains(seen, a.Group) {
			seen = append(seen, a.Group)
			fmt.Fprintf(w, "  %s:", a.Group)
			for _, b := range Aliases {
				if b.Group == a.Group {
					fmt.Fprint(w, " "+b.Verb)
				}
			}
			fmt.Fprintln(w)
		}
	}
}

func operation(id string) (server.Operation, bool) {
	for _, o := range server.Operations() {
		if o.ID == id {
			return o, true
		}
	}
	return server.Operation{}, false
}

func kebab(name string) string { return strings.ReplaceAll(name, "_", "-") }

func scalar(t reflect.Type) bool {
	switch server.Elem(t).Kind() {
	case reflect.String, reflect.Int, reflect.Int32, reflect.Int64, reflect.Bool:
		return true
	}
	return false
}

func typed(f server.Field, raw string) (any, error) {
	switch server.Elem(f.Type).Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64:
		return strconv.ParseInt(raw, 10, 64)
	case reflect.Bool:
		return strconv.ParseBool(raw)
	}
	return raw, nil
}

// Run executes "vsh admin" with args after the word admin; it returns the process exit code.
func (c *Cmd) Run(ctx context.Context, args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		c.help(c.out)
		return 0
	}
	global := flag.NewFlagSet("admin", flag.ContinueOnError)
	global.SetOutput(c.err)
	base := global.String("url", c.env("VSH_ADMIN_URL"), "base address of the engine")
	token := global.String("token", "", "bearer token (from $VSH_ADMIN_TOKEN when empty)")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := global.Args()
	var (
		op         server.Operation
		positional []string
		ok         bool
	)
	switch {
	case len(rest) == 2 && rest[0] == "call" && (rest[1] == "-h" || rest[1] == "--help" || rest[1] == "-help"):
		c.help(c.out)
		return 0
	case len(rest) >= 2 && rest[0] == "call":
		if rest[1] == "-h" || rest[1] == "--help" || rest[1] == "-help" {
			c.help(c.out)
			return 0
		}
		if op, ok = operation(rest[1]); !ok {
			fmt.Fprintf(c.err, "unknown operation %q\n", rest[1])
			return 2
		}
		rest = rest[2:]
	case len(rest) >= 2:
		i := slices.IndexFunc(Aliases, func(a Alias) bool { return a.Group == rest[0] && a.Verb == rest[1] })
		if i < 0 {
			return c.usage()
		}
		op, _ = operation(Aliases[i].Operation)
		positional, rest = Aliases[i].Positional, rest[2:]
	default:
		return c.usage()
	}
	if slices.ContainsFunc(rest, func(a string) bool { return a == "-h" || a == "--help" || a == "-help" }) {
		c.opHelp(op, positional)
		return 0
	}
	if *base == "" {
		fmt.Fprintln(c.err, "no engine address: set --url or VSH_ADMIN_URL")
		return 2
	}
	req, err := c.request(ctx, op, positional, rest, strings.TrimRight(*base, "/"), cmp.Or(*token, c.env("VSH_ADMIN_TOKEN")))
	if err != nil {
		fmt.Fprintln(c.err, err)
		return 2
	}
	return c.send(req)
}

func opFlags(op server.Operation) (*flag.FlagSet, map[string]*string, *string, *string) {
	fs := flag.NewFlagSet(op.ID, flag.ContinueOnError)
	values := map[string]*string{}
	for _, f := range server.Fields(server.Elem(reflect.TypeOf(op.In()))) {
		if scalar(f.Type) && !slices.Contains(op.PathParams(), f.Name) {
			values[f.Name] = fs.String(kebab(f.Name), "", "field "+f.Name)
		}
	}
	raw := fs.String("json", "", "request body or fields as JSON, under the flags")
	key := fs.String("idempotency-key", "", "retry-safe key of a mutation; random when empty")
	return fs, values, raw, key
}

func (c *Cmd) opHelp(op server.Operation, positional []string) {
	fs, _, _, _ := opFlags(op)
	usage := "Usage: vsh admin call " + op.ID
	for _, a := range Aliases {
		if a.Operation == op.ID {
			usage = "Usage: vsh admin " + a.Group + " " + a.Verb
		}
	}
	for _, p := range append(slices.Clone(op.PathParams()), positional...) {
		usage += " <" + kebab(p) + ">"
	}
	command.WriteHelp(c.out, usage, op.Method+" "+op.Path+" (operation "+op.ID+"); $VSH_ADMIN_URL and $VSH_ADMIN_TOKEN give the engine and its bearer token.", fs)
}

func (c *Cmd) request(ctx context.Context, op server.Operation, positional, args []string, base, token string) (*http.Request, error) {
	fields := server.Fields(server.Elem(reflect.TypeOf(op.In())))
	fs, values, raw, key := opFlags(op)
	fs.SetOutput(c.err)
	if err := parseInterleaved(fs, args); err != nil {
		return nil, err
	}
	body := map[string]any{}
	if *raw != "" {
		if err := json.Unmarshal([]byte(*raw), &body); err != nil {
			return nil, fmt.Errorf("--json: %w", err)
		}
	}
	pos := fs.Args()
	names := append(slices.Clone(op.PathParams()), positional...)
	if len(pos) > len(names) {
		return nil, fmt.Errorf("%s takes %d values after its name, got %d", op.ID, len(names), len(pos))
	}
	set := map[string]string{}
	for i, v := range pos {
		set[names[i]] = v
	}
	fs.Visit(func(f *flag.Flag) {
		for name, ptr := range values {
			if kebab(name) == f.Name {
				set[name] = *ptr
			}
		}
	})
	path, query := op.Path, url.Values{}
	for name, v := range set {
		i := slices.IndexFunc(fields, func(f server.Field) bool { return f.Name == name })
		if i < 0 {
			return nil, fmt.Errorf("%s has no field %s", op.ID, name)
		}
		val, err := typed(fields[i], v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		switch {
		case slices.Contains(op.PathParams(), name):
			path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(v))
		case op.HasBody():
			body[name] = val
		default:
			query.Set(name, v)
		}
	}
	for k, v := range body {
		if !op.HasBody() {
			query.Set(k, fmt.Sprint(v))
		}
	}
	var in io.Reader
	if op.HasBody() {
		data, _ := json.Marshal(body)
		in = bytes.NewReader(data)
	}
	target := base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, op.Method, target, in)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(server.HeaderAuthorization, "Bearer "+token)
	}
	if op.Method != http.MethodGet {
		if *key == "" {
			*key = randomKey()
		}
		req.Header.Set(server.HeaderIdempotencyKey, *key)
	}
	return req, nil
}

func parseInterleaved(fs *flag.FlagSet, args []string) error {
	var positional []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional, args = append(positional, args[0]), args[1:]
	}
	return fs.Parse(positional)
}

func randomKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *Cmd) send(req *http.Request) int {
	resp, err := c.client.Do(req)
	if err != nil {
		fmt.Fprintln(c.err, err)
		return 1
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 400 {
		var p struct{ Code, Detail string }
		_ = json.Unmarshal(data, &p)
		fmt.Fprintf(c.err, "%d %s: %s\n", resp.StatusCode, p.Code, p.Detail)
		return 1
	}
	var pretty bytes.Buffer
	if len(data) > 0 && json.Indent(&pretty, data, "", "  ") == nil {
		fmt.Fprintln(c.out, pretty.String())
	}
	return 0
}
