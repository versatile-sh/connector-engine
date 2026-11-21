package manage

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func peopleCommand() *command.Command {
	var yes bool
	return &command.Command{Name: "people", Summary: "List the people who use this console, invite more and change or remove them.", Default: "list", Subs: []*command.Command{
		leaf("list", "", "List the people, their roles and the links still waiting.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(0, ""); err != nil {
				return err
			}
			data, err := e.Client.People(ctx)
			if err != nil {
				return err
			}
			return e.print(data, func(w io.Writer) {
				fmt.Fprintln(w, "USERNAME\tROLE\tKEYS")
				for _, p := range data.People {
					fmt.Fprintf(w, "%s\t%s\t%d\n", p.Username, p.Role, p.Keys)
				}
				for _, l := range data.Invites {
					fmt.Fprintf(w, "(%s link %s)\t%s\texpires %s\n", l.Purpose, l.ID, l.Role, l.ExpiresAt.Local().Format("2006-01-02 15:04"))
				}
			})
		}, opts{}),
		leaf("invite", "owner|admin|member", "Make a link that lets one person join with that role; it is printed once.", nil, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the role: owner, admin or member"); err != nil {
				return err
			}
			var out struct {
				Link      string `json:"link"`
				ExpiresAt string `json:"expires_at"`
			}
			if err := e.Client.Do(ctx, http.MethodPost, "/api/invites", map[string]string{"role": e.X.Args[0]}, &out); err != nil {
				return err
			}
			return e.print(out, func(w io.Writer) {
				fmt.Fprintln(e.X.Stderr, "works once, until "+out.ExpiresAt+"; open it on the console's own address")
				fmt.Fprintln(w, out.Link)
			})
		}, opts{}),
		leaf("role", "<username> owner|admin|member", "Give a person another role.", nil, func(ctx context.Context, e *Env) error {
			if len(e.X.Args) != 2 {
				return command.Usage(e.X.Path, "give the username and the role")
			}
			p, err := e.Client.Person(ctx, e.X.Args[0])
			if err != nil {
				return err
			}
			if err := e.Client.Do(ctx, http.MethodPut, "/api/people/"+esc(p.Person)+"/role", map[string]string{"role": e.X.Args[1]}, nil); err != nil {
				return err
			}
			return e.print(map[string]string{"username": p.Username, "role": e.X.Args[1]}, func(w io.Writer) { fmt.Fprintf(w, "%s is now %s\n", p.Username, e.X.Args[1]) })
		}, opts{}),
		leaf("remove", "<username>", "Remove a person: they are signed out and their keys stop working.", func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "skip the confirmation")
		}, func(ctx context.Context, e *Env) error {
			if err := e.args(1, "the username"); err != nil {
				return err
			}
			p, err := e.Client.Person(ctx, e.X.Args[0])
			if err != nil {
				return err
			}
			if err := e.confirm("Remove "+p.Username+"? They are signed out, their keys stop working now and their provider sign-ins are deleted.", yes); err != nil {
				return err
			}
			if err := e.Client.Do(ctx, http.MethodDelete, "/api/people/"+esc(p.Person), nil, nil); err != nil {
				return err
			}
			return e.print(map[string]string{"removed": p.Username}, func(w io.Writer) { fmt.Fprintf(w, "removed %s\n", p.Username) })
		}, opts{}),
	}}
}

// PeopleData is what the console lists on its People page.
type PeopleData struct {
	People  []console.PersonRow  `json:"people"`
	Invites []console.LinkRow    `json:"invites"`
	Audit   []console.AuditEntry `json:"audit"`
}

// People lists the console's people, the links waiting and the newest audit entries.
func (c *Client) People(ctx context.Context) (PeopleData, error) {
	var out PeopleData
	err := c.Do(ctx, http.MethodGet, "/api/people", nil, &out)
	return out, err
}

// Person finds the user named username, ignoring case.
func (c *Client) Person(ctx context.Context, username string) (console.PersonRow, error) {
	data, err := c.People(ctx)
	if err != nil {
		return console.PersonRow{}, err
	}
	for _, p := range data.People {
		if strings.EqualFold(p.Username, username) {
			return p, nil
		}
	}
	return console.PersonRow{}, command.Fail(command.ExitFailure, "run 'vsh people'", "nobody is named %s", username)
}
