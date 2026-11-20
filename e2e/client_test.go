package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

type caller struct {
	bin   binaries
	url   string
	token string
}

func (c caller) run(ctx context.Context, op string, body any, pos ...string) ([]byte, string, error) {
	args := append([]string{"admin", "call", op}, pos...)
	if body != nil {
		raw, _ := json.Marshal(body)
		args = append(args, "--json", string(raw))
	}
	cmd := exec.CommandContext(ctx, c.bin.vsh, args...)
	cmd.Env = append(os.Environ(), "VSH_ADMIN_URL="+c.url, "VSH_ADMIN_TOKEN="+c.token)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	return out.Bytes(), strings.TrimSpace(errOut.String()), err
}

func (c caller) call(t *testing.T, out any, op string, body any, pos ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, stderr, err := c.run(ctx, op, body, pos...)
	if err != nil {
		t.Fatalf("vsh admin call %s %v: %v: %s", op, pos, err, stderr)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s answered %s: %v", op, raw, err)
		}
	}
}

func (c caller) refused(t *testing.T, op string, body any, pos ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, stderr, err := c.run(ctx, op, body, pos...)
	if err == nil {
		t.Fatalf("vsh admin call %s %v succeeded, want a refusal", op, pos)
	}
	return stderr
}

func (c caller) refusedWith(t *testing.T, status string, op string, body any, pos ...string) {
	t.Helper()
	if msg := c.refused(t, op, body, pos...); !strings.HasPrefix(msg, status) {
		t.Fatalf("%s %v answered %q, want %s", op, pos, msg, status)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
