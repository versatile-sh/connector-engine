// Package selfhosted lists what the self-hosted build adds to the core: the hostkey secrets adapter, the unlock command of the engine, and the keychain and notify commands of cnctl.
package selfhosted

import (
	"context"
	"io"
	"os"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/cli"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/notify"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

// Register adds the self-hosted adapters to r.
func Register(r *app.Registry) {
	app.Register(r, app.Secrets, app.Adapter{Name: "hostkey", Capabilities: []string{"secrets.envelope", "secrets.crypto_shredding"}, Check: checkHostkey}, openHostkey)
}

// EngineCommands returns the subcommands the self-hosted engine binary adds: unlock.
func EngineCommands() map[string]app.Command {
	return map[string]app.Command{
		"unlock": func(cfg config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
			return hostkey.Main(args, cfg.Secrets.UnlockSocket, stdin, stdout, stderr)
		},
	}
}

// Commands returns the subcommands the self-hosted cnctl adds: keychain and notify.
func Commands() map[string]cli.Command {
	return map[string]cli.Command{
		"keychain": func(ctx context.Context, args []string, stdout, stderr io.Writer, _ []string) int {
			return keychain.Run(ctx, keychain.System(), args, os.Stdin, stdout, stderr)
		},
		"notify": func(ctx context.Context, args []string, _, stderr io.Writer, environ []string) int {
			return notify.Command(ctx, args, cli.Getenv(environ), stderr)
		},
	}
}

func openHostkey(ctx context.Context, env app.Env) (port.SecretStore, func(), error) {
	hk, err := hostkey.Open(ctx, env.Config.Secrets.Dir, env.Config.Secrets.UnlockSocket, env.Log)
	if err != nil {
		return nil, nil, err
	}
	return hk, hk.Close, nil
}

func checkHostkey(c config.Config, acknowledged func(string) bool) config.Findings {
	var fs config.Findings
	if c.Secrets.UnlockSocket == "" {
		fs = append(fs, config.Finding{Level: config.LevelError, Where: "secrets.unlockSocket", Message: "the hostkey adapter waits for the unlock key on this socket; it is empty"})
	}
	if !acknowledged(config.AckPlaintextRootKey) {
		fs = append(fs, config.Finding{Level: c.RefusalLevel(), Where: "port secrets",
			Message: "hostkey keeps a plaintext root key on a host without a keychain and until it is migrated; acknowledge " + config.AckPlaintextRootKey + " to run with it"})
	}
	return fs
}
