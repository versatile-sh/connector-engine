package standalone

import (
	"context"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

const loopbackAny = "127.0.0.1:0"

func registry(kc keychain.Keychain, account string) *app.Registry {
	r := app.Core()
	selfhosted.Register(r)
	app.Register(r, app.Secrets, app.Adapter{Name: "keychain", Capabilities: []string{"secrets.envelope", "secrets.crypto_shredding"}, Check: checkKeychain},
		func(ctx context.Context, env app.Env) (port.SecretStore, func(), error) {
			s, err := hostkey.Unlock(ctx, env.Config.Secrets.Dir, kc, account, env.Log)
			if err != nil {
				return nil, nil, err
			}
			return s, s.Close, nil
		})
	return r
}

func checkKeychain(c config.Config, acknowledged func(string) bool) config.Findings {
	if acknowledged(config.AckPlaintextRootKey) {
		return nil
	}
	return config.Findings{{Level: c.RefusalLevel(), Where: "port secrets", Message: "keychain keeps a plaintext root key on a host without a keychain; acknowledge " + config.AckPlaintextRootKey + " to run with it"}}
}

func engineProfile(p paths, r *app.Registry) (config.Config, adapters.File, error) {
	engineYAML, adaptersYAML := selfhosted.PersonProfile()
	cfg, err := config.Parse(engineYAML)
	if err != nil {
		return cfg, adapters.File{}, err
	}
	cfg.Server.Listen, cfg.Server.OperatorListen, cfg.Server.GRPCListen = loopbackAny, loopbackAny, ""
	cfg.Secrets.Dir, cfg.Secrets.UnlockSocket = p.engineSecrets, ""
	cfg.Runtime.LoopbackHost = ""
	cfg.Auth.Operator.TokenRef = "file:" + p.operatorFile
	cfg = cfg.With(r.Builtins()...)
	file, err := adapters.Load(cfg.AdaptersSpec(), adaptersYAML, nil)
	if err != nil {
		return cfg, file, err
	}
	file.Ports[config.PortStore] = adapters.Port{Use: adapters.Uses{"builtin:sqlite"}, Config: map[string]any{"path": p.store}}
	file.Ports[config.PortSecrets] = adapters.Port{Use: adapters.Uses{"builtin:keychain"}}
	return cfg, file, nil
}

func (in *Install) unlocker(dir string) func(context.Context) (port.SecretStore, error) {
	return func(ctx context.Context) (port.SecretStore, error) {
		return hostkey.Unlock(ctx, dir, in.o.Keychain, in.paths.account, in.o.Log)
	}
}
