package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/versatile-sh/connector-engine/config"
)

func secretHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeOnce(path, value string, mode os.FileMode) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(value); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

func initSecrets(_ config.Config, args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "/run/connector-engine", "directory of the secrets volume")
	dbHost := fs.String("db-host", "postgres:5432", "host:port of the Postgres service")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	appPassword := secretHex(24)
	files := []struct {
		name, value string
		mode        os.FileMode
	}{
		{"operator-token", secretHex(32), 0o600},
		{"pg-admin-password", secretHex(24), 0o644},
		{"pg-app-password", appPassword, 0o644},
		{"database-url", "", 0o600},
	}
	for _, f := range files {
		value := f.value
		if f.name == "database-url" {
			raw, err := os.ReadFile(filepath.Join(*dir, "pg-app-password"))
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			value = (&url.URL{Scheme: "postgres", User: url.UserPassword("cn", strings.TrimSpace(string(raw))), Host: *dbHost, Path: "/connector_engine", RawQuery: "sslmode=disable"}).String()
		}
		made, err := writeOnce(filepath.Join(*dir, f.name), value, f.mode)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if made {
			fmt.Fprintf(stdout, "wrote %s\n", f.name)
		}
	}
	return 0
}
