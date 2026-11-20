// Package selfhosted lists what the self-hosted build adds to the core: the hostkey secrets and sqlite store adapters, the unlock command of the engine, and the keychain and notify commands of vsh.
package selfhosted

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/notify"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
	"github.com/versatile-sh/connector-engine/store/sqlite"
)

//go:embed deploy/profiles/person/connector-engine.yaml deploy/profiles/person/adapters.yaml
var person embed.FS

// PersonProfile returns the connector-engine.yaml and the adapters.yaml of the person profile.
func PersonProfile() (engine, adapters []byte) {
	engine, _ = person.ReadFile("deploy/profiles/person/connector-engine.yaml")
	adapters, _ = person.ReadFile("deploy/profiles/person/adapters.yaml")
	return engine, adapters
}

// Register adds the self-hosted adapters to r: the hostkey secrets and the embedded sqlite store.
func Register(r *app.Registry) {
	app.Register(r, app.Stores, app.Adapter{Name: "sqlite", Capabilities: []string{"store.transactional_outbox"},
		Settings: map[string]adapters.ConfigKind{"path": adapters.ConfigString}}, openSQLite)
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

// Commands returns the leaves the self-hosted vsh adds to the core: keychain and notify.
func Commands() []*command.Command {
	return []*command.Command{keychainCommand(), notify.Command()}
}

func keychainCommand() *command.Command {
	verb := func(name, summary string) *command.Command {
		return &command.Command{Name: name, Summary: summary, Setup: func(fs *flag.FlagSet) command.Runner {
			install := fs.String("name", "connector-engine", "the install, its compose project name")
			return func(ctx context.Context, x *command.Exec) error {
				if len(x.Args) > 0 {
					return command.Usage(x.Path, "unexpected argument %q", x.Args[0])
				}
				if code := keychain.Run(ctx, keychain.System(), []string{name, "--name", *install}, x.Stdin, x.Stdout, x.Stderr); code != 0 {
					return command.Silent(code)
				}
				return nil
			}
		}}
	}
	return &command.Command{Name: "keychain", Summary: "Keep the unlock key of a Docker install in the OS keychain.", Subs: []*command.Command{
		verb("get", "Print the unlock key."), verb("ensure", "Create the unlock key once and print it."),
		verb("import", "Store the unlock key read on stdin."), verb("delete", "Delete the unlock key."),
	}}
}

func openSQLite(ctx context.Context, env app.Env) (port.Store, func(), error) {
	path := env.Setting("path")
	if path == "" {
		return nil, nil, errors.New("store: the sqlite adapter needs a path")
	}
	s, err := sqlite.Open(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("store: %w", err)
	}
	return s, s.Close, nil
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
