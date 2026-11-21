package keychain

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

var ctx = context.Background()

type fake map[string]string

func (f fake) Get(_ context.Context, a string) (string, error) {
	if v, ok := f[a]; ok {
		return v, nil
	}
	return "", ErrNotFound
}

func (f fake) Set(_ context.Context, a, s string) error { f[a] = s; return nil }

func (f fake) Delete(_ context.Context, a string) error { delete(f, a); return nil }

func vsh(t *testing.T, kc Keychain, stdin string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(ctx, kc, args, strings.NewReader(stdin), &out, &errOut)
	return code, strings.TrimSpace(out.String() + errOut.String())
}

func TestEnsureCreatesTheKeyOnceAndGetReturnsIt(t *testing.T) {
	kc := fake{}
	code, first := vsh(t, kc, "", "ensure", "--name", "p1")
	if b, err := hex.DecodeString(first); code != 0 || err != nil || len(b) != 32 {
		t.Fatalf("ensure: %d %q", code, first)
	}
	if code, again := vsh(t, kc, "", "ensure", "--name", "p1"); code != 0 || again != first {
		t.Fatalf("a second ensure keeps the key: %q", again)
	}
	if code, got := vsh(t, kc, "", "get", "--name", "p1"); code != 0 || got != first {
		t.Fatalf("get: %d %q", code, got)
	}
	if code, _ := vsh(t, kc, "", "get", "--name", "p2"); code != 1 {
		t.Fatalf("another install has no key: %d", code)
	}
}

func TestImportRestoresARecoveryKeyButNeverOverwritesAnother(t *testing.T) {
	kc := fake{}
	key := strings.Repeat("ab", 32)
	if code, out := vsh(t, kc, key+"\n", "import", "--name", "p1"); code != 0 || kc["p1"] != key {
		t.Fatalf("import: %d %q", code, out)
	}
	if code, _ := vsh(t, kc, key, "import", "--name", "p1"); code != 0 {
		t.Fatal("importing the same key again is a no-op")
	}
	if code, _ := vsh(t, kc, strings.Repeat("cd", 32), "import", "--name", "p1"); code != 1 || kc["p1"] != key {
		t.Fatal("another key must not replace the stored one")
	}
	if code, _ := vsh(t, kc, "short", "import", "--name", "p3"); code != 1 {
		t.Fatal("a malformed key is refused")
	}
}

func TestNoKeychainExitsWithItsOwnCode(t *testing.T) {
	if code, _ := vsh(t, nil, "", "ensure", "--name", "p1"); code != ExitUnavailable {
		t.Fatalf("exit %d", code)
	}
	if code, _ := vsh(t, fake{}, "", "ensure", "--name", "../etc"); code != 2 {
		t.Fatal("an account that is not a project name is refused")
	}
}

type recorded struct {
	stdin string
	args  []string
}

func recorder(calls *[]recorded, results map[string]int) runner {
	store := map[string]string{}
	return func(_ context.Context, stdin, name string, args ...string) (string, string, int, error) {
		*calls = append(*calls, recorded{stdin, append([]string{name}, args...)})
		verb := args[0]
		if name == security && verb == "-i" {
			store["k"] = strings.Fields(stdin)[len(strings.Fields(stdin))-1]
			return "", "", 0, nil
		}
		switch verb {
		case "store":
			store["k"] = stdin
		case "find-generic-password", "lookup":
			if v, ok := store["k"]; ok {
				return v + "\n", "", 0, nil
			}
			return "", "", results[verb], nil
		}
		return "", "", 0, nil
	}
}

func TestTheSecretTravelsOnStdinNeverAsAnArgument(t *testing.T) {
	for name, kc := range map[string]func(r runner) Keychain{
		"macOS":          func(r runner) Keychain { return macOS{run: r} },
		"secret service": func(r runner) Keychain { return secretService{run: r} },
	} {
		t.Run(name, func(t *testing.T) {
			var calls []recorded
			k := kc(recorder(&calls, map[string]int{"find-generic-password": 44, "lookup": 1}))
			code, key := vsh(t, k, "", "ensure", "--name", "p1")
			if code != 0 || len(key) != 64 {
				t.Fatalf("ensure: %d %q", code, key)
			}
			sawStdin := false
			for _, c := range calls {
				if slices.ContainsFunc(c.args, func(a string) bool { return strings.Contains(a, key) }) {
					t.Fatalf("the key is in argv: %v", c.args)
				}
				sawStdin = sawStdin || strings.Contains(c.stdin, key)
			}
			if !sawStdin {
				t.Fatal("the key was never written on stdin")
			}
		})
	}
}

func TestTheSecretServiceOfThisSession(t *testing.T) {
	if os.Getenv("CN_SECRET_SERVICE_TEST") != "1" {
		t.Skip("set CN_SECRET_SERVICE_TEST=1 to store, read and delete one throwaway item in the session's Secret Service")
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("no secret-tool")
	}
	b := make([]byte, 4)
	rand.Read(b)
	account := "vsh-test-" + hex.EncodeToString(b)
	kc := secretService{run: execRun}
	t.Cleanup(func() { _ = kc.Delete(ctx, account) })
	code, key := vsh(t, kc, "", "ensure", "--name", account)
	if code != 0 {
		t.Fatalf("ensure: %q", key)
	}
	if got, err := kc.Get(ctx, account); err != nil || got != key {
		t.Fatalf("get: %q %v", got, err)
	}
	if err := kc.Delete(ctx, account); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Get(ctx, account); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

func TestAnotherServiceNeverTouchesTheUnlockKeys(t *testing.T) {
	for name, kc := range map[string]func(r runner) Keychain{
		"macOS":          func(r runner) Keychain { return macOS{run: r, service: "vsh"} },
		"secret service": func(r runner) Keychain { return secretService{run: r, service: "vsh"} },
	} {
		t.Run(name, func(t *testing.T) {
			var calls []recorded
			k := kc(recorder(&calls, nil))
			if err := k.Set(context.Background(), "session-1", "s"); err != nil {
				t.Fatal(err)
			}
			_, _ = k.Get(context.Background(), "session-1")
			_ = k.Delete(context.Background(), "session-1")
			for _, c := range calls {
				all := c.stdin + " " + strings.Join(c.args, " ")
				if strings.Contains(all, Service) || !strings.Contains(all, "vsh") {
					t.Fatalf("a call outside service vsh: %q", all)
				}
			}
		})
	}
}
