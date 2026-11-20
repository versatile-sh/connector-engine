# connector-engine

A neutral Go engine that lets a tenant plug tool servers (MCP, REST, gRPC) into its agents and decide, per agent and per function, what each one may call. It imports listings from a catalog, pins every function by hash, evaluates a four-layer grant table, holds a call for a person's approval when the rule says ask, and publishes CloudEvents. The specification is `adaptable-research-engine/studies/connector-engine/03-engine.md`; the build plan is [docs/PLAN.md](docs/PLAN.md) and what runs today is [docs/STATUS.md](docs/STATUS.md).

## Quick start (Docker)

```
make up                      # or: make up CONSOLE_PORT=9123
```

This builds one image (engine, `cnctl`, console) and starts the `person` profile of `deploy/compose.yaml`: Postgres, the engine and the console. Open `http://127.0.0.1:7465` (or your port), create the owner account, import an MCP server by URL, turn each function off, on or ask, then connect an AI app with the address alone: `claude mcp add --transport http versatile http://127.0.0.1:<port>/mcp`, `codex mcp add versatile --url http://127.0.0.1:<port>/mcp`, the one-click links for Cursor and VS Code, or the config shown for Claude Desktop, OpenCode, Antigravity and Pi (through the community pi-mcp-adapter) under "Access". Antigravity returns its code through its own page at `https://antigravity.google/oauth-callback`, the one https redirect the console accepts, listed exactly; the code alone is useless without the app's PKCE verifier. The app follows the MCP authorization spec: it registers itself, opens the console in your browser, you allow it once, and it gets a token and a refresh token, with no key to copy. Clients without OAuth (your own scripts, `cnctl`) use an access key made on the same page; the console shows the exact command or config for each one. Opening `/mcp` in a browser leads to that page. Only that port is published, on 127.0.0.1; the engine's listeners stay inside the compose network.

| Command | What it does |
|---|---|
| `make up` / `make down` / `make logs` | start, stop, follow the `person` profile; `PROFILE=headless` runs the engine alone. With the `person` profile, `make up` ends with `make unlock` |
| `make unlock` | deliver the unlock key from the host's keychain to the engine and the console after any restart ([docs/SECRETS.md](docs/SECRETS.md)) |
| `make keychain-migrate` / `keychain-revert` / `keychain-status` | move an existing plaintext `root.key` under the keychain (verified, idempotent), back again, or report |
| `make keychain-recovery` / `keychain-import` | print the unlock key for a password manager, or restore it into this host's keychain from stdin |
| `make cli ARGS="tools list"` | `cnctl` from the image against the console's `/mcp`; set `CN_AGENT_TOKEN` |
| `make admin ARGS="tenants create t2"` | `cnctl admin` against the operator listener with the generated operator token |
| `make test` | Go tests, the console typecheck and `adapters check` of both profiles |
| `make check` | the scripted end-to-end check in `scripts/console-check.sh` (builds, runs, cleans up) |
| `make notify` | `cnctl notify` on the host: desktop popups for held calls, with the device key paired in the console's Notifications page |

- Approvals can reach you outside the console: in the AI app itself (MCP elicitation, per app), as a desktop popup (`make notify`), or in Discord, Slack or Telegram with buttons. Set them up under Notifications; nothing is sent until you add one. Design and threat model: [docs/approvals-surfaces.md](docs/approvals-surfaces.md).
- `CONSOLE_PORT` is the only port setting. It sets the published port, the Host allowlist (`127.0.0.1`, `localhost` and `[::1]` at that port) and the OAuth redirect `http://127.0.0.1:<port>/oauth/callback`. Put it in `deploy/.env` to keep it.
- Outside Docker: `connector-console --listen 127.0.0.1:<port> --engine http://127.0.0.1:8080 --engine-operator http://127.0.0.1:8081 --operator-token-file <file> --state <file>`; the allowlist follows the port of `--listen`.
- Install links: `http://127.0.0.1:<port>/import?listing=<https URL of a listing in the Manifest format>` or `?mcp=<https MCP URL>` open a confirm screen (after sign-in) with the origin, title, icon, publisher and each function's proposed choice. Nothing is installed before the click.
- Root keys: in the `person` profile the engine's and the console's root keys rest wrapped by an unlock key kept in the OS keychain (macOS Keychain, Secret Service, DPAPI) and piped into each container by `make unlock`; without a keychain they stay plaintext files and the console warns. `PROJECT` names the compose project and the keychain item. See [docs/SECRETS.md](docs/SECRETS.md) for the delivery, the migration and the threat model.
- First boot writes a random operator token and the Postgres passwords into the `secrets` volume. The console keeps the owner account (PBKDF2-SHA256) in the `console-data` volume. Secrets the console keeps for itself, such as notifier tokens, go through `console/sealed`: a `port.SecretStore` (envelope encryption, key store in `console-data/secrets`) seals each value, so the state file holds only `port.Sealed` records; passwords and tokens it only checks stay hashes.
- The console is the OAuth 2.1 authorization server of the `person` profile only: `/.well-known/oauth-protected-resource[/mcp]` names it, `/.well-known/oauth-authorization-server` describes it, `/oauth/register` takes dynamic client registration (RFC 7591; loopback `http` redirects on any port and app schemes such as `cursor:` or `vscode:`, never `https`), and `/oauth/authorize` (PKCE S256 only) and `/oauth/token` (`authorization_code`, rotating `refresh_token`) do the rest. Allowing an app creates the access key named after it, so its grants, the key list and Revoke work as for any key. Access tokens are engine agent tokens living one hour; the console keeps registered clients and the SHA-256 of refresh tokens (90 days) in its state file.
- Skills (`person` profile): a skill link `http://127.0.0.1:<port>/import?listing=<https URL of a skill catalog entry>` opens the same confirm screen, dispatched on `kind: skill`. It shows the licence, the source at its pinned commit, the file list with sizes and a warning when the skill carries scripts. Nothing is fetched beyond the entry until you confirm; then each file is fetched over public https, checked against its sha256 and its address pinned to `source.commit`, and refused for a branch name, `..`, an absolute path or a size over the limits (256 files, 1 MiB a file, 4 MiB a skill). The Skills tab sets each skill off, on or ask per access key; keys made later start off. AI apps see two tools, `skill_list` and `skill_load`, and a short index of their skills in the server instructions. The gateway serves skill files as text and never runs them. A skill without a redistributable licence is `metadata_only`: it is listed, and `skill_load` answers with its source instead of its files.
- "Activity" lists every function call an app made through the console's `/mcp`, per hour and per connector; a connector's page links to its own calls, a `skill_load` shows as the skill and the file it read, and a call opens with its arguments and result excerpt. The console captures them on its proxy (the engine keeps only a hash of the arguments) into `activity/YYYY-MM-DD.jsonl` beside the state file, mode 0600, for 30 days. Arguments are recorded by default and result excerpts (4 KB) only when turned on; likely secrets (token, password, secret, authorization, API key, bearer and key-shaped values) are replaced before anything is written. "Delete all activity" removes the files and hides the engine's older call records from the page.
- Server instructions (`person` profile, `gateway.relayInstructions: pinned`): the text an MCP server sends at initialize is kept with its connector, quarantined until a person accepts it, and quarantined again when it changes. Once accepted and turned on, AI apps read it in the engine's own server instructions, one quoted section per connector. The connector page shows the text under "Server instructions", off or on; turning it on accepts the text shown. The engine default and the `headless` profile keep `never`.
- Prompts and resources (`person` profile, `gateway.upstreamContent: granted`): a connector's prompts are listed as `<alias>__<name>` and its resources as `<alias>+<uri>`, each off, on or ask like a function and quarantined on change. The import review and the connector page list them with a text preview; the review proposes resources on and prompts off, because a prompt's text arrives in the chat as if you wrote it. With `gateway.skills: tools+prompts` every skill is also a prompt under its own name, so an app offers it as a slash command and loads it through the gateway; the console never copies skills into an app's folders. The engine default and `headless` refuse all three.
- The `headless` profile is how a product embeds the engine: no console, tokens from an external OIDC/JWKS issuer (edit `deploy/profiles/headless/connector-engine.yaml`), the engine published on `127.0.0.1:8080` and `:8081`.
- While `github.com/agents-control/plugkit` is private, `make up` vendors the modules with the host's `go` so the image builds without credentials.

## Layout

| Path | What it holds |
|---|---|
| `port/` | neutral types, the port interfaces, error codes |
| `core/` | pure rules: catalog visibility, grants, the 16 evaluation steps, approval transitions, pinning, listing checks, projection, names |
| `service/` | the use cases of §13.1 and the worker (approval timers, sweeps, re-list, fan-out) |
| `gateway/` | the agent endpoint `/mcp`: list, call, `find`/`call`, `approval_status`, `approval_cancel` |
| `server/` | the operation table and the HTTP, gRPC and management MCP surfaces built from it |
| `runtime/` | `ConnectorRuntime` adapters: `remote` (MCP), `declarative` (REST), `grpc`, `skill` (pinned skill files as text, no process), `memory` |
| `source/` | `ListingSource` adapters: `files`, `openapi`, `reflection`, `bsr`, `registry`, `docker`, `skillcatalog` (https catalog entries, no URL built in) |
| `store/` | `memory` and `postgres` (row-level security per tenant) |
| `secrets/`, `auth/`, `credential/` | envelope encryption (`local`, `hostkey`), token verification and the local issuer, the credential broker |
| `cli/keychain` | `cnctl keychain`: the unlock key in the host's keychain |
| `filter/`, `index/`, `sink/`, `relay/` | result filter, tool index, event sinks, the outbox relay |
| `app/`, `config/` | the composition root and `connector-engine.yaml` with `adapters.yaml` |
| `cmd/connector-engine`, `cmd/cnctl` | the server and the command line |
| `cmd/connector-console`, `console/` | the local web console of the `person` profile; it talks to the engine over HTTP only, frontend in `console/web` |
| `deploy/`, `Dockerfile`, `Makefile` | the image, `compose.yaml` with the `person` and `headless` profiles, their configuration files |
| `e2e/` | real runs of the built binaries on Postgres against live upstreams |
| `scripts/` | `smoke.sh` (one binary, memory store) and `e2e.sh` (the full suite) |

## Run it locally

```
go build -o /tmp/ce/connector-engine ./cmd/connector-engine
go build -o /tmp/ce/cnctl ./cmd/cnctl
CN_OPERATOR_TOKEN=dev /tmp/ce/connector-engine --config config/connector-engine.yaml --adapters /path/to/adapters.yaml
```

`scripts/smoke.sh` writes a working pair of files (dev mode, memory store, the local token issuer) and drives the binary with `curl` and `cnctl`; copy its heredocs as a starting point. The engine prints `http=... grpc=... operator=...` once it listens. The operator listener takes the `CN_OPERATOR_TOKEN` bearer; the public listener takes person and agent tokens, which the local issuer mints at `POST /local/token`.

Every number is a key of `config/connector-engine.yaml`; the ports (store, secrets, auth, runtime, source, sink, filter, index) are chosen in `adapters.yaml`. `connector-engine adapters check` validates a file the way boot does.

## The CLI

```
cnctl admin <group> <verb> [path values] [--field value] [--json BODY]
cnctl admin call <operationId> [path values] [--json BODY]
cnctl tools list|search QUERY|call NAME [--args JSON] [--fields /a,/b] [--out FILE]
cnctl skills export --to DIR [--skill NAME] [--prune]
```

- `cnctl admin` is every operation of the table over HTTP. It reads `CNCTL_URL` and `CNCTL_TOKEN` (or `--url`, `--token`). Groups: `listings`, `sources`, `connectors`, `functions`, `bindings`, `grants`, `approvals`, `overlays`, `tenants`, `roles`, `groups`, `operations`, `adapters`, `events`; `call` reaches the rest.
- `cnctl skills export` is an MCP client of `/mcp` too: it loads every skill granted to the agent with `skill_list` and `skill_load`, checks each file's sha256 and writes the exact bytes to `DIR/<skill>/` with mode 0644. Use `~/.claude/skills` (Claude Code) or `~/.agents/skills` (Codex, Cursor, VS Code, OpenCode, Gemini CLI). It records what it wrote in `DIR/.cnctl-skills.json`, never overwrites a file it did not write, never writes through a symlink, and with `--prune` removes only the files that manifest lists and this export did not write. Exported scripts get the trust of local skills, so it prints a warning when it writes one.
- `cnctl tools` is an MCP client of `/mcp` for an agent. It reads `CN_ENDPOINT` (the `/mcp` address) and `CN_AGENT_TOKEN`, writes the full answer to `--out` (a temporary file when empty) and prints a short summary. A held call prints its `approval_id`; ask again with `tools call approval_status`.

## Tests

| Command | What it runs |
|---|---|
| `go test -race ./...` | unit and integration tests; Postgres suites skip without Docker |
| `scripts/smoke.sh` | the binary on the memory store |
| `scripts/e2e.sh` | the e2e suite: builds both binaries, starts a throwaway `postgres:16-alpine` and live upstreams, drives the engine through `cnctl` and the go-sdk MCP client |

Set `CN_PG=0` (or run without Docker) and the e2e suite runs on the memory store and skips the restart case. The container it starts is removed by its exact name.
