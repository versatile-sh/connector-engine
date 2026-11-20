![The headless engine](../assets/section-headless.svg)

# The headless engine

The `headless` profile is the engine alone: no console, no keychain, no person. Your product embeds it, sends it tokens from your own identity provider, and drives it through its API and its events. Use it for teams, companies and SaaS products.

| | `person` | `headless` |
|---|---|---|
| Identity | the console is the OAuth server | tokens from your OIDC/JWKS issuer |
| Who decides approvals | one person, in the console or a chat app | your product, through the API and events |
| Binaries | `selfhosted/cmd/*` | `cmd/connector-engine`, `cmd/cnctl` |
| Profile files | `selfhosted/deploy/profiles/person/` | `deploy/profiles/headless/` |

The headless binaries contain no self-hosted code; `scripts/check-layering.sh` enforces it.

## Try it in one minute

A dev pair with a memory store and a local token issuer, from a clone:

```sh
go install github.com/versatile-sh/connector-engine/cmd/connector-engine@main
go install github.com/versatile-sh/connector-engine/cmd/cnctl@main
CN_OPERATOR_TOKEN=dev connector-engine --config examples/dev/connector-engine.yaml --adapters examples/dev/adapters.yaml
```

```text
connector-engine dev mode=dev http=127.0.0.1:8080 grpc=127.0.0.1:9090 operator=127.0.0.1:8081
```

In a second terminal:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/livez
CNCTL_URL=http://127.0.0.1:8081 CNCTL_TOKEN=dev cnctl admin tenants create t1
```

```text
204
{
  "id": "t1",
  "state": "active",
  "key_ref": "local:t1:1",
  ...
}
```

- No release is tagged yet, so `go install` takes `main`. From the first release, `CONNECTOR_ENGINE_PROFILE=headless` with the [installer](../getting-started/install.md#linux-and-macos) or the `ghcr.io/versatile-sh/connector-engine-headless` image work too.
- `scripts/smoke.sh` drives this pair end to end with `curl` and `cnctl`; copy its heredocs as a starting point.

## Run it with Postgres

With Docker, from a clone:

```sh
make up PROFILE=headless
```

It starts `init` (operator token and Postgres passwords), Postgres and the engine with `deploy/profiles/headless/`. The public listener is on `127.0.0.1:8080` and the operator listener on `127.0.0.1:8081`. Set your issuer in `deploy/profiles/headless/connector-engine.yaml` first ([OIDC](oidc.md)).

On a server you control: [run it on a VPS](../self-hosted/vps.md#option-b-team-or-headless). On a platform-as-a-service: [community templates](cloud.md).

## Configuration files

| File | Holds |
|---|---|
| `connector-engine.yaml` | every setting; the defaults are in `config/connector-engine.yaml` ([reference](../reference/config.md)) |
| `adapters.yaml` | the adapter for each port: store, secrets, auth, runtime, source, sink, filter, index |

Production refuses risky settings unless `adapters.yaml` acknowledges them by name (`acknowledge: [...]`), for example `plaintext_root_key` for the `localfile` secrets adapter.

```sh
connector-engine --config connector-engine.yaml --adapters adapters.yaml adapters check
```

```text
adapters ok
```

Run it before every deploy: it validates both files the way boot does and prints each warning.

## Listeners and tokens

| Listener | Key and default | Takes |
|---|---|---|
| public HTTP | `server.listen`, `:8080` | person and agent tokens; the agent endpoint is `/mcp`; probes `/livez`, `/readyz` |
| gRPC | `server.grpcListen`, `:9090` | the same tokens as the public listener |
| operator | `server.operatorListen`, off (`:8081` in the profiles) | the operator bearer token (`auth.operator.tokenRef`) |

Keep the operator listener on a private network. It can do everything, including `GET /v1/snapshot` when `server.snapshot: true` (off in `headless`), which returns every row and the plaintext keys.

## Adapters and the registry

Every port has built-in adapters, chosen in `adapters.yaml`:

| Port | Built-in adapters |
|---|---|
| store | `postgres` (row-level security per tenant), `memory` |
| secrets | `localfile`, `onepassword`, `memory` |
| auth | `jwks`, `local` (dev issuer) |
| runtime | `remote` (MCP), `declarative` (REST), `grpc`, `skill`, `memory` |
| source | `files`, `official` (MCP registry), `docker`, `openapi`, `reflection`, `bsr`, `skillcatalog` |
| sink | `log`, `webhook` (Standard Webhooks signatures) |
| filter, index | `builtin`, `lexical` |

```sh
cnctl admin adapters list
```

Your own adapter is one Go package with a `Register(r *app.Registry)` function and one line in your own `main`:

```go
adapters := app.Core()
mycompany.Register(adapters)
os.Exit(app.Main(app.Build{Version: version, Adapters: adapters}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
```

How to write one: [CONTRIBUTING.md](../../CONTRIBUTING.md#add-an-adapter). How the registry works: [ARCHITECTURE.md](../../ARCHITECTURE.md#the-self-hosted-layer).

## Next

| Read | For |
|---|---|
| [OIDC](oidc.md) | plugging in your identity provider |
| [Postgres](postgres.md) | the database role and tenants |
| [Scaling](scaling.md) | instances and replicas |
| [Security model](security-model.md) | what the engine protects and what it trusts |
| [Events](../reference/events.md) | driving your product from CloudEvents |
