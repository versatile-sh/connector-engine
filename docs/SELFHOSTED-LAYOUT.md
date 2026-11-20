# Self-hosted layout

Target: every piece of the local, self-hosted experience lives under `selfhosted/`. The repository builds, tests and installs with no access to the private `github.com/agents-control/plugkit` module. The engine core keeps no knowledge of self-hosted adapters or commands.

Status: design, Phase 1. Nothing moves until the in-flight console branches (`refactor/*`, `refactor-e-frontend`, `feat/skill-modal`) have merged. Base read: `origin/main` at `b03dc20`.

## Invariants

| # | Invariant | Checked by |
| --- | --- | --- |
| I1 | Dependency direction is core ← `selfhosted/`. No package outside `selfhosted/` imports a package under it, including test imports. | `scripts/check-layering.sh` in `make test` (see [Verification](#verification)) |
| I2 | `cmd/connector-engine` (the headless binary that products embed) links no self-hosted package and behaves as today. | `go list -deps ./cmd/connector-engine` has no `/selfhosted/` path |
| I3 | The module builds with `GOFLAGS=-mod=mod GOPRIVATE= GOPROXY=off` and no plugkit checkout. Outsiders can `go install github.com/versatile-sh/connector-engine/cmd/connector-engine@latest`. | the offline build in [Verification](#verification) |
| I4 | User-facing entry points stay: `make up`, `make down`, `make unlock`, `make keychain-*`, `make notify`, `make check`, the image `connector-engine:local`, the compose project `connector-engine` and its volume names. | `make check` on project `connector-engine-check`, port 7466 |
| I5 | Wire-visible names stay: `SchemaID` and the other `https://agents-control.dev/...` URIs, the `connector-engine` engine name, the `connector-engine/error` meta code, the adapter names in adapters files (`builtin:hostkey`). | unchanged golden tests |

## What is self-hosted, what is core

Self-hosted means "only the person profile on a person's own machine uses it". Core means "the headless engine, or any profile, needs it".

| Today | Classification | Reason |
| --- | --- | --- |
| `console/` (Go), `console/sealed`, `console/web` | self-hosted | the local console |
| `cmd/connector-console` (except `init`) | self-hosted | the console binary |
| `console/testmcp`, `console/testmcp/notes` | **core test fixture** | `gateway/relay_test.go` imports `notes`; the check image runs it. Moves to `internal/testmcp` |
| `connector-console init` | **core** | the headless compose profile runs it to write the operator token and the Postgres passwords. Becomes `connector-engine init` |
| `surface/` and its five adapters | self-hosted | approval channels of the console |
| `cli/keychain`, `cli/notify` | self-hosted | host keychain, host notifications |
| `secrets/hostkey` | self-hosted | needs the host's unlock socket and keychain |
| `secrets/local`, `secrets/onepassword`, `secrets/memory` | core | headless profiles select them; `hostkey` and `console/sealed` build on `secrets/local` |
| `deploy/profiles/person`, `deploy/check`, `deploy/compose.check.yaml`, the `engine` and `console` compose services | self-hosted | person profile and its check |
| `deploy/compose.yaml` (`init`, `postgres`, `engine-headless`), `deploy/profiles/headless`, `deploy/postgres` | core | headless profile |
| `scripts/console-check.sh`, `scripts/keychain.sh` | self-hosted | |
| `scripts/smoke.sh`, `scripts/e2e.sh`, `e2e/` | core | they build only `cmd/connector-engine` and `cmd/cnctl` |
| `docs/SECRETS.md`, `docs/approvals-surfaces.md` | self-hosted | |
| the 7 plugkit packages | **core** | `app`, `config`, `server`, `catalog`, `sink/webhook` and `e2e` import them |

## Target tree

```
.
├── app/                         core composition root; gains Main and Extensions
├── cli/                         core CLI; gains Command, Core and Main
│   ├── admin/ skills/ tools/
├── cmd/
│   ├── cnctl/                   core cnctl: admin, skills, tools
│   └── connector-engine/        core engine: serve, adapters, init
├── config/                      gains Builtin and AdaptersSpec(extra ...Builtin)
├── deploy/
│   ├── compose.yaml             init, postgres, engine-headless
│   ├── postgres/
│   └── profiles/headless/
├── internal/
│   ├── plugkit/                 in-module copy of 7 plugkit packages, MIT, see below
│   │   ├── LICENSE              upstream licence, verbatim
│   │   ├── UPSTREAM.md          repository, commit, package list, rewrite rule
│   │   ├── adapters/ api/acplug/v1/ conform/ discovery/ host/ outbox/ usage/
│   └── testmcp/                 test MCP server, moved from console/testmcp
├── scripts/
│   ├── check-layering.sh        enforces I1 and I2
│   └── sync-plugkit.sh          refreshes internal/plugkit from a plugkit commit
├── selfhosted/
│   ├── README.md                what lives here, how to run it
│   ├── selfhosted.go            package selfhosted: the self-hosted contributions, one list
│   ├── selfhosted.mk            unlock, keychain-*, notify, check, cnctl
│   ├── cli/keychain/ cli/notify/
│   ├── cmd/
│   │   ├── cnctl/               core commands + keychain, notify
│   │   ├── connector-console/   console serve
│   │   └── connector-engine/    core engine + hostkey + unlock
│   ├── console/                 Go console, sealed/, web/
│   ├── deploy/
│   │   ├── compose.yaml         include ../../deploy/compose.yaml; engine, console
│   │   ├── compose.check.yaml
│   │   ├── check/
│   │   └── profiles/person/
│   ├── docs/                    SECRETS.md, approvals-surfaces.md
│   ├── scripts/                 console-check.sh, keychain.sh
│   ├── secrets/hostkey/
│   └── surface/                 desktop, discord, slack, telegram, webhook
├── Dockerfile                   default target: self-hosted image; target headless: core only
└── Makefile                     core targets; include selfhosted/selfhosted.mk
```

Go `internal/` rule: `selfhosted/...` stays inside the module, so it keeps importing `internal/egress`, `internal/plugkit/...` and the core packages it uses today (`port`, `secrets/local`, `source/skillcatalog`, `server`).

## The composition seam

Three couplings make the core know `hostkey` today:

1. `app/build.go` `builder.secrets` opens `hostkey` by name.
2. `app.document()` type-asserts `*hostkey.Store` to add the `secrets.root_key_<form>` capability.
3. `cmd/connector-engine` runs `hostkey.Main` for `unlock`, and `config` lists `hostkey` in the `secrets.adapter` enum, in `Builtins`, in `Capabilities`, and checks `secrets.unlockSocket` and the plaintext-root-key acknowledgement.

`cmd/cnctl` has the same shape: its command map imports `cli/keychain` and `cli/notify`.

### Option A: explicit extensions passed by the main

The core exposes one value type and one entry point. A main composes the value and passes it in. Nothing is global.

```go
package config

// Builtin declares an adapter that a build adds to a port: its name, capabilities and the checks it adds to connector-engine.yaml.
type Builtin struct {
	Port         string
	Name         string
	Capabilities []string
	Check        func(Config) adapters.Findings
}

// AdaptersSpec declares the ports of the engine, with the builtins of the core and those a build adds.
func (c Config) AdaptersSpec(extra ...Builtin) adapters.Spec
```

```go
package app

// Extensions are the adapters and commands a build adds to the core engine.
type Extensions struct {
	Secrets  []SecretsAdapter
	Commands map[string]Command
}

// SecretsAdapter is a secrets port adapter that a build registers by name.
type SecretsAdapter struct {
	config.Builtin
	Open func(ctx context.Context, cfg config.Config, log *slog.Logger) (port.SecretStore, func(), error)
}

// Command is a subcommand of the engine binary, run after connector-engine.yaml loads.
type Command func(cfg config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int

// Main runs the engine binary: serve, adapters check|plan, init, and the commands of ext.
func Main(args, environ []string, stdin io.Reader, stdout, stderr io.Writer, ext Extensions) int
```

`Options` gains `Extensions Extensions`. `builder.secrets` looks the selected name up in `ext.Secrets` before the core builtins. `app.document()` replaces the type assertion with a structural check that the core owns:

```go
// reporter is a port adapter that reports capabilities known only once it is open.
type reporter interface{ Capabilities() []string }
```

The core CLI gets the same treatment:

```go
package cli

// Command is one cnctl subcommand.
type Command func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int

// Core returns the subcommands every cnctl build has: admin, skills, tools.
func Core() map[string]Command

// Main dispatches args to commands and prints usage for an unknown one.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string, commands map[string]Command) int
```

The mains:

```go
// cmd/connector-engine
func main() { os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr, app.Extensions{})) }

// cmd/cnctl
func main() { /* signal context */ os.Exit(cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ(), cli.Core())) }

// selfhosted/cmd/connector-engine
func main() { os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr, selfhosted.Engine())) }

// selfhosted/cmd/cnctl
func main() { /* signal context */ cmds := cli.Core(); maps.Copy(cmds, selfhosted.Commands()); os.Exit(cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ(), cmds)) }
```

`selfhosted/selfhosted.go` is the single self-hosted composition list:

```go
// Package selfhosted lists what the self-hosted build adds to the core engine and to cnctl.
package selfhosted

// Engine returns the hostkey secrets adapter and the unlock command.
func Engine() app.Extensions

// Commands returns the keychain and notify subcommands of cnctl.
func Commands() map[string]cli.Command
```

### Option B: init-time registry and blank imports

`config` and `cli` keep package-level registries (`config.RegisterBuiltin`, `cli.Register`), like `database/sql` drivers. `secrets/hostkey` and `cli/keychain` register themselves in `init()`. `selfhosted/cmd/*` blank-imports them, and the core mains import nothing extra.

### Comparison

| Criterion | A: explicit extensions | B: init registry |
| --- | --- | --- |
| Interface depth | one value type per surface, one `Main` | one `Register` per surface, simpler signature |
| Locality | a reader sees every self-hosted contribution in `selfhosted.go` and the main | the composition is spread over `init()` functions and blank imports |
| Testability | a test builds `Extensions{...}` with a fake adapter; no shared state | tests share a process-wide registry; order and leakage matter |
| Seam placement | at the main, the composition root the backend guide prescribes (`docs/back/02-COMPOSITION-ROOT`) | inside library packages, as a side effect of import |
| Failure mode | forgetting an extension fails at compile time or at the first `adapters check` | a missing blank import silently drops an adapter |
| Extensibility | a third build (for example a product with its own vault adapter) writes its own main | same, through blank imports |

Choice: **Option A**. It keeps one composition root per binary, explicit and testable. It adds no global state. B saves about ten lines, and it hides the composition that should stay visible.

### Decision on configuration keys

Two ways to keep `hostkey` out of `config`:

1. The registration carries the whole configuration section and its YAML decoding. This is fully decoupled, but `config.Load` would need raw YAML plumbing per adapter and a second validation pass.
2. The core keeps the neutral key `secrets.unlockSocket` (documented as "a socket where a secrets adapter receives its root key") and its YAML field. The `secrets.adapter` enum becomes the core names plus the registered names. The rules that mention `hostkey` (the socket is required, the plaintext root-key acknowledgement) move into the registration's `Builtin.Check`.

Choice: **2**. Strict YAML decoding stays as it is. The core holds one inert field and no rule about `hostkey`. Selecting `hostkey` in a core build fails with a clear finding: `secrets.adapter "hostkey" is not in this build; it ships in selfhosted/cmd/connector-engine`. Upgrade to 1 when a second adapter needs its own section.

`AckPlaintextRootKey` stays a core constant: the acknowledgement mechanism is core, and only its use moves.

## The plugkit copy

### Closure

The engine imports 4 plugkit packages directly. Their closure is **7 packages**, the set `vendor/modules.txt` lists today:

| Package | Pulled by |
| --- | --- |
| `adapters` | `app`, `config` |
| `discovery` | `app`, `server`, `server/internal/harness`, `console` |
| `outbox` | `sink/webhook`, `e2e` |
| `usage` | `catalog` |
| `conform` | `adapters` |
| `host` | `adapters`, `outbox` |
| `api/acplug/v1` (generated protobuf) | `outbox` |

Publishing 4 packages was approved. Publishing 7 needs the same approval for `conform`, `host` and `api/acplug/v1`. Trimming them out would change upstream `adapters` and `outbox` (plugin hosting, the gRPC sink), which breaks the byte-for-byte sync. The recommendation is to copy all 7 unchanged.

Licence: plugkit's `LICENSE` is MIT, `Copyright (c) 2026 agents.control`, the same as this repository. MIT allows the redistribution. The copy keeps that file verbatim at `internal/plugkit/LICENSE`.

### Home: `internal/plugkit`, import paths rewritten

| Option | Outsider `go install ...@version` | Library consumers | Sync | Verdict |
| --- | --- | --- | --- | --- |
| `third_party/plugkit` + `replace github.com/agents-control/plugkit => ./third_party/plugkit` | **fails**: the go command refuses `go install pkg@version` when the module's `go.mod` has a `replace` | **fails**: a dependency's `replace` is ignored, so consumers still resolve the private module | byte-identical | rejected: breaks I3 |
| Rename into own packages (fork, new names, free to edit) | works | works | none, the fork drifts | rejected for now |
| `internal/plugkit/...`, upstream files unchanged except import paths | works | works | scripted, one rewrite rule | **chosen** |

`internal/` keeps plugkit's API out of connector-engine's public surface: no outside module can import it, so it carries no compatibility promise. Core and `selfhosted/` can both import it.

### Rewrite rule and sync

`scripts/sync-plugkit.sh <plugkit-dir> [commit]`:

1. Checks out the commit (default: the one in `UPSTREAM.md`) in a temporary worktree of `<plugkit-dir>`.
2. Copies the 7 package directories (sources, tests and `testdata`) and `LICENSE` into `internal/plugkit/`.
3. Rewrites import specs only: in import blocks, `"github.com/agents-control/plugkit/<p>"` becomes `"github.com/versatile-sh/connector-engine/internal/plugkit/<p>"`. It never touches other strings. The raw descriptor in `api/acplug/v1/*.pb.go` keeps its `go_package` bytes: they are length-prefixed, and the runtime does not read them.
4. Drops any copied test that imports a plugkit package outside the 7, and lists it in `UPSTREAM.md`.
5. Rewrites `UPSTREAM.md` with the repository, the commit, the date, the package list and the dropped tests, then runs `gofmt -l` and `go build ./internal/plugkit/...`.

The protobuf registry keys on the `.proto` file name. A binary that links both the upstream plugkit and this copy would panic at init. connector-engine drops the upstream requirement, so that cannot happen here. A product that embeds both must use one or the other; this is noted in `UPSTREAM.md`.

Initial commit: `329d53ecfe45`, the version `go.mod` pins today. `go mod tidy` then removes `github.com/agents-control/plugkit` from `go.mod` and `go.sum`.

## Module path

The module path becomes `github.com/versatile-sh/connector-engine`, in one dedicated commit:

- `go mod edit -module github.com/versatile-sh/connector-engine`.
- Rewrite every Go import, the `go_package` option in `api/connectorengine/v1/connectorengine.proto`, and the paths in docs, the Makefile and scripts. The pattern is anchored on `github.com/versatile-sh/connector-engine`, so the plugkit paths and the `agents-control.dev` URIs are untouched.
- **Regenerate** `api/connectorengine/v1/*.pb.go` (protoc-gen-go v1.36.12, protoc v7.36.1, as the file header records). Do not edit it with sed: the raw descriptor holds `go_package` as a length-prefixed string (`Z` tag, length `S` = 83), and the new path is 2 bytes shorter.
- Downstream tools that import the module switch to the new path after the commit lands.

Event schema URIs (`catalog.SchemaID`, `https://agents-control.dev/...`) **stay**. They are identifiers on the wire, not fetch locations. Consumers match them exactly. A new name would need a schema version bump with a period that accepts both. That decision is separate from going public.

## Binaries, image and compose

| Binary | Source | Links self-hosted code |
| --- | --- | --- |
| `connector-engine` | `cmd/connector-engine` | no |
| `cnctl` (core) | `cmd/cnctl` | no |
| `connector-engine-selfhosted` | `selfhosted/cmd/connector-engine` | yes |
| `cnctl` (self-hosted) | `selfhosted/cmd/cnctl` | yes |
| `connector-console` | `selfhosted/cmd/connector-console` | yes |
| `testmcp` | `internal/testmcp` | no |

Dockerfile:

- The build stage builds all of the above. The web stage copies from `selfhosted/console/web`.
- `target: headless` holds core `connector-engine` and core `cnctl` only. This is the clean image for products that embed the engine.
- The default (last) stage is `connector-engine:local` as today. It holds core `connector-engine` at `/usr/local/bin/connector-engine` (still the image entrypoint), `connector-engine-selfhosted`, the self-hosted `cnctl` and `connector-console`. One image keeps `CN_IMAGE`, `make cli` and `make admin` unchanged. The cost is one extra engine binary in the person image.
- `target: testmcp` builds `./internal/testmcp`.

Compose:

- `deploy/compose.yaml` keeps `init`, `postgres` and `engine-headless`. `init` runs `/usr/local/bin/connector-engine init` and writes the same files as today.
- `selfhosted/deploy/compose.yaml` starts with `include: [../../deploy/compose.yaml]` and defines `engine` (entrypoint `/usr/local/bin/connector-engine-selfhosted`, `./profiles/person` mounted) and `console`. `include` resolves each file's relative paths against that file's own directory. Plain `-f a -f b` merging resolves against the first file, so it would not work here. `include` needs Docker Compose 2.20 or newer.
- The project name stays `connector-engine` (`-p`), so the volumes `connector-engine_secrets`, `_pgdata`, `_engine-data` and `_console-data` are reused on the next `make up`.

Makefile:

- The root `Makefile` keeps `up`, `down`, `logs`, `ps`, `build`, `vendor`, `test`, `cli` and `admin`, and ends with `include selfhosted/selfhosted.mk`.
- `COMPOSE` selects `selfhosted/deploy/compose.yaml` for `PROFILE=person` (the default) and `deploy/compose.yaml` for `PROFILE=headless`.
- `selfhosted.mk` holds `cnctl` (builds `./selfhosted/cmd/cnctl` into `bin/cnctl`), `unlock`, `keychain-*`, `notify` and `check`. `up` still ends with `unlock` for the person profile.
- `make up` from the repository root stays the one-liner.
- `make test` runs `go test -race ./...`, the web typecheck in `selfhosted/console/web`, `scripts/check-layering.sh`, the headless `adapters check` through `./cmd/connector-engine`, and the person `adapters check` through `./selfhosted/cmd/connector-engine`.
- `vendor` stays for offline image builds. It no longer needs private access.

Path-bound files to update in the move: `Dockerfile` (`COPY console/web`, the build paths), `.dockerignore` (`console/web/dist/app`), the `//go:embed` in `console/web` (relative, moves with the package), the compose mounts, `scripts/console-check.sh` (compose files, `connector-engine-testmcp:local`), `scripts/keychain.sh` (called by `selfhosted.mk`), `config/profiles_test.go` (reads `deploy/profiles/*`: the person half moves to `selfhosted/`, next to the code that can register `hostkey`), `docs/internal/STATUS.md` and `README.md` paths.

## Migration order (Phase 2)

Base: `origin/main` after the console branches merge. One worktree, one PR, each commit builds and passes `go vet ./...`:

1. **Module path rename**: `go.mod`, imports, the `.proto` option, regenerated `*.pb.go`, doc paths. Mechanical, nothing else.
2. **plugkit copy**: `internal/plugkit` via `scripts/sync-plugkit.sh`, imports switched, `go mod tidy` drops the requirement.
3. **testmcp to core**: `git mv console/testmcp internal/testmcp`, imports, the Dockerfile target.
4. **init to core**: `connector-console init` becomes `connector-engine init`; compose `init` switches; `connector-console` keeps `init` as a deprecated alias for one release, so an older compose file still works.
5. **Composition seams**: `config.Builtin`, `AdaptersSpec(extra...)`, `app.Extensions`, `app.Main`, `cli.Core` and `cli.Main`, the `reporter` interface. The hostkey rules move into a registration. `cmd/connector-engine` and `cmd/cnctl` still pass the hostkey and keychain extensions at this point, so behaviour is proven unchanged before anything moves.
6. **Move**: `git mv` of every self-hosted path in the table above into `selfhosted/`. The commit holds renames plus import-line edits only (`git diff -M --stat` shows renames at 90% similarity or more).
7. **Self-hosted mains**: `selfhosted/selfhosted.go`, `selfhosted/cmd/{connector-engine,cnctl}`. The core mains drop the self-hosted extensions. Dockerfile, compose, Makefile and `selfhosted.mk`.
8. **Layering guard**: `scripts/check-layering.sh` wired into `make test`.
9. **Docs**: `selfhosted/README.md`, move `SECRETS.md` and `approvals-surfaces.md`, update `README.md`, `docs/internal/STATUS.md` and `docs/internal/PLAN.md` paths, this document's status.

## Verification

```sh
# I1: no core package imports selfhosted/, test imports included
go list -f '{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... \
  | grep -v '^github.com/versatile-sh/connector-engine/selfhosted' | grep '/selfhosted' && exit 1
# I2: the headless binary links nothing self-hosted
go list -deps ./cmd/connector-engine ./cmd/cnctl | grep -E '/selfhosted(/|$)|plugkit' | grep -v '/internal/plugkit/' && exit 1
# I3: builds and tests with no plugkit and no network
GOFLAGS=-mod=mod GOPRIVATE= GOPROXY=off go build ./... && GOFLAGS=-mod=mod GOPRIVATE= GOPROXY=off go vet ./...
# suite (wait while the 1-minute load is above 12)
GOMAXPROCS=4 go test -p 2 -race ./...
# web
(cd selfhosted/console/web && npm ci --no-audit --no-fund && npm run typecheck && npm test)
# I4: the person stack end to end, own project on port 7466
make check
```

The offline build runs with the plugkit checkout moved aside, so a stray `replace` or cache hit cannot mask a missing package.

## Risks

| Risk | Mitigation |
| --- | --- |
| The module rename and the move touch most files and conflict with in-flight branches | start Phase 2 only after they merge; steps 1 and 6 are mechanical and easy to redo on a new base |
| A sed on generated protobuf corrupts the descriptor | regenerate `connectorengine.pb.go`; the plugkit rewrite touches import specs only |
| A live install on port 7465 runs the old compose file | volume and project names stay; after merge, `make up` rebuilds and the person `engine` switches entrypoint; the unlock socket and the keychain item `service=connector-engine` are unchanged. Phase 2 never touches the 7465 stack |
| Docker Compose older than 2.20 rejects `include` | `selfhosted/README.md` states the minimum; the check fails fast with Compose's own error |
| The closure publishes 7 plugkit packages, not 4 | maintainer approval for `conform`, `host` and `api/acplug/v1` before step 2 |
| The plugkit copy drifts from upstream | `UPSTREAM.md` pins the commit; `sync-plugkit.sh` is the only way to change `internal/plugkit` |
| A future core change imports `selfhosted/` again | `check-layering.sh` in `make test` fails the build |
