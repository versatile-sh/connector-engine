# selfhosted

Everything the `person` profile adds to the core engine: the local console, the approval surfaces, the `hostkey` secrets and `sqlite` store adapters, the `cnctl keychain` and `cnctl notify` commands, their binaries, the compose files and scripts that run them on a person's own machine, and the install without Docker that runs them as one binary.

The dependency direction is one way: `selfhosted/` imports the core, and no core package imports `selfhosted/`. `scripts/check-layering.sh` (part of `make test`) enforces it, test imports included. Layout and rationale: [docs/internal/SELFHOSTED-LAYOUT.md](../docs/internal/SELFHOSTED-LAYOUT.md).

## Composition

`selfhosted.go` is the one list of what this build adds:

| Function | Adds | Used by |
| --- | --- | --- |
| `Register(r *app.Registry)` | the `builtin:hostkey` secrets adapter and its checks of `connector-engine.yaml`, and the `builtin:sqlite` store (setting `path`) | `cmd/connector-engine` |
| `EngineCommands()` | `connector-engine-selfhosted unlock` | `cmd/connector-engine` |
| `Commands()` | `cnctl keychain`, `cnctl notify` | `cmd/cnctl` |
| `standalone.Commands(version)` | `up`, `stop`, `status`, `export`, `import`, `reset-password`, `uninstall` of the install without Docker | `cmd/connector-engine` |
| `PersonProfile()` | the person profile's `connector-engine.yaml` and `adapters.yaml`, embedded | `standalone` |

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
| `secrets/hostkey` | root keys wrapped by an unlock key from the host's keychain, delivered over the unlock socket or read in-process |
| `standalone/` | the install without Docker: data directory, one-process supervisor, control socket, lifecycle commands |
| `cli/keychain`, `cli/notify` | the host keychain and the host notification helper |
| `deploy/compose.yaml` | includes `../../deploy/compose.yaml` (init, Postgres) and adds `engine` and `console` |
| `deploy/profiles/person`, `deploy/check`, `deploy/compose.check.yaml` | the profile and the override `make check` runs |
| `scripts/` | `console-check.sh` (`make check`), `keychain.sh` (`make unlock`, `make keychain-*`) |
| `selfhosted.mk` | the make targets above, included by the root `Makefile` |

## Run

From the repository root, as before: `make up`, `make unlock`, `make notify`, `make check`, and `make export`, `make import FILE=...`, `make reset-password` for [backups and the account](../docs/using/account-backup.md). The compose project stays `connector-engine`, so the volumes of an existing install are reused. `deploy/compose.yaml` is included through Compose `include`, which needs Docker Compose 2.20 or newer.

## Data and backups

Everything an install knows lives in four Docker volumes of the project:

| Volume | Holds |
|---|---|
| `pgdata` | the engine's database: connectors, grants, skills, sealed credentials, audit |
| `engine-data` | the engine's key store: the wrapped root key, the key ring and the local issuer's signing keys |
| `console-data` | the owner account, access keys and agents, OAuth clients, the console's sealed secrets and key store, activity, settings |
| `secrets` | what `init` writes once per install: the operator token and the Postgres passwords |

`docker compose down -v`, `docker system prune --volumes` and `docker volume prune -a` delete these volumes. That loses everything, and the keychain cannot bring it back, because the keychain holds only the key that unwraps the root keys. Export a backup before you run any of them.

A full backup (`make export`, or Account → Export) is one encrypted file. It holds a consistent snapshot of the engine's database, both key stores with their root keys in plaintext, and the console's state and activity. It opens on a new machine without the old keychain, and the import wraps the root keys again with the new machine's keychain. The `secrets` volume is not in it: a new install writes its own. The format is in [account and backup](../docs/using/account-backup.md#full-backups). User documentation: [the person profile](../docs/self-hosted/person.md).

## Run without Docker

`standalone/` runs the engine and the console as one binary, `connector-engine-selfhosted`, over the `builtin:sqlite` store, with no Docker, no Postgres and no CGO. Its commands, data directory, keychain and the move from a Docker install are documented in [the person profile](../docs/self-hosted/person.md#without-docker). The binary name and its flags are a stable interface: the installers and the release build rely on them.

### Build

```sh
(cd selfhosted/console/web && npm ci && npm run build)
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o connector-engine-selfhosted ./selfhosted/cmd/connector-engine
```

The frontend must be built first: `go:embed` takes `selfhosted/console/web/dist`, and an empty `dist` serves no console. The build is pure Go on linux, darwin and windows, amd64 and arm64.
