# Secrets at rest

Every secret the engine keeps (credentials, held call arguments, results, client secrets) is a `port.Sealed` record: a data key per record, wrapped by a key-encryption key per tenant, wrapped by one root key (`secrets/internal/ring`). Postgres never holds plaintext. This page is about the last link: where the root key rests and who can read it.

## Adapters of the `secrets` port

| Adapter | Root key at rest | Who selects it |
|---|---|---|
| `builtin:localfile` (`secrets/local`) | `root.key`, plaintext hex, mode 0600, refused when group or other can read it | the default, the `headless` profile, any config that does not opt in |
| `builtin:hostkey` (`selfhosted/secrets/hostkey`) | `root.key.wrapped`: the root sealed with AES-256-GCM under an unlock key that lives in the host's OS keychain | the `person` profile |
| `builtin:keychain` (`hostkey.Unlock`, registered by `selfhosted/standalone`) | the same `root.key.wrapped`, with the unlock key read from the OS keychain inside the process | the install without Docker (`connector-engine-selfhosted up`) |
| `builtin:onepassword` (`secrets/onepassword`) | `root.key.wrapped`, under an unlock key read at start from a 1Password item through a 1Password Connect server | opt-in by config, any profile, headless included |
| `memory` | in the process only | tests and dev |

`hostkey` is `secrets/local` with the root file wrapped; the key ring, the rotation files (`root.key.next`, `root.key.next.wrapped`) and the crash recovery are the same code. Selecting it is opt-in (`ports.secrets.use: builtin:hostkey` and `secrets.unlockSocket`); the engine binary never needs it, and the `headless` selection is pinned by `config/profiles_test.go`.

The console seals its own secrets (notifier tokens) through `selfhosted/console/sealed`, a `port.SecretStore` under key scope `selfhosted/console`, with the same adapter: `hostkey` in the `person` profile (`CONSOLE_UNLOCK_SOCKET`), `secrets/local` otherwise. Owner passwords, session cookies and refresh tokens stay hashes.

The local token issuer keeps its Ed25519 signing keys in `local-issuer.json` beside the ring, each seed sealed under the installation key, so tokens it minted survive a restart, a reboot and `make keychain-migrate`. At boot it rotates a signing key older than `auth.localIssuer.rotateEvery` (30 days by default). The retired key stays in the published key set for `max(1h, auth.agentTokenMaxLifetime)`, until every token it signed has expired. With `secrets: memory` the key is made at start and never stored, as before.

## How the unlock key reaches the container

The engine and the console run in Docker and cannot reach the host keychain. The host reads the key and pipes it into the running container:

```
make up | make unlock
  bin/cnctl keychain get --name <project>          host: macOS Keychain, Secret Service, or DPAPI
    | docker compose exec -T engine connector-engine unlock
        -> unix socket /run/connector-engine-unlock/unlock.sock   tmpfs inside the container, 0700, uid 65532
        -> secrets/hostkey: unwrap root.key.wrapped in memory, open the store
```

- The key travels on pipes only: never in argv, an environment variable, a log line, a compose file or a file on any disk. `cnctl keychain` writes it on the stdin of `security -i` (macOS) and `secret-tool store` (Linux), never as an argument.
- The socket is in a `tmpfs` mount of each container, owned by the image user. No other container and no host process reaches it; `docker exec` needs the Docker socket, which is root-equivalent anyway.
- It behaves the same on Linux, Docker Desktop for macOS and for Windows: it needs neither a bind-mounted host socket (those do not cross the Docker Desktop VM) nor a host file.
- After a restart (a crash, `docker restart`, a reboot with `restart: unless-stopped`) the container comes back locked and waits on the socket: the engine logs `waiting for the host to deliver the unlock key` and serves nothing; the console serves and shows "locked". `make unlock` opens both. This is the price of never resting the key outside the keychain. A locked engine with an unlocked console shows no banner: the console only sees an engine that does not answer. An AI app calling `/mcp` meanwhile gets 503 with `Retry-After: 5` and a JSON-RPC error: "the gateway is locked: run make unlock …" while the console is locked too, else "the gateway is not answering; after a restart it stays locked until you run make unlock …".

Tested on Linux with gnome-keyring. The macOS and Windows backends are covered by command-line fakes and a cross-compile only.

Alternatives rejected, from Docker's documentation and not measured here: compose `secrets:` without Swarm are bind mounts of a host file (a plaintext key on the host disk; macOS and Windows have no per-user tmpfs to put it in); environment variables show up in `docker inspect` and `/proc`; a host unix socket bind-mounted into a container is not expected to cross the Docker Desktop VM.

## Without Docker

`connector-engine-selfhosted up` runs the engine and the console as one process of the user, so it reads the unlock key from the keychain itself, at start, through the same backends: no socket, no `docker exec`, and no locked state after a restart. A fresh root is created wrapped; a plaintext root, left by a run without a keychain, is wrapped (`Migrate`) on the first start that finds one; a wrapped root without its key refuses to start, and when the keychain holds no item for it the start reads the keychain only, never minting a new key over the lost one. `--no-keychain`, or a host without a keychain, keeps plaintext roots with the warning of [Without a keychain](#without-a-keychain). The account is `local-` and 12 hex digits of the data directory's hash, so it never meets a Docker install's item. Layout and commands: [../README.md](../README.md#run-without-docker).

Each Docker install has its own keychain item: service `connector-engine`, account the compose project name (`PROJECT`, default `connector-engine`).

| Host | Store | Notes |
|---|---|---|
| macOS | login keychain through `/usr/bin/security` | the item is readable by `security` without a prompt, so by any process of the user |
| Linux | Secret Service (gnome-keyring, KWallet) through `secret-tool` (package `libsecret-tools` or `libsecret`) over D-Bus | any process of the user on the session bus can read it while the collection is unlocked |
| Windows | DPAPI, `%LOCALAPPDATA%\connector-engine\<project>.unlock`, bound to the Windows user | run `make` from Git Bash or WSL with Docker Desktop |

## Without a keychain

When `bin/cnctl` cannot be built (no Go on the host) or the host has no keychain, `make unlock` says so and tells a fresh store to keep a plaintext `root.key`, as `localfile` does. The console shows a warning while either root key is a plaintext file. A store already wrapped never falls back: without its unlock key it stays locked, and the plaintext choice is refused.

## Migrating an existing install

`make keychain-migrate` (idempotent):

1. creates the unlock key in the keychain if missing and reads it back;
2. delivers it, then asks each container to migrate: write `root.key.wrapped`, read it back, check it equals the live root and unwraps every tenant key in `keystore.json`, and only then remove `root.key`;
3. restarts the engine and the console, unlocks them from the keychain and checks both report `wrapped`.

A failure before the removal leaves `root.key` in place and the store running on it; run the command again. If both files exist after a crash, the plaintext one wins and the next migration finishes the job. `make keychain-revert` writes `root.key` back (checked the same way) and removes the wrapped file; the keychain item stays.

Recovery: `make keychain-recovery` prints the unlock key once, for a password manager. On a new machine, `make keychain-import` reads it on stdin into that machine's keychain. Without the unlock key and without a backup of `root.key`, the records cannot be opened: that is the point.

## 1Password

`builtin:onepassword` reads the unlock key once at start from one field of one 1Password item, through a [1Password Connect](https://developer.1password.com/docs/connect/) server you run (`GET /v1/vaults/{vault}/items/{item}`, bearer token, the item's `fields[].label` and `value`). The adapter is tested against a fake of that endpoint, not against a live Connect server. Then it is the same wrapped `secrets/local` store. Nothing else talks to 1Password and nothing is written back. It is opt-in and has no default.

1. Deploy a Connect server reachable from the engine (for example in the same compose network). Create a Connect token that can read one vault.
2. In the 1Password app, add an item `connector-engine-unlock` with a password field labelled `unlock` that holds 64 hexadecimal characters (for example `openssl rand -hex 32`, pasted into the app rather than written into a shell command, where it would sit in argv and the history). Note the vault id and the item id (`op item get connector-engine-unlock --format json`).
3. Put the Connect token in a file only the engine reads, then configure:
   ```yaml
   # connector-engine.yaml
   secrets:
     dir: /var/lib/connector-engine/secrets
     onePassword: {connectUrl: "http://op-connect:8080", tokenRef: "file:/run/connector-engine/op-connect-token", vault: <vault id>, item: <item id>, field: unlock}
   # adapters.yaml
   ports:
     secrets: {use: builtin:onepassword}
   ```
   No `plaintext_root_key` acknowledgement is needed.
4. Start the engine.
   - A fresh directory gets a wrapped root.
   - A directory with a plaintext `root.key` is migrated at the first start, with the same verified steps as `make keychain-migrate`. Selecting the adapter is the consent.
   - Keep a copy of the field: without it the records cannot be opened.

Limits:
- The engine fails to start when Connect is unreachable, the token is refused, or the field is missing or not 64 hexadecimal characters.
- Rotating the item field is not supported: the value in 1Password must stay the one the root was wrapped with.
- Service account tokens are not supported. They need the `op` CLI or an SDK, and the distroless image ships neither; a Connect server is the path inside the container.

| Threat | `onepassword` |
|---|---|
| Copy of the volumes, disk image, backup | nothing opens without the item |
| Malware as the owner, root, the Docker socket | the Connect token file is in the volume next to the engine: whoever reads it can ask Connect for the field, while Connect is up and the token is valid |
| Compromised engine process | the key is in memory while it runs |

## Threat model

| Threat | `localfile` | `hostkey` |
|---|---|---|
| A copy of the Docker volumes, a disk image, a backup or a stolen disk | the copy holds `root.key`: everything opens | the copy holds only wrapped keys: nothing opens without the keychain item |
| Another local user without root | refused by 0600 and Docker's root-owned volume directory | same, and the keychain is per user |
| Malware running as the owner | reads nothing in the root-owned volume, but can use Docker if the user is in the `docker` group | can read the keychain item (no prompt on Linux and for `security` on macOS) and use Docker: not protected |
| Root on the host, or anyone with the Docker socket | everything | everything: they can read the container's memory or run `exec` |
| A compromised engine or console process | the opened records | the same: the key is in memory while it runs |
| Logs, `docker inspect`, process lists | no key | no key: pipes only |

A full backup ([console.md](console.md#backups)) holds both root keys in plaintext inside a file encrypted under your passphrase (Argon2id, AES-256-GCM), so it opens on a machine without the old keychain. The import wraps them again with the new machine's key. Whoever holds the file and the passphrase holds every record: the passphrase is that file's only protection. To take the snapshot, the console reads the engine's keys in plaintext from the operator listener on the compose network (`server.snapshot: true`, person profile only). Whoever holds the operator token in the `secrets` volume and reaches that network can do the same, as they could already drive every operation of the engine.

What it does not do: protect a running, unlocked install; protect against the owner's own account being compromised; replace backups (keep the recovery key).
