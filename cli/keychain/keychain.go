// Package keychain keeps the unlock key of a local install in the host's OS keychain; it is the cnctl keychain command.
package keychain

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"

	"github.com/agents-control/connector-engine/secrets/local"
)

// Service is the keychain service every unlock key is stored under; the account is the install's compose project.
const Service = "connector-engine"

// ErrNotFound is returned when the keychain has no item for the account.
var ErrNotFound = errors.New("no unlock key in the keychain for this install")

// ErrUnavailable is returned when this host has no usable OS keychain.
var ErrUnavailable = errors.New("no OS keychain on this host")

// Keychain stores one secret per account under Service.
type Keychain interface {
	Get(ctx context.Context, account string) (string, error)
	Set(ctx context.Context, account, secret string) error
	Delete(ctx context.Context, account string) error
}

// ExitUnavailable is the exit code of Run when the host has no keychain, so scripts can fall back to the plaintext file.
const ExitUnavailable = 3

var accountName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)

// Run is cnctl keychain get|ensure|import|delete --name NAME: ensure creates the key once, get and ensure print it, import reads it on stdin.
func Run(ctx context.Context, kc Keychain, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: cnctl keychain get|ensure|import|delete --name NAME")
		return 2
	}
	fs := flag.NewFlagSet("keychain "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "connector-engine", "the install, its compose project name")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if !accountName.MatchString(*name) {
		fmt.Fprintln(stderr, "cnctl keychain: --name is a compose project name")
		return 2
	}
	key, err := run(ctx, kc, args[0], *name, stdin)
	switch {
	case errors.Is(err, ErrUnavailable):
		fmt.Fprintln(stderr, "cnctl keychain:", err)
		return ExitUnavailable
	case err != nil:
		fmt.Fprintln(stderr, "cnctl keychain:", err)
		return 1
	case key != "":
		fmt.Fprintln(stdout, key)
	}
	return 0
}

func run(ctx context.Context, kc Keychain, verb, name string, stdin io.Reader) (string, error) {
	if kc == nil {
		return "", ErrUnavailable
	}
	switch verb {
	case "get":
		return kc.Get(ctx, name)
	case "ensure":
		key, err := kc.Get(ctx, name)
		if !errors.Is(err, ErrNotFound) {
			return key, err
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		return store(ctx, kc, name, hex.EncodeToString(b))
	case "import":
		line, err := bufio.NewReader(io.LimitReader(stdin, 256)).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		b, err := local.ParseUnlockKey(line)
		if err != nil {
			return "", err
		}
		key := hex.EncodeToString(b)
		switch old, err := kc.Get(ctx, name); {
		case err == nil && old != key:
			return "", errors.New("the keychain already holds another unlock key for this install; delete it first")
		case err == nil:
			return "", nil
		case !errors.Is(err, ErrNotFound):
			return "", err
		}
		_, err = store(ctx, kc, name, key)
		return "", err
	case "delete":
		return "", kc.Delete(ctx, name)
	}
	return "", fmt.Errorf("unknown verb %q", verb)
}

func store(ctx context.Context, kc Keychain, name, key string) (string, error) {
	if err := kc.Set(ctx, name, key); err != nil {
		return "", err
	}
	back, err := kc.Get(ctx, name)
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare([]byte(back), []byte(key)) != 1 {
		return "", errors.New("the keychain returned another value than the one stored")
	}
	return key, nil
}
