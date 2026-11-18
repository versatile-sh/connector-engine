# connector-engine

A neutral Go engine that lets a tenant plug tool servers (MCP, REST, gRPC) into its agents and decide, per agent and per function, what each one may call. It imports listings from a catalog, pins every function by hash, evaluates a four-layer grant table, holds a call for a person's approval when the rule says ask, and publishes CloudEvents. The specification is `adaptable-research-engine/studies/connector-engine/03-engine.md`; the build plan is [docs/PLAN.md](docs/PLAN.md) and what runs today is [docs/STATUS.md](docs/STATUS.md).

## Layout

| Path | What it holds |
|---|---|
| `port/` | neutral types, the port interfaces, error codes |
| `core/` | pure rules: catalog visibility, grants, the 16 evaluation steps, approval transitions, pinning, listing checks, projection, names |
| `service/` | the use cases of §13.1 and the worker (approval timers, sweeps, re-list, fan-out) |
| `gateway/` | the agent endpoint `/mcp`: list, call, `find`/`call`, `approval_status`, `approval_cancel` |
| `server/` | the operation table and the HTTP, gRPC and management MCP surfaces built from it |
| `runtime/` | `ConnectorRuntime` adapters: `remote` (MCP), `declarative` (REST), `grpc`, `memory` |
| `source/` | `ListingSource` adapters: `files`, `openapi`, `reflection`, `bsr`, `registry`, `docker` |
| `store/` | `memory` and `postgres` (row-level security per tenant) |
| `secrets/`, `auth/`, `credential/` | envelope encryption, token verification and the local issuer, the credential broker |
| `filter/`, `index/`, `sink/`, `relay/` | result filter, tool index, event sinks, the outbox relay |
| `app/`, `config/` | the composition root and `connector-engine.yaml` with `adapters.yaml` |
| `cmd/connector-engine`, `cmd/cnctl` | the server and the command line |
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
```

- `cnctl admin` is every operation of the table over HTTP. It reads `CNCTL_URL` and `CNCTL_TOKEN` (or `--url`, `--token`). Groups: `listings`, `sources`, `connectors`, `functions`, `bindings`, `grants`, `approvals`, `overlays`, `tenants`, `roles`, `groups`, `operations`, `adapters`, `events`; `call` reaches the rest.
- `cnctl tools` is an MCP client of `/mcp` for an agent. It reads `CN_ENDPOINT` (the `/mcp` address) and `CN_AGENT_TOKEN`, writes the full answer to `--out` (a temporary file when empty) and prints a short summary. A held call prints its `approval_id`; ask again with `tools call approval_status`.

## Tests

| Command | What it runs |
|---|---|
| `go test -race ./...` | unit and integration tests; Postgres suites skip without Docker |
| `scripts/smoke.sh` | the binary on the memory store |
| `scripts/e2e.sh` | the e2e suite: builds both binaries, starts a throwaway `postgres:16-alpine` and live upstreams, drives the engine through `cnctl` and the go-sdk MCP client |

Set `CN_PG=0` (or run without Docker) and the e2e suite runs on the memory store and skips the restart case. The container it starts is removed by its exact name.
