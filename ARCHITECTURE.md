![How it is built](docs/assets/section-architecture.svg)

# Architecture

connector-engine decides, for each agent and each function of each connected tool server, whether a call runs, waits for a person, or is refused. This page explains the layers, the path of one call, and the rules the engine enforces.

## Layers: ports and adapters

Dependencies point inward: adapters import `port`, `core` imports only `port`, and `app/` wires everything.

| Layer | Packages | Rule |
|---|---|---|
| Ports | `port/` | neutral types, port interfaces, error and reason codes. No infrastructure types (pgx, `net/http`, gRPC, a JSON library) |
| Core | `core/*` | pure functions over `port` types: no I/O, no clock, no globals |
| Use cases | `service/` | one store transaction per mutation; the worker runs timers, sweeps, re-lists and event fan-out |
| Inbound surfaces | `gateway/`, `server/` | translate MCP, HTTP, gRPC and management MCP into use cases |
| Outbound adapters | `store/`, `runtime/`, `source/`, `secrets/`, `auth/`, `credential/`, `sink/`, `filter/`, `index/` | implement one port each |
| Composition root | `app/`, `config/`, `cmd/*` | read the configuration, open the adapters, start the listeners |

### Ports and their built-in adapters

| Port | Purpose | Built-in adapters |
|---|---|---|
| `Store` | state, audit rows and the event outbox in one transaction | `postgres` (row-level security per tenant), `memory`; `sqlite` (tenant-scoped queries, WAL) in the self-hosted build |
| `SecretStore` | envelope encryption with a key per tenant; destroying the key shreds the data | `localfile`, `onepassword`, `memory`; `hostkey` in the self-hosted build |
| `PrincipalVerifier` | verify bearer tokens | `jwks`, `local` (dev issuer) |
| `ConnectorRuntime` | start a connector, list its functions, call one | `remote` (MCP), `declarative` (REST), `grpc`, `skill`, `memory` |
| `ListingSource` | import listings from a catalog | `files`, `official` (MCP registry), `docker`, `openapi`, `reflection`, `bsr`, `skillcatalog` |
| `EventSink` | deliver CloudEvents | `log`, `webhook` (Standard Webhooks signatures) |
| `ResultFilter` | redact secrets in results | `builtin` |
| `ToolIndex` | search the functions granted to an agent | `lexical` |
| `PrincipalVerifier` (`auth`) | verify an agent or person token | `jwks`, `local` |
| `DefinitionScanner` (`scanner`), `DescriptionSuggester` (`suggester`) | scan a definition at pin time; propose an overlay description | none built in: a port joins `adapters.yaml` once a build registers an adapter on it, and `vsh admin overlays suggest` answers `unavailable` until then |
| `Authorizer` (`authz`) | evaluate calls and role questions | the built-in table in `authz/table` (`authz` joins `adapters.yaml` once a build registers another adapter) |

`adapters.yaml` selects one adapter per port (several for runtimes and sources). `connector-engine adapters check` validates the selection the way boot does.

## The path of one call

```text
AI app ──MCP──▶ gateway (/mcp)
                  │  verify token, map tools/call
                  ▼
                service.Call
                  │  resolve exposed name ─▶ connector + function
                  │  Authorizer.Evaluate ─▶ allow | ask_once | ask | deny
                  │  check arguments, look up a repeated call
                  ├── deny ───────────▶ answers like an unknown tool
                  ├── ask ────────────▶ approval record ─▶ pending_approval
                  └── allow ──────────▶ ConnectorRuntime.Call
                                           │  credential injected
                                           ▼
                                        ResultFilter, projection
                  ◀───────────────────── result
                  outbox row in the same transaction ─▶ relay ─▶ EventSink
```

- `tools/list` follows the same path with `Authorizer.Search`, so an agent sees only functions it may call.
- A denied function answers exactly like an unknown one: an agent cannot probe what exists.
- The authorizer is asked first, without arguments when they fail the parse, so a denied function still answers like an unknown one; only an allowed function answers `invalid_argument`, and nothing is forwarded or re-listed.
- Arguments are read once by `core/args` before the token scan, the schema check and the call hash. It refuses an object that repeats a key at any depth.
- It refuses a top-level value that is not an object; `null` and absent arguments are accepted.
- It refuses data after the top-level value.
- The gateway's own tools (`find`, `call`, approval and skill tools) apply the same parse to their parameters, and `call` also refuses wrapper keys that differ from `name`, `arguments` and `fields` only by case.
- Every event is written to the outbox inside the use case's transaction. `relay/` delivers it at least once, in order. Schema URIs (`https://agents-control.dev/...`) are stable wire identifiers.

## Grants

Evaluation walks four grant layers, then caps the result:

| Order | Layer | Effect |
|---|---|---|
| 1 | tenant | optional; narrows when present |
| 2 | workspace | required; a missing row denies |
| 3 | agent | required; a missing row denies |
| 4 | user acted for | optional; narrows when present |
| then | catalog ceiling | the listing's visibility caps the mode |
| then | destructive functions | `allow` becomes `ask_once` (or `ask`) |
| then | exposure check | an agent holding private data, untrusted content and an outbound path is forced to `ask` |
| then | session grant | an approved `ask_once` allows the rest of the session |

The strictest mode wins across layers. `core/evaluate` holds the steps and `core/grant` the row rules.

## Approvals

- `ask` holds the call. The gateway waits up to `approval.holdWindow` (20 s), then answers `pending_approval` with an id the agent polls through `approval_status` or drops with `approval_cancel`.
- A person decides through the operator API, the console, or an approval surface.
- An approved call runs once. A repeated identical call (same session, same call hash) attaches to the open record instead of opening a new one.
- A forwarded call is never sent again. When its result cannot be recorded, the agent gets `outcome_unknown` and the row stays `started` until the worker sweeps it. A repeat that a session approval would allow while the last identical call is `started` or `outcome_unknown` is held for a person.
- States: `pending`, `approved`, `executing`, `executed`, `failed`, `outcome_unknown`, `denied`, `expired`, `cancelled`. `core/approval` holds the transitions; the worker runs the timers and a denial brake.

## Pinning and quarantine

- Each function definition is hashed with RFC 8785 canonical JSON when it is captured.
- A new function, or a changed description or schema, is `quarantined` until a person accepts it. A grant made against an older hash goes stale and needs `confirmGrant`.
- Re-lists run on a schedule, so a server that changes its tools after install is caught.
- Server instructions, prompts and resources are pinned the same way when a profile relays them.
- `core/pinning` holds the hash and state rules.

## Profiles

| Profile | Identity | Who decides approvals | Binaries |
|---|---|---|---|
| `headless` | tokens from your OIDC/JWKS issuer | your product, through the operator API and events | `cmd/connector-engine`, `cmd/vsh` |
| `person` | the console is the OAuth 2.1 server for AI apps; Owner, Admin and Member roles | the console's users, in the console or an approval surface | `selfhosted/cmd/*` |

Each profile is a pair of `connector-engine.yaml` and `adapters.yaml` under `deploy/profiles/headless` or `selfhosted/deploy/profiles/person`.

## The self-hosted layer

`selfhosted/` holds everything only a person's own machine needs: the console, the approval surfaces (desktop, Discord, Slack, Telegram, webhook), the host keychain (`selfhosted/secrets/hostkey`, `selfhosted/cli/keychain`), desktop notifications, and the install without Docker (`selfhosted/standalone`), which runs the engine and the console in one process over the `sqlite` store.

- Dependency direction: `selfhosted/` imports the core; no core package imports `selfhosted/`. `scripts/check-layering.sh` enforces it in `make test` and CI.
- Composition: every adapter of the store, secrets, runtime, source, sink, filter, index, auth and authz ports, the engine's own included, is registered in an `app.Registry` by port and name: `app.Register(r, app.Secrets, app.Adapter{Name: "hostkey", ...}, open)`. `app.Core()` returns the engine's own; `app/build.go` opens what `adapters.yaml` selects through the registry. Each main builds its registry and its extra subcommands and passes them to `app.Main` (engine) or builds a tree with `cli.Root` and runs it with `command.Main` (`vsh`). `selfhosted/selfhosted.go` lists the self-hosted adapters and commands; `selfhosted/cmd/vsh/main.go` adds the TUI, `manage` and the `standalone` lifecycle commands itself.

```go
// cmd/connector-engine: the core alone
func main() { os.Exit(app.Main(app.Build{Version: version}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr)) }

// selfhosted/cmd/connector-engine: the core plus hostkey, sqlite and unlock
func main() {
	adapters := app.Core()
	selfhosted.Register(adapters)
	os.Exit(app.Main(app.Build{Version: version, Adapters: adapters, Commands: selfhosted.EngineCommands()}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
```

- The console reaches the engine over HTTP only, like any other client; in the no-Docker install both run in one process and still talk over loopback HTTP.
- Design records are kept outside this repository.
