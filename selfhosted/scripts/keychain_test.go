package scripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const composeStub = `#!/usr/bin/env bash
set -euo pipefail
dir="$STUB_DIR"
case "$1" in
  ps) cat "$dir/$5.id" ;;
  exec)
    service="$3"
    op=""
    while [ $# -gt 0 ]; do if [ "$1" = --op ]; then op="$2"; fi; shift; done
    case "$op" in
      unlock)
        cat >/dev/null
        echo "$service" >>"$dir/deliveries"
        echo wrapped >"$dir/$service.state"
        if [ -e "$dir/$service.restarts" ]; then
          rm "$dir/$service.restarts"
          echo locked >"$dir/$service.state"
          echo "$service-2" >"$dir/$service.id"
        fi
        if [ -e "$dir/$service.stuck" ]; then echo locked >"$dir/$service.state"; fi
        echo wrapped
        ;;
      status) cat "$dir/$service.state" ;;
    esac
    ;;
esac
`

const curlStub = `#!/usr/bin/env bash
if grep -q locked "$STUB_DIR/engine.state" "$STUB_DIR/console.state"; then echo '{"ready":false}'; else echo '{"ready":true}'; fi
`

const vshStub = `#!/usr/bin/env bash
if [ "$2" = get ]; then echo 00112233; fi
`

func unlock(t *testing.T, prepare func(dir string)) (string, string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the scripts run under bash on the host")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"compose": composeStub, "curl": curlStub, "vsh": vshStub} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, service := range []string{"engine", "console"} {
		if err := os.WriteFile(filepath.Join(dir, service+".id"), []byte(service+"-1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, service+".state"), []byte("locked\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prepare(dir)
	cmd := exec.Command("bash", "keychain.sh", "unlock")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "STUB_DIR="+dir,
		"COMPOSE="+filepath.Join(bin, "compose"), "VSH="+filepath.Join(bin, "vsh"),
		"UNLOCK_ATTEMPTS=3", "UNLOCK_BACKOFF=0", "UNLOCK_CONFIRM=2")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	deliveries, _ := os.ReadFile(filepath.Join(dir, "deliveries"))
	return string(deliveries), stderr.String(), err
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockConfirmsBothOnTheFirstTry(t *testing.T) {
	deliveries, stderr, err := unlock(t, func(string) {})
	if err != nil || deliveries != "engine\nconsole\n" {
		t.Fatalf("err %v, deliveries %q, stderr %s", err, deliveries, stderr)
	}
}

func TestUnlockDeliversAgainToARestartedEngine(t *testing.T) {
	deliveries, stderr, err := unlock(t, func(dir string) { touch(t, filepath.Join(dir, "engine.restarts")) })
	if err != nil {
		t.Fatalf("unlock failed: %v\n%s", err, stderr)
	}
	if deliveries != "engine\nconsole\nengine\nconsole\n" || !strings.Contains(stderr, "engine was restarted or recreated after the unlock") {
		t.Fatalf("deliveries %q, stderr %s", deliveries, stderr)
	}
}

func TestUnlockFailsLoudlyWhenTheEngineStaysLocked(t *testing.T) {
	deliveries, stderr, err := unlock(t, func(dir string) { touch(t, filepath.Join(dir, "engine.stuck")) })
	if err == nil || !strings.Contains(stderr, "unlock failed: the engine and the console did not both confirm unlocked after 3 attempts") {
		t.Fatalf("err %v, stderr %s", err, stderr)
	}
	if strings.Count(deliveries, "engine") != 3 {
		t.Fatalf("deliveries %q", deliveries)
	}
}
