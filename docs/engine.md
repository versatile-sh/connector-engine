# Run the engine

How to run `connector-engine` without Docker and drive it with `cnctl`. For the design, read [ARCHITECTURE.md](../ARCHITECTURE.md).

## Build and start

```sh
go install github.com/versatile-sh/connector-engine/cmd/connector-engine@latest
go install github.com/versatile-sh/connector-engine/cmd/cnctl@latest
CN_OPERATOR_TOKEN=dev connector-engine --config config/connector-engine.yaml --adapters adapters.yaml
```

- `scripts/smoke.sh` writes a working pair of files (dev mode, memory store, local token issuer) and drives the binary with `curl` and `cnctl`. Copy its heredocs as a starting point.
- The engine prints `http=... grpc=... operator=...` once it listens.

## Configuration

| File | Holds |
|---|---|
| `connector-engine.yaml` | every setting, with its default in `config/connector-engine.yaml` |
| `adapters.yaml` | the adapter chosen for each port: store, secrets, auth, runtime, source, sink, filter, index |

`connector-engine adapters check` validates both files the way boot does. Run it before you deploy a change.

## Listeners and tokens

| Listener | Key and default | Takes |
|---|---|---|
| public HTTP | `server.listen`, `:8080` | person and agent tokens; the agent endpoint is `/mcp` |
| gRPC | `server.grpcListen`, `:9090` | the same tokens as the public listener |
| operator | `server.operatorListen`, off (`:8081` in the profiles) | the `CN_OPERATOR_TOKEN` bearer |

In dev mode the local issuer mints tokens at `POST /local/token`. In production, configure an OIDC/JWKS issuer (see `deploy/profiles/headless/connector-engine.yaml`).

## cnctl

```text
cnctl admin <group> <verb> [path values] [--field value] [--json BODY]
cnctl admin call <operationId> [path values] [--json BODY]
cnctl tools list|search QUERY|call NAME [--args JSON] [--fields /a,/b] [--out FILE]
cnctl skills export --to DIR [--skill NAME] [--prune]
```

| Command | Reads | Does |
|---|---|---|
| `cnctl admin` | `CNCTL_URL`, `CNCTL_TOKEN` (or `--url`, `--token`) | every operation of the operation table, over HTTP. Groups: `listings`, `sources`, `connectors`, `functions`, `bindings`, `grants`, `approvals`, `overlays`, `tenants`, `roles`, `groups`, `operations`, `adapters`, `events`. `call` reaches the rest |
| `cnctl tools` | `CN_ENDPOINT` (the `/mcp` address), `CN_AGENT_TOKEN` | an MCP client for one agent. It writes the full answer to `--out` and prints a summary. A held call prints its `approval_id`; poll it with `tools call approval_status` |
| `cnctl skills export` | `CN_ENDPOINT`, `CN_AGENT_TOKEN` | writes every skill granted to the agent to `DIR/<skill>/` after checking each file's sha256 |

`cnctl skills export` details:

- Typical targets are `~/.claude/skills` (Claude Code) and `~/.agents/skills` (Codex, Cursor, VS Code, OpenCode, Gemini CLI).
- It records what it wrote in `DIR/.cnctl-skills.json`. It never overwrites a file it did not write and never writes through a symlink.
- `--prune` removes only files listed in that manifest that this export did not write.
- Exported scripts get the trust of local skills, so it warns when it writes one.

The self-hosted build of `cnctl` (`selfhosted/cmd/cnctl`) adds `keychain` and `notify`.

## Repository layout

| Path | Holds |
|---|---|
| `port/` | neutral types, port interfaces, error codes |
| `core/` | pure rules: catalog visibility, grants, evaluation, approval transitions, pinning, listing checks, projection, names |
| `service/` | use cases and the worker (approval timers, sweeps, re-list, event fan-out) |
| `gateway/` | the agent endpoint `/mcp` |
| `server/` | the operation table and the HTTP, gRPC and management MCP surfaces built from it |
| `runtime/` | `ConnectorRuntime` adapters: `remote` (MCP), `declarative` (REST), `grpc`, `skill`, `memory` |
| `source/` | `ListingSource` adapters: `files`, `openapi`, `reflection`, `bsr`, `registry`, `docker`, `skillcatalog` |
| `store/` | `memory` and `postgres` (row-level security per tenant) |
| `secrets/`, `auth/`, `credential/` | envelope encryption, token verification and the local issuer, the credential broker |
| `filter/`, `index/`, `sink/`, `relay/` | result filter, tool index, event sinks, outbox relay |
| `app/`, `config/` | the composition root and the configuration files |
| `cmd/connector-engine`, `cmd/cnctl` | the headless binaries |
| `internal/plugkit` | in-module copy of the plugin kit (adapters, outbox, discovery) |
| `selfhosted/` | the `person` profile: console, approval surfaces, host keychain, its binaries and compose file |
| `deploy/` | `compose.yaml` and the `headless` profile |
| `e2e/`, `scripts/` | end-to-end runs of the built binaries, smoke and e2e scripts |
