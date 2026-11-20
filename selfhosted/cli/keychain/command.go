package keychain

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"os/exec"
	"strings"
)

type runner func(ctx context.Context, stdin, name string, args ...string) (stdout, stderr string, code int, err error)

func execRun(ctx context.Context, stdin, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), errOut.String(), exit.ExitCode(), nil
	}
	return out.String(), errOut.String(), 0, err
}

func failed(what, stderr string) error {
	return errors.New(what + ": " + strings.TrimSpace(stderr))
}

type macOS struct {
	run     runner
	service string
}

func service(s string) string { return cmp.Or(s, Service) }

func label(s, account string) string {
	if service(s) == Service {
		return "connector-engine unlock key (" + account + ")"
	}
	return s + " (" + account + ")"
}

const security = "/usr/bin/security"

func (m macOS) Get(ctx context.Context, account string) (string, error) {
	out, errOut, code, err := m.run(ctx, "", security, "find-generic-password", "-s", service(m.service), "-a", account, "-w")
	switch {
	case err != nil:
		return "", err
	case code == 44:
		return "", ErrNotFound
	case code != 0:
		return "", failed("security find-generic-password", errOut)
	}
	return strings.TrimSpace(out), nil
}

func (m macOS) Set(ctx context.Context, account, secret string) error {
	_, errOut, code, err := m.run(ctx, "add-generic-password -U -s "+service(m.service)+" -a "+account+" -w "+secret+"\n", security, "-i")
	if err == nil && code != 0 {
		err = failed("security add-generic-password", errOut)
	}
	return err
}

func (m macOS) Delete(ctx context.Context, account string) error {
	_, errOut, code, err := m.run(ctx, "", security, "delete-generic-password", "-s", service(m.service), "-a", account)
	if err == nil && code != 0 && code != 44 {
		err = failed("security delete-generic-password", errOut)
	}
	return err
}

type secretService struct {
	run     runner
	service string
}

func (s secretService) Get(ctx context.Context, account string) (string, error) {
	out, errOut, code, err := s.run(ctx, "", "secret-tool", "lookup", "service", service(s.service), "account", account)
	switch {
	case err != nil:
		return "", err
	case code != 0 && strings.TrimSpace(errOut) == "":
		return "", ErrNotFound
	case code != 0:
		return "", errors.Join(ErrUnavailable, failed("secret-tool lookup", errOut))
	}
	return strings.TrimSpace(out), nil
}

func (s secretService) Set(ctx context.Context, account, secret string) error {
	_, errOut, code, err := s.run(ctx, secret, "secret-tool", "store", "--label", label(s.service, account), "service", service(s.service), "account", account)
	if err == nil && code != 0 {
		err = failed("secret-tool store", errOut)
	}
	return err
}

func (s secretService) Delete(ctx context.Context, account string) error {
	_, errOut, code, err := s.run(ctx, "", "secret-tool", "clear", "service", service(s.service), "account", account)
	if err == nil && code != 0 {
		err = failed("secret-tool clear", errOut)
	}
	return err
}
