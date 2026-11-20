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
| `docs/` | [SECRETS.md](docs/SECRETS.md), [approvals-surfaces.md](docs/approvals-surfaces.md) |
| `selfhosted.mk` | the make targets above, included by the root `Makefile` |

## Run

From the repository root, as before: `make up`, `make unlock`, `make notify`, `make check`, and `make export`, `make import FILE=...`, `make reset-password` for [backups and the account](docs/console.md#backups). The compose project stays `connector-engine`, so the volumes of an existing install are reused. `deploy/compose.yaml` is included through Compose `include`, which needs Docker Compose 2.20 or newer.

## Data and backups

Everything an install knows lives in four Docker volumes of the project:

| Volume | Holds |
|---|---|
| `pgdata` | the engine's database: connectors, grants, skills, sealed credentials, audit |
| `engine-data` | the engine's key store: the wrapped root key, the key ring and the local issuer's signing keys |
| `console-data` | the owner account, access keys and agents, OAuth clients, the console's sealed secrets and key store, activity, settings |
| `secrets` | what `init` writes once per install: the operator token and the Postgres passwords |

`docker compose down -v`, `docker system prune --volumes` and `docker volume prune -a` delete these volumes. That loses everything, and the keychain cannot bring it back, because the keychain holds only the key that unwraps the root keys. Export a backup before you run any of them.

A full backup (`make export`, or Account → Export) is one encrypted file. It holds a consistent snapshot of the engine's database, both key stores with their root keys in plaintext, and the console's state and activity. It opens on a new machine without the old keychain, and the import wraps the root keys again with the new machine's keychain. The `secrets` volume is not in it: a new install writes its own. The format is in [docs/console.md](docs/console.md#backups).

## Run without Docker

One binary, `connector-engine-selfhosted`, runs the engine and the console in one process, on the loopback, over the `builtin:sqlite` store. It needs no Docker, no Postgres and no CGO.

```sh
connector-engine-selfhosted up --open
```

The binary name and the flags below are a stable interface: the installers and the release build rely on them.

| Command | Flags | Does |
| --- | --- | --- |
| `up` | `--data DIR`, `--port 7465`, `--open`, `--no-keychain` | runs the install in the foreground until Ctrl-C, `stop` or SIGTERM; prints the console URL and the agent endpoint (`<console>/mcp`) |
| `stop` | `--data DIR` | asks the running install to stop and waits for it |
| `status` | `--data DIR` | prints the pid, the version, the console URL and the data directory; exits 3 when nothing runs |
| `export` | `--data DIR`, `--out FILE`, `--setup`, `--yes` | writes a full encrypted backup (`.cebak`, asks a passphrase twice) or, with `--setup`, a shareable setup without credentials; the default file is dated, in `$BACKUP_DIR` or `~/connector-engine-backups` |
| `import FILE` | `--data DIR`, `--yes` | shows what the file holds, asks, then restores it; a full backup replaces everything and the process restarts the engine and the console on the restored data |
| `reset-password` | `--data DIR` | sets a new console password and signs every console session out |
| `uninstall` | `--data DIR`, `--keep-data` or `--purge`, `--yes`, `--no-keychain` | stops the install; `--purge` also deletes the data directory and the unlock key in the keychain; the binary itself is removed by its installer or by hand |

`export`, `import` and `reset-password` reach the running process through its control socket, so the install must be up. Every command takes `--data`; the default comes from `$CN_DATA_DIR`, else the per-user data directory of the OS.

### Data directory

| OS | Default |
| --- | --- |
| Linux and the BSDs | `$XDG_DATA_HOME/connector-engine`, else `~/.local/share/connector-engine` |
| macOS | `~/Library/Application Support/connector-engine` |
| Windows | `%AppData%\connector-engine` |

The directory and its subdirectories are created with mode 0700:

| Path | Holds |
| --- | --- |
| `engine/store.db` (and `-wal`, `-shm`) | the engine's database: SQLite in WAL mode, `synchronous=FULL`, a 5 s busy timeout |
| `engine/secrets/` | the engine's key store: the root key, the key ring and the local issuer's signing keys |
| `console/` | the owner account, access keys and agents, OAuth clients, activity, settings, and the console's key store in `console/secrets/` |
| `operator-token` | the token the console uses on the engine's operator listener, written once with mode 0600 |
| `control.sock` | the control socket of the running process, mode 0600; it also refuses a second `up` on the same directory |

### Network

The console listens on `127.0.0.1:7465` (`--port`). The engine's public and operator listeners take free ports on `127.0.0.1`, because only the console talks to them; nothing listens on another interface. The settings are those of the person profile (`deploy/profiles/person`), with the listeners on the loopback, the key store paths in the data directory, no unlock socket and no `host.docker.internal`.

### Keychain

The unlock key lives in the OS keychain (macOS Keychain, Secret Service on Linux, DPAPI on Windows) under the service `connector-engine` and the account `local-<12 hex digits of the data directory's hash>`, so it never collides with a Docker install's account. The process reads it directly at start: no socket, no `docker exec`. A fresh root key is created wrapped; a plaintext root key left by an earlier run without a keychain is wrapped on the first start that finds one. Without a keychain (or with `--no-keychain`) the root keys stay plaintext files and the console shows the same warning as the Docker install; a wrapped root key never falls back to plaintext.

### Move a Docker install here

1. On the Docker install: `make export` (or Account, Export). Keep the `.cebak` file and its passphrase.
2. `make down`: both installs use port 7465, and `down` keeps the Docker volumes.
3. Start the new install: `connector-engine-selfhosted up`.
4. In another terminal: `connector-engine-selfhosted import ~/connector-engine-backups/connector-engine-backup-<stamp>.cebak`, and type the passphrase.

The backup carries the engine's Postgres rows, both key stores in plaintext and the console's state. The SQLite store restores Postgres snapshots as they are, the root keys are wrapped again by this machine's keychain, and the access keys the agents already hold keep working. The way back (SQLite to Postgres) is not supported yet. `selfhosted/standalone/migrate_test.go` runs this path against a throwaway Postgres.

### Run it as a service

The binary does not install a service yet. `up` stays in the foreground and stops cleanly on SIGTERM, so a systemd user unit (`ExecStart=/path/to/connector-engine-selfhosted up`, `Restart=on-failure`), a launchd agent (`ProgramArguments` with `up`, `KeepAlive`) or a Windows scheduled task at logon run it as is.

### Build

```sh
(cd selfhosted/console/web && npm ci && npm run build)
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o connector-engine-selfhosted ./selfhosted/cmd/connector-engine
```

The frontend must be built first: `go:embed` takes `selfhosted/console/web/dist`, and an empty `dist` serves no console. The build is pure Go on linux, darwin and windows, amd64 and arm64.
