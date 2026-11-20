package command_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/cli/command"
)

func tree(got *[]string, key *string) *command.Command {
	leaf := func(name string) *command.Command {
		return &command.Command{Name: name, Args: "<x>", Summary: name + " things.", Setup: func(fs *flag.FlagSet) command.Runner {
			k := fs.String("key", "", "the key")
			return func(_ context.Context, x *command.Exec) error {
				*got, *key = append([]string{x.Path}, x.Args...), *k
				if len(x.Args) > 0 && x.Args[0] == "fail" {
					return command.Fail(command.ExitAuth, "run 'vsh login'", "sign in first")
				}
				if len(x.Args) > 0 && x.Args[0] == "boom" {
					return errors.New("boom")
				}
				return nil
			}
		}}
	}
	return &command.Command{Name: "vsh", Subs: []*command.Command{{Name: "keys", Summary: "Keys.", Default: "list", Subs: []*command.Command{leaf("list"), leaf("create")}}}}
}

func TestDispatch(t *testing.T) {
	cases := []struct {
		args []string
		code int
		path string
		key  string
		err  string
	}{
		{[]string{"keys"}, 0, "vsh keys list", "", ""},
		{[]string{"keys", "--key", "k"}, 0, "vsh keys list", "k", ""},
		{[]string{"keys", "create", "a", "--key", "k", "b"}, 0, "vsh keys create a b", "k", ""},
		{[]string{"keys", "create", "--", "--key"}, 0, "vsh keys create --key", "", ""},
		{[]string{"keys", "create", "fail"}, command.ExitAuth, "", "", "next: run 'vsh login'"},
		{[]string{"keys", "create", "boom"}, command.ExitFailure, "", "", "vsh: boom"},
		{[]string{"keys", "create", "--nope"}, command.ExitUsage, "", "", "vsh keys create --help"},
		{[]string{"keys", "nope"}, command.ExitUsage, "", "", "unknown command"},
		{[]string{}, command.ExitUsage, "", "", "Commands:"},
	}
	for _, c := range cases {
		var got []string
		var key string
		var out, errw bytes.Buffer
		code := command.Main(context.Background(), tree(&got, &key), c.args, nil, &out, &errw, nil)
		if code != c.code || (c.path != "" && strings.Join(got, " ") != c.path) || key != c.key || !strings.Contains(errw.String(), c.err) {
			t.Errorf("%v: code %d ran %v key %q err %q", c.args, code, got, key, errw.String())
		}
	}
}

func TestHelp(t *testing.T) {
	var got []string
	var key string
	var out bytes.Buffer
	if code := command.Main(context.Background(), tree(&got, &key), []string{"keys", "--help"}, nil, &out, &out, nil); code != 0 {
		t.Fatalf("code %d", code)
	}
	if want := "Usage: vsh keys <command>\n\nKeys.\n\nCommands:\n  list (default)  list things.\n  create          create things.\n\nRun 'vsh keys <command> --help' for details.\n"; out.String() != want {
		t.Fatalf("group help:\n%s", out.String())
	}
	out.Reset()
	command.Main(context.Background(), tree(&got, &key), []string{"keys", "create", "-h"}, nil, &out, &out, nil)
	if want := "Usage: vsh keys create <x> [flags]\n\ncreate things.\n\nFlags:\n  --key string  the key\n"; out.String() != want {
		t.Fatalf("leaf help:\n%s", out.String())
	}
	if got != nil {
		t.Fatalf("help ran the command: %v", got)
	}
}
