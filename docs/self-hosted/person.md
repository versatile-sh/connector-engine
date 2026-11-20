![The person profile](../assets/section-person.svg)

# The person profile

The `person` profile runs connector-engine for one person on their own machine. It adds a local web console, which is also the OAuth server for your AI apps, approval surfaces, and a keychain for the root keys.

## What runs

| Service | Image entrypoint | Does |
|---|---|---|
| `init` | `connector-engine init` | writes the operator token and the Postgres passwords once, then exits |
| `postgres` | `postgres:16-alpine` | the engine's database |
| `engine` | `connector-engine-selfhosted` | the engine with the `hostkey` secrets adapter |
| `console` | `connector-console serve` | the web console on `127.0.0.1:<CONSOLE_PORT>` |

Only the console port is published, on `127.0.0.1`. The compose project is `connector-engine`; `PROJECT=<name>` with its own `CONSOLE_PORT` runs a second, separate install.

## Commands

| Command | What it does |
|---|---|
| `make up` / `make down` / `make logs` / `make ps` | start, stop, follow, list the stack |
| `make unlock` | deliver the unlock key from the host keychain after a restart |
| `make notify` | desktop popups for calls waiting for approval |
| `make export` / `make import FILE=...` | full backup or shareable setup, and restore ([account and backup](../using/account-backup.md)) |
| `make reset-password` | set a new console password from this machine |
| `make cli ARGS="tools list"` | `cnctl` from the image against the console's `/mcp`; set `CN_AGENT_TOKEN` |
| `make admin ARGS="tenants create t2"` | `cnctl admin` against the operator listener with the generated operator token |
| `make keychain-status` / `keychain-migrate` / `keychain-revert` | report, move a plaintext root key under the keychain, or back ([keychain](keychain.md)) |
| `make keychain-recovery` / `keychain-import` | print the unlock key for a password manager, or restore it from stdin |

## Without Docker

One binary, `connector-engine-selfhosted`, runs the engine and the console in one process on `127.0.0.1`, over an embedded SQLite store. It needs no Docker, no Postgres and no Go. The installers that put it on your machine are available after the first release (v0.1.0); until then, [build it from source](../../selfhosted/README.md#build).

```sh
connector-engine-selfhosted up --open
```

| Command | Flags | Does |
|---|---|---|
| `up` | `--data DIR`, `--port 7465`, `--open`, `--no-keychain` | runs in the foreground until Ctrl-C, `stop` or SIGTERM; prints the console URL and the agent endpoint (`<console>/mcp`) |
| `stop` | `--data DIR` | asks the running install to stop and waits |
| `status` | `--data DIR` | prints the pid, version, console URL and data directory; exits 3 when nothing runs |
| `export` | `--data DIR`, `--out FILE`, `--setup`, `--yes` | a full encrypted backup (`.cebak`, asks a passphrase twice), or a shareable setup with `--setup`; the default file is dated, in `$BACKUP_DIR` or `~/connector-engine-backups` |
| `import FILE` | `--data DIR`, `--yes` | shows what the file holds, asks, then restores it; a full backup replaces everything and restarts the engine and the console on the restored data |
| `reset-password` | `--data DIR` | sets a new console password and signs every session out |
| `uninstall` | `--data DIR`, `--keep-data` or `--purge`, `--yes`, `--no-keychain` | stops the install; `--purge` also deletes the data directory (only one that holds an install) and the unlock key |

`export`, `import` and `reset-password` reach the running process through its control socket, so the install must be up.

### Data directory

`--data`, else `$CN_DATA_DIR`, else:

| OS | Default |
|---|---|
| Linux and the BSDs | `$XDG_DATA_HOME/connector-engine`, else `~/.local/share/connector-engine` |
| macOS | `~/Library/Application Support/connector-engine` |
| Windows | `%AppData%\connector-engine` |

It is created with mode 0700, and `up` refuses one that group or others can open.

| Path | Holds |
|---|---|
| `engine/store.db` (and `-wal`, `-shm`) | the engine's database: SQLite in WAL mode, `synchronous=FULL`, a 5 s busy timeout |
| `engine/secrets/` | the engine's key store: the root key, the key ring and the local issuer's signing keys |
| `console/` | the owner account, access keys and agents, OAuth clients, activity, settings, and the console's key store in `console/secrets/` |
| `operator-token` | the token the console uses on the engine's operator listener, mode 0600 |
| `control.sock` | the control socket of the running process, mode 0600; it also refuses a second `up` on the same directory |

### Network and keychain

- The console listens on `127.0.0.1:7465` (`--port`). The engine's listeners take free ports on `127.0.0.1`; nothing listens on another interface.
- The settings are those of the person profile, with the listeners on the loopback, the key stores in the data directory, no unlock socket and no `host.docker.internal`.
- The unlock key is read from the OS keychain at start: no `make unlock`, no locked state after a restart. Its account is `local-` and 12 hex digits of the data directory's hash, so it never meets a Docker install's item ([keychain](keychain.md#without-docker)).
- Without a keychain (or with `--no-keychain`) the root keys stay plaintext files and the console warns; a wrapped root key never falls back to plaintext.

### Move a Docker install here

1. On the Docker install: `make export`. Keep the `.cebak` file and its passphrase.
2. `make down`: both installs use port 7465, and `down` keeps the Docker volumes.
3. `connector-engine-selfhosted up`.
4. In another terminal: `connector-engine-selfhosted import ~/connector-engine-backups/connector-engine-backup-<stamp>.cebak`, and type the passphrase.

The root keys are wrapped again by this machine's keychain, and the access keys your apps already hold keep working. The way back (SQLite to Postgres) is not supported yet.

### Run it as a service

`up` stays in the foreground and stops cleanly on SIGTERM. Point a systemd user unit (`ExecStart=/path/to/connector-engine-selfhosted up`, `Restart=on-failure`), a launchd agent (`ProgramArguments` with `up`, `KeepAlive`) or a Windows scheduled task at logon at it.

## Port and host

- `CONSOLE_PORT` (default 7465) is the only port setting. Put `CONSOLE_PORT=9123` in `selfhosted/deploy/.env` to keep it.
- It sets the published port, the Host allowlist (`127.0.0.1`, `localhost` and `[::1]` at that port) and the OAuth redirect `http://127.0.0.1:<port>/oauth/callback`.

Outside Docker, run the console against an engine you started yourself:

```sh
connector-console --listen 127.0.0.1:7465 --engine http://127.0.0.1:8080 \
  --engine-operator http://127.0.0.1:8081 --operator-token-file <file> --state <file>
```

`connector-console --help` lists every flag.

## Profile settings

The profile is a pair of files in `selfhosted/deploy/profiles/person/`. What it sets beyond the engine defaults:

| Setting | Value | Why |
|---|---|---|
| `auth.localIssuer` | enabled | the console mints agent tokens for your apps |
| `secrets` port | `builtin:hostkey` | root keys wrapped by a key in your OS keychain |
| `runtime.allowPrivateHosts` | `true` | you can import servers on your own network or `localhost` |
| `runtime.loopbackHost` | `host.docker.internal` | a server on the host's loopback is reachable from the container |
| `gateway.skills` | `tools+prompts` | skills are tools and slash-command prompts |
| `gateway.relayInstructions`, `gateway.upstreamContent` | `pinned`, `granted` | server instructions, prompts and resources are offered, pinned |
| `server.snapshot` | `true` | the console can take full backups |
| `retention` | events and idempotency keys 7 days, audit and calls 90 days | the history stays small |

## Console secrets

| Item | Where |
|---|---|
| operator token, Postgres passwords | the `secrets` volume, written on first boot |
| owner account (PBKDF2-SHA256) | the `console-data` volume |
| notifier tokens and other console secrets | sealed (envelope encryption, keys in `console-data/secrets`); the state file holds only sealed records |
| the key that signs approval links | sealed the same way; while the console is locked, a link answers 503 until `make unlock` |
| engine and console root keys | wrapped by an unlock key in the OS keychain ([keychain](keychain.md)) |

The console's design system (tokens, typography, components) is in [`selfhosted/console/web/DESIGN.md`](../../selfhosted/console/web/DESIGN.md). Code layout: [`selfhosted/README.md`](../../selfhosted/README.md).
