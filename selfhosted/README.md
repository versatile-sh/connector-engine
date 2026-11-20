# selfhosted

Everything the `person` profile adds to the core engine: the local console, the approval surfaces, the `hostkey` secrets adapter, the `cnctl keychain` and `cnctl notify` commands, their binaries, and the compose files and scripts that run them on a person's own machine.

The dependency direction is one way: `selfhosted/` imports the core, and no core package imports `selfhosted/`. `scripts/check-layering.sh` (part of `make test`) enforces it, test imports included. Layout and rationale: [docs/SELFHOSTED-LAYOUT.md](../docs/SELFHOSTED-LAYOUT.md).

## Composition

`selfhosted.go` is the one list of what this build adds:

| Function | Adds | Used by |
| --- | --- | --- |
| `Register(r *app.Registry)` | the `builtin:hostkey` secrets adapter and its checks of `connector-engine.yaml` | `cmd/connector-engine` |
| `EngineCommands()` | `connector-engine-selfhosted unlock` | `cmd/connector-engine` |
| `Commands()` | `cnctl keychain`, `cnctl notify` | `cmd/cnctl` |

A main composes the core with them, in two lines:

```go
adapters := app.Core()
selfhosted.Register(adapters)
```

A third-party adapter works the same way: one package with a `Register(r *app.Registry)` that calls `app.Register(r, app.Secrets, app.Adapter{Name: ...}, open)`, and one line in its own main.

## Tree

| Path | What |
| --- | --- |
| `cmd/connector-engine` | the engine of a person install, shipped in the image as `connector-engine-selfhosted` |
| `cmd/cnctl` | `cnctl` with the core commands plus `keychain` and `notify` |
| `cmd/connector-console` | the console server |
| `console/`, `console/sealed`, `console/web` | the console backend, its sealed secrets, the frontend |
| `surface/` | approval channels: desktop, Discord, Slack, Telegram, webhook |
| `secrets/hostkey` | root keys wrapped by an unlock key from the host's keychain |
| `cli/keychain`, `cli/notify` | the host keychain and the host notification helper |
| `deploy/compose.yaml` | includes `../../deploy/compose.yaml` (init, Postgres) and adds `engine` and `console` |
| `deploy/profiles/person`, `deploy/check`, `deploy/compose.check.yaml` | the profile and the override `make check` runs |
| `scripts/` | `console-check.sh` (`make check`), `keychain.sh` (`make unlock`, `make keychain-*`) |
| `docs/` | [SECRETS.md](docs/SECRETS.md), [approvals-surfaces.md](docs/approvals-surfaces.md) |
| `selfhosted.mk` | the make targets above, included by the root `Makefile` |

## Run

From the repository root, as before: `make up`, `make unlock`, `make notify`, `make check`. The compose project stays `connector-engine`, so the volumes of an existing install are reused. `deploy/compose.yaml` is included through Compose `include`, which needs Docker Compose 2.20 or newer.
