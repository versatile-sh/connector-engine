package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/cli"
)

func TestUnknownGroupPrintsTheGroups(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Main(context.Background(), []string{"nope"}, &out, &errw, nil, cli.Core()); code != 2 || !strings.Contains(errw.String(), "tools") {
		t.Fatalf("code %d: %s", code, errw.String())
	}
}

func TestGroupErrorExitsOne(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Main(context.Background(), []string{"tools", "list"}, &out, &errw, nil, cli.Core()); code != 1 || !strings.Contains(errw.String(), "CN_ENDPOINT") {
		t.Fatalf("code %d: %s", code, errw.String())
	}
}
