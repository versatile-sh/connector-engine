# connector-engine

connector-engine is a Go engine that sits between AI apps and the tool servers they call (MCP, REST, gRPC). You import a server, then decide per agent and per function what may run: off, on, or ask a person first. Every function is pinned by hash, so a changed description or a new tool is quarantined until someone accepts it. Each decision and call is published as a CloudEvent.

## Quick start

Requirements: Docker with Compose 2.20 or newer.

```sh
make up                      # or: make up CONSOLE_PORT=9123
```

1. Open `http://127.0.0.1:7465` and create the owner account.
2. Import an MCP server by URL, then set each function to off, on or ask.
3. Connect an AI app to `http://127.0.0.1:7465/mcp`. The app signs in through your browser (OAuth), so there is no key to copy.

```sh
claude mcp add --transport http versatile http://127.0.0.1:7465/mcp
codex mcp add versatile --url http://127.0.0.1:7465/mcp
```

The console's **Access** page has one-click links for Cursor and VS Code, and the config for Claude Desktop, OpenCode, Antigravity and other clients.

Only the console port is published, on `127.0.0.1`. Stop the stack with `make down`.

> **Your data lives in Docker volumes.** `docker compose down -v`, `docker system prune --volumes` and `docker volume prune -a` delete them, and with them every connector, credential, access key and the root keys. Nothing can bring them back. Export a backup first: **Account → Export** in the console, or `make export`. See [Backups](selfhosted/docs/console.md#backups).

## Profiles

| Profile | For | What runs | Start |
|---|---|---|---|
| `person` (default) | one person on their own machine | Postgres, the engine and the local web console; the console is the OAuth server for AI apps | `make up` |
| `headless` | a product that embeds the engine | Postgres and the engine only; tokens come from your OIDC/JWKS issuer | `make up PROFILE=headless` |

The headless binaries (`cmd/connector-engine`, `cmd/cnctl`) contain no self-hosted code. Everything the `person` profile adds lives in [`selfhosted/`](selfhosted/README.md).

## Common commands

| Command | What it does |
|---|---|
| `make up` / `make down` / `make logs` | start, stop, follow the stack |
| `make unlock` | deliver the unlock key from the host keychain after a restart ([secrets](selfhosted/docs/SECRETS.md)) |
| `make notify` | desktop popups for calls waiting for approval |
| `make export` / `make import FILE=...` | write an encrypted full backup to `~/connector-engine-backups/`, or restore one; `MODE=setup` exports a shareable setup without credentials ([backups](selfhosted/docs/console.md#backups)) |
| `make reset-password` | set a new console password from this machine when it is forgotten |
| `make test` | Go tests, web typecheck, layering check, `adapters check` of both profiles |
| `make check` | scripted end-to-end check of the `person` stack |

## Documentation

| Read | To |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | understand ports and adapters, the request path, grants, approvals and pinning |
| [docs/engine.md](docs/engine.md) | run the engine without Docker and use `cnctl` |
| [selfhosted/docs/console.md](selfhosted/docs/console.md) | use the console: approvals, install links, skills, activity |
| [selfhosted/docs/SECRETS.md](selfhosted/docs/SECRETS.md) | see how root keys are stored and unlocked |
| [selfhosted/docs/approvals-surfaces.md](selfhosted/docs/approvals-surfaces.md) | see how approvals reach chat apps and the desktop |
| [CONTRIBUTING.md](CONTRIBUTING.md) | set up, test and send a change |
| [SECURITY.md](SECURITY.md) | report a vulnerability |

## License

MIT, see [LICENSE](LICENSE).
