# Self-hosted layout

Target: every piece of the local, self-hosted experience lives under `selfhosted/`. The repository builds, tests and installs with no access to the private `github.com/agents-control/plugkit` module. The engine core keeps no knowledge of self-hosted adapters or commands.

Status: executed. The design was approved in #25. The registry seam replaced the extensions-only seam of that draft, to also remove the adapter `switch` statements.

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
├── app/                         core composition root: Registry, Core, Build, Main
├── cli/                         core CLI; gains Command, Core and Main
│   ├── admin/ skills/ tools/
├── cmd/
│   ├── cnctl/                   core cnctl: admin, skills, tools
│   └── connector-engine/        core engine: serve, adapters, init
├── config/                      gains Builtin and Config.With
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

Before the move, the core knew `hostkey` in three places, and it chose every adapter in `switch` statements (`app/build.go`, 14 `u.Name ==` cases):

1. `builder.secrets` opened `hostkey` by name.
2. `app.document()` type-asserted `*hostkey.Store` to add the `secrets.root_key_<form>` capability.
3. `cmd/connector-engine` ran `hostkey.Main` for `unlock`, and `config` listed `hostkey` in the `secrets.adapter` enum, in `Builtins` and in `Capabilities`, and checked `secrets.unlockSocket` and the plaintext-root-key acknowledgement.

`cmd/cnctl` had the same shape: its command map imported `cli/keychain` and `cli/notify`.

### Option A: an explicit registry the built-ins use too (chosen)

Each binary builds one `*app.Registry` and passes it in. The core registers its own adapters through the same call a third party uses. Nothing is global, and nothing happens in `init()`.

```go
package app

// Port names a port of the engine and the Go type its adapters provide.
type Port[T any] struct{ name string }

var (
	Stores   = Port[port.Store]{config.PortStore}
	Secrets  = Port[port.SecretStore]{config.PortSecrets}
	Runtimes = Port[port.ConnectorRuntime]{config.PortRuntime}
	Sources  = Port[port.ListingSource]{config.PortSource}
	Sinks    = Port[port.EventSink]{config.PortSink}
	Filters  = Port[port.ResultFilter]{config.PortFilter}
	Indexes  = Port[port.ToolIndex]{config.PortIndex}
)

// Adapter declares an adapter of a port: name, capabilities, the settings it accepts in adapters.yaml, the checks it adds to connector-engine.yaml.
type Adapter = config.Builtin

// Factory opens an adapter once its port is selected; the closer may be nil.
type Factory[T any] func(ctx context.Context, env Env) (T, func(), error)

// Register adds the adapter a on the port p; a duplicate name on a port panics, as a composition error.
func Register[T any](r *Registry, p Port[T], a Adapter, open Factory[T])

// Core returns a registry holding the engine's own adapters.
func Core() *Registry

// Env is what a factory may read: the configuration, its port's settings, references, the clock, the log, the ports opened before it.
type Env struct {
	Config   config.Config
	Settings map[string]any
	Lookup   func(string) string
	Clock    port.Clock
	Log      *slog.Logger
	Store    port.Store
	Secrets  port.SecretStore
	Applier  credential.Applier
}
```

The typed `Port[T]` makes `Register(r, app.Secrets, ..., openSink)` a compile error. `Adapter{Name: "memory"}` registers the memory tier of a port. The auth port stays the core's own: its verifier is tied to the local issuer and has one implementation.

`app.New` takes `Options.Adapters` (nil is `Core()`). `builder` opens each selection through the registry, so `app/build.go` has no `switch` on adapter names. An adapter that knows extra capabilities only once open, such as `hostkey` with how its root key rests, implements `Capabilities() []string`. `app.document()` checks for that interface and no longer type-asserts.

Each binary is a `Build`:

```go
// Build is what one engine binary composes: its version, its adapters (nil is Core) and the subcommands it adds.
type Build struct {
	Version  string
	Adapters *Registry
	Commands map[string]Command
}

// Command is a subcommand of the engine binary; it runs once connector-engine.yaml has loaded.
type Command func(cfg config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int

// Main runs the engine binary: serve, "adapters" check|plan, "init", or a subcommand of b.
func Main(b Build, args, environ []string, stdin io.Reader, stdout, stderr io.Writer) int
```

The core CLI follows the same pattern:

```go
package cli

type Command func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int

// Core returns the subcommands every cnctl build has: admin, skills, tools.
func Core() map[string]Command

// Main runs the subcommand args[0] of commands, or prints their names and returns 2.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string, commands map[string]Command) int
```

The mains:

```go
// cmd/connector-engine: the core alone
app.Main(app.Build{Version: version}, ...)

// selfhosted/cmd/connector-engine
adapters := app.Core()
selfhosted.Register(adapters)
app.Main(app.Build{Version: version, Adapters: adapters, Commands: selfhosted.EngineCommands()}, ...)

// selfhosted/cmd/cnctl
commands := cli.Core()
maps.Copy(commands, selfhosted.Commands())
cli.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Environ(), commands)
```

A third-party adapter is one package with a `Register(r *app.Registry)` and one line in its own main. Out-of-process plugin selections (`plugin:<name>`) are still refused: no port of this build speaks the plugin protocol yet, and that is separate from this seam.

### Option B: init-time registry and blank imports

`app` keeps a package-level registry, as `database/sql` does for drivers. `secrets/hostkey` registers itself in `init()`. `selfhosted/cmd/*` blank-imports it, and the core mains import nothing extra.

### Comparison

| Criterion | A: explicit registry | B: init registry |
| --- | --- | --- |
| Interface depth | `Register` plus a typed `Port`, one `Main` per binary | `Register` alone, a simpler signature |
| Locality | each main shows its whole composition; the built-ins are one list in `app/core.go` | the composition is spread over `init()` functions and blank imports |
| Testability | a test builds `app.Core()`, registers a fake, boots; no shared state (`TestAnAddedSecretsAdapterIsSelectableAndReportsItsCapabilities`) | tests share a process-wide registry; order and leakage matter |
| Seam placement | at the main, the composition root (`docs/back/02-COMPOSITION-ROOT`) | inside library packages, as a side effect of import |
| Failure mode | a missing registration fails `adapters check` with `unknown built-in` | a missing blank import silently drops an adapter |

Choice: **Option A**. It keeps one composition root per binary, explicit and testable. It adds no global state. B saves about ten lines, and it hides the composition that should stay visible.

### Configuration keys

`config` cannot import `app`, so it learns the adapters of a build as data: `cfg.With(r.Builtins()...)`. `AdaptersSpec` then lists each port's builtins, capabilities and settings from those declarations, and `Check` accepts a `secrets.adapter` only if the build has it. `app.Main` and `app.New` call `With`. A test that loads adapters without a boot calls `cfg.With(app.Core().Builtins()...)`.

The core keeps the neutral key `secrets.unlockSocket` ("where an adapter of the build receives its unlock key"), because `config.Load` decodes strictly. The rules that mention `hostkey` (the socket is required, the plaintext root-key acknowledgement) live in its registration's `Check`. In a core build, `secrets.adapter: hostkey` fails with `"hostkey" is not one of memory, localfile, onepassword, the secrets adapters of this build`. Per-adapter configuration sections would remove the inert key. That is the upgrade once a second adapter needs its own section.


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
4. **Composition seams**: `config.Builtin` and `Config.With`, `app.Registry`, `app.Core`, `app.Build` and `app.Main`, `cli.Core` and `cli.Main`, the `Capabilities()` interface. The hostkey rules move into a registration. `app/build.go` opens every selection through the registry. `selfhosted/selfhosted.go` is created here, and `cmd/connector-engine` and `cmd/cnctl` still compose it at this point, so behaviour is proven unchanged before anything moves.
5. **init to core**: `connector-console init` becomes `connector-engine init`, and compose `init` switches through `app.Main`. No alias is kept: the compose file and the image ship from the same commit.
6. **Move**: `git mv` of every self-hosted path in the table above into `selfhosted/`. The commit holds renames plus import-line edits only (`git diff -M --stat` shows renames at 90% similarity or more).
7. **Self-hosted mains**: `selfhosted/cmd/{connector-engine,cnctl}`. The core mains drop the self-hosted registrations and commands. Dockerfile, compose, Makefile and `selfhosted.mk`.
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
