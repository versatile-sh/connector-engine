package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/app"
)

func TestInitWritesTheFirstBootSecretsOnce(t *testing.T) {
	dir := t.TempDir()
	var out, errw bytes.Buffer
	if code := app.Main(app.Build{}, []string{"init", "--dir", dir, "--db-host", "db:5432"}, nil, nil, &out, &errw); code != 0 {
		t.Fatalf("code %d: %s", code, errw.String())
	}
	url, err := os.ReadFile(filepath.Join(dir, "database-url"))
	if err != nil || !strings.Contains(string(url), "@db:5432/connector_engine") {
		t.Fatalf("database-url %q: %v", url, err)
	}
	token, _ := os.ReadFile(filepath.Join(dir, "operator-token"))
	out.Reset()
	if code := app.Main(app.Build{}, []string{"init", "--dir", dir}, nil, nil, &out, &errw); code != 0 || out.Len() != 0 {
		t.Fatalf("second init: code %d, wrote %q", code, out.String())
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "operator-token")); !bytes.Equal(token, again) {
		t.Fatal("a second init rewrote the operator token")
	}
}

func TestEngineSubcommandsAnswerHelpWithExitZero(t *testing.T) {
	for _, args := range [][]string{{"init", "--help"}, {"adapters", "--help"}, {"adapters", "check", "-h"}} {
		var out, errw bytes.Buffer
		if code := app.Main(app.Build{}, args, nil, nil, &out, &errw); code != 0 || out.Len() == 0 {
			t.Fatalf("%v: %d %q %q", args, code, out.String(), errw.String())
		}
	}
}

func TestABuildCannotShadowABuiltInSubcommand(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	app.Main(app.Build{Commands: map[string]app.Command{"init": nil}}, nil, nil, nil, nil, nil)
}
