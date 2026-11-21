package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/cli/command"
)

func run(args ...string) (int, string, string) {
	var out, errw bytes.Buffer
	code := command.Main(context.Background(), cli.Core(), args, strings.NewReader(""), &out, &errw, nil)
	return code, out.String(), errw.String()
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	if code, _, errw := run("nope"); code != command.ExitUsage || !strings.Contains(errw, "vsh --help") {
		t.Fatalf("code %d: %s", code, errw)
	}
}

func TestGroupErrorExitsOne(t *testing.T) {
	if code, _, errw := run("tools", "list"); code != command.ExitFailure || !strings.Contains(errw, "CN_ENDPOINT") {
		t.Fatalf("code %d: %s", code, errw)
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"tools", "--help"}, {"tools", "call", "-h"}, {"skills", "export", "--help"}, {"admin", "--help"}, {"admin", "tenants", "create", "--help"}} {
		code, out, errw := run(args...)
		if code != command.ExitOK || !strings.HasPrefix(out, "Usage: vsh") || errw != "" {
			t.Fatalf("%v: code %d out %q err %q", args, code, out, errw)
		}
	}
}
