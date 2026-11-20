# Secrets at rest

Every secret the engine keeps (credentials, held call arguments, results, client secrets) is a `port.Sealed` record: a data key per record, wrapped by a key-encryption key per tenant, wrapped by one root key (`secrets/internal/ring`). Postgres never holds plaintext. This page is about the last link: where the root key rests and who can read it.

## Adapters of the `secrets` port

| Adapter | Root key at rest | Who selects it |
|---|---|---|
| `builtin:localfile` (`secrets/local`) | `root.key`, plaintext hex, mode 0600, refused when group or other can read it | the default, the `headless` profile, any config that does not opt in |
| `builtin:hostkey` (`secrets/hostkey`) | `root.key.wrapped`: the root sealed with AES-256-GCM under an unlock key that lives in the host's OS keychain | the `person` profile |
| `memory` | in the process only | tests and dev |

`hostkey` is `secrets/local` with the root file wrapped; the key ring, the rotation files (`root.key.next`, `root.key.next.wrapped`) and the crash recovery are the same code. Selecting it is opt-in (`secrets.adapter: hostkey`, `secrets.unlockSocket`, `ports.secrets.use: builtin:hostkey`); the engine binary never needs it, and the `headless` selection is pinned by `config/profiles_test.go`.

The console seals its own secrets (notifier tokens) through `console/sealed`, a `port.SecretStore` under key scope `console`, with the same adapter: `hostkey` in the `person` profile (`CONSOLE_UNLOCK_SOCKET`), `secrets/local` otherwise. Owner passwords, session cookies and refresh tokens stay hashes.

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
- After a restart (a crash, `docker restart`, a reboot with `restart: unless-stopped`) the container comes back locked and waits on the socket: the engine logs `waiting for the host to deliver the unlock key` and serves nothing; the console serves and shows "locked". `make unlock` opens both. This is the price of never resting the key outside the keychain.

Alternatives rejected: compose `secrets:` without Swarm are bind mounts of a host file, which is a plaintext key on the host disk (and macOS and Windows have no per-user tmpfs). Environment variables show up in `docker inspect` and in `/proc`. A host socket bind mount does not work through Docker Desktop.

Each install has its own keychain item: service `connector-engine`, account the compose project name (`PROJECT`, default `connector-engine`).

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

## Threat model

| Threat | `localfile` | `hostkey` |
|---|---|---|
| A copy of the Docker volumes, a disk image, a backup or a stolen disk | the copy holds `root.key`: everything opens | the copy holds only wrapped keys: nothing opens without the keychain item |
| Another local user without root | refused by 0600 and Docker's root-owned volume directory | same, and the keychain is per user |
| Malware running as the owner | reads nothing in the root-owned volume, but can use Docker if the user is in the `docker` group | can read the keychain item (no prompt on Linux and for `security` on macOS) and use Docker: not protected |
| Root on the host, or anyone with the Docker socket | everything | everything: they can read the container's memory or run `exec` |
| A compromised engine or console process | the opened records | the same: the key is in memory while it runs |
| Logs, `docker inspect`, process lists | no key | no key: pipes only |

What it does not do: protect a running, unlocked install; protect against the owner's own account being compromised; replace backups (keep the recovery key).
