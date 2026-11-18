package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestUnknownGroupPrintsTheGroups(t *testing.T) {
	var out, errw bytes.Buffer
	if code := run(context.Background(), []string{"nope"}, &out, &errw); code != 2 || !strings.Contains(errw.String(), "tools") {
		t.Fatalf("code %d: %s", code, errw.String())
	}
}

func TestGroupErrorExitsOne(t *testing.T) {
	var out, errw bytes.Buffer
	if code := run(context.Background(), []string{"tools", "list"}, &out, &errw); code != 1 || !strings.Contains(errw.String(), "CN_ENDPOINT") {
		t.Fatalf("code %d: %s", code, errw.String())
	}
}
