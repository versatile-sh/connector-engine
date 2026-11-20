# Build plan

> Internal build notes, kept for history. Section numbers (`§n`) and decision ids (`OD-n`, `SK-n`) refer to the project's original design specification, which is not published. The code and its tests are the source of truth.

How `connector-engine` was built from its design specification, cited below as `§n`. The design options and the decisions taken on them are cited as `OD-n`.

## 0. Decisions taken for the build

Engineering choices were delegated to the build on 2026-11-18. Every open decision OD-1 to OD-49 is built at the specification's recommendation, and every default is a configuration key (§14), so a later decision changes configuration, not code. The name is `connector-engine` (OD-1).

Decisions taken after the specification (ids are never renumbered):

| Id | Decision | Built as |
|---|---|---|
| OD-50 (2026-11-19, supersedes OD-32) | A connector's own prompts, resources and server instructions are relayed under the user's control, not refused. Choices SK-1 (resources proposed on, prompts proposed off, with a preview), SK-8 (this row) and SK-9 (`pinned` instructions) | `gateway.relayInstructions: never \| pinned` and `gateway.upstreamContent: refuse \| granted`, defaults `never` and `refuse` (OD-32's behaviour, the `headless` profile); the `person` profile sets `pinned` and `granted`. Items pinned like functions, selectors `prompt:`, `res:`, `instructions:` never reached by `*` or `class:`; subscriptions are not relayed |

| Convention | Rule here |
|---|---|
| Layers | `port/` holds neutral types, port interfaces and error codes. `core/*` is pure and imports only `port`. Adapters import `port`. `app/` is the only composition root. `service/` holds the use cases |
| Surfaces | One operation table in `server/operations.go` feeds HTTP (OpenAPI 3.1), gRPC (`connectorengine.v1`, generated), the management MCP surface (read-only by default, §13.2) and the `cnctl` CLI. A parity test fails when an operation lacks a form. The agent endpoint `/mcp` is MCP only (§10) |
| Events | Transactional outbox (`Tx.Emit`), CloudEvents 1.0 over Standard Webhooks, through `plugkit/outbox` |
| Adapters | Built-in, `memory` fake or a plugin per port, selected in `adapters.yaml` through `plugkit/adapters` |
| Code | Zero inline comments. A godoc of at most 2 lines on every exported symbol. Identifiers in English. No duplicated code, no wrapper that only forwards, no infrastructure type (pgx, net/http, grpc, a JSON library) crosses into `port` or `core`. Tests run with `-race` |
| Real runs | Integration tests use throwaway containers and skip when Docker is absent (`CN_DOCKER=0`, `CN_PG=0`). They remove only what they created, by exact name. No browser automation |
| Git | Each wave on a branch, merged into `main` with a merge commit, never squashed |

## 1. Scope

| Study phase (§18) | In this build | Out, and why |
|---|---|---|
| 1. One function for one agent | all | none |
| 2. Ask on use | all | `ApprovalChannel` adapters other than events (form elicitation mapped) |
| 3. The store | sources: own files, official registry v0.1, Docker catalog; review, trust levels, `lintListing`, exposure check, read-only management MCP | client registration by CIMD and DCR (pre-registered OAuth and API keys only); shared reviewed catalog (OD-19) |
| 4. Declarative connectors and search | `rest_declarative`, `grpc_declarative`, IDL sources (BSR, reflection, OpenAPI directory), `find`/`call`, `ToolIndex`, the agent CLI, projection | ACI.dev converter |
| 4b. Result and text curation | `ResultFilter` with the built-in regex and secret adapter, description overlays with `DescriptionSuggester` port (no built-in model adapter) | Presidio sidecar (a plugin, mapped) |
| 5. Later | none | container runtime (OD-20), Cedar, OpenBao/KMS, elicitation and tasks adapters |

## 2. Package layout

| Package | One line |
|---|---|
| `port/` | IDs, model types (tenant, workspace, listing, connector, function, binding, grant, approval, credential, overlay), the 16 port interfaces of §3.1, `Claims`, `Event`, error codes |
| `catalog/` | the 40 published and 3 consumed event types of §12, each with a JSON Schema and `x-usage` through `plugkit/usage` |
| `core/catalog` | the visibility predicate of §5.5 (V1 to V8) |
| `core/grant` | grant rows, levels to rows, row rules G1 to G10 (§6) |
| `core/evaluate` | the 16 steps and M1 to M5 of §7.1, `Evaluate` and `Search` |
| `core/approval` | transitions A1 to A14, timers, the denial brake, the repeat rules R1 to R4 (§8) |
| `core/pinning` | RFC 8785 canonical hash, function states, quarantine, accept and confirm rules (§5.3, §9), overlay hash (§9.1) |
| `core/listing` | manifest validation (§5.2), `class_source` order, `trust_level`, effective status |
| `core/projection` | RFC 6901 path validation against an `outputSchema` and the trim of `structuredContent` (§10.5) |
| `core/names` | exposed names `<alias>__<function>` and the lookup table (§10.1) |
| `service/` | use cases of §13.1, the worker (timers, sweep, re-list, fan-out, `outcome_unknown`) |
| `gateway/` | the agent endpoint: L1 to L7, C1 to C12, `find`, `call`, `approval_status`, `approval_cancel`, both MCP eras on `modelcontextprotocol/go-sdk` v1.8.0 |
| `runtime/remote`, `runtime/declarative`, `runtime/grpc`, `runtime/memory` | `ConnectorRuntime` adapters (§3.3) |
| `source/files`, `source/registry`, `source/docker`, `source/bsr`, `source/reflection`, `source/openapi`, `source/memory` | `ListingSource` adapters (§3.1, OD-45) |
| `runtime/skill`, `source/skillcatalog`, `cli/skills` | skills, added after the build waves: the skill kind served as pinned text, https catalog entries, and the host-side export (`docs/internal/STATUS.md`, "Skills") |
| `store/memory`, `store/postgres` | `Store` with audit and outbox rows in one transaction; Postgres with forced row-level security per tenant; one conformance suite |
| `secrets/local`, `selfhosted/secrets/hostkey`, `secrets/memory` | `SecretStore`: envelope encryption, a key-encryption key per tenant, `Destroy` for crypto-shredding (§4.4); `hostkey` wraps the root under an unlock key from the host keychain (`docs/SECRETS.md`) |
| `auth/jwks`, `auth/local` | `PrincipalVerifier`: JWKS of configured issuers; local issuer for the quickstart (acknowledged opt-in) |
| `credential/` | `CredentialInjector` and the broker for API keys and pre-registered OAuth with refresh (§11) |
| `filter/builtin` | `ResultFilter`: regex and secret patterns, pt-BR CPF and CNPJ included |
| `index/lexical` | `ToolIndex` |
| `server/` | operation table, HTTP, gRPC, OpenAPI, management MCP, parity test |
| `app/`, `config/`, `cmd/connector-engine`, `cmd/cnctl` | composition root, `adapters.yaml` and `connector-engine.yaml`, the binaries |
| `e2e/` | real runs: Postgres, a mock upstream MCP server, a REST server, a gRPC server with reflection, a scripted agent on the Go SDK |

## 3. Waves

| Wave | Branch | Owner | Contents | Done when |
|---|---|---|---|---|
| W1 Foundation | `w1/foundation` | one agent | `port/`, `catalog/`, every `core/*` package, `store/memory` with the conformance suite, `go.mod` | the truth table of §7.2 (67 rows), the owner cases, the catalog cases and the approval invariants of the spikes pass as unit tests of `core/*` with `-race`; every port of §3.1 is declared |
| W2a Persistence and identity | `w2/a` | agent | `store/postgres`, `secrets/*`, `auth/*`, outbox relay and sinks | the store conformance suite passes on memory and on a throwaway Postgres 16; RLS refuses a cross-tenant read; a superuser role is refused at boot |
| W2b Runtimes and sources | `w2/b` | agent | `runtime/*`, `source/*`, `credential/`, `filter/builtin`, `index/lexical` | each runtime captures definitions and calls a function against a local test server (MCP both eras, REST, gRPC with reflection and with a descriptor set); each source imports fixtures as `unreviewed` |
| W2c Use cases | `w2/c` | agent | `service/` and the worker on `store/memory` | every operation of §13.1 has a use case with a test; the worker transitions of §8.2 run against the memory store with a fake clock |
| W3a Gateway and agent CLI | `w3/a` | agent | `gateway/`, `cnctl tools list|search|call` | a Go SDK client lists and calls through the gateway for each owner case; `find`/`call` above the threshold; a held call answers `pending_approval` and runs once after `decideApproval`; projection and the result filter change the result only when on |
| W3b Surfaces and composition | `w3/b` | agent | `server/`, `app/`, `config/`, `cmd/*` | parity test passes; `connector-engine` boots from `config/` with memory adapters and with Postgres; management MCP refuses mutations by default and `decideApproval` always |
| W4 Real runs and review | `w4/e2e` | agent, then a review | `e2e/`, `scripts/smoke.sh`, `docs/internal/STATUS.md` | the owner cases (i) to (viii) and the six 2026-11-18 ideas pass end to end on Postgres with real upstream servers; every review finding fixed or answered |

W2a, W2b and W2c run in parallel; W3a and W3b run in parallel after W2 is merged. At most three agents at a time.
