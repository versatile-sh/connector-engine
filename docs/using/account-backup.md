![Account and backup](../assets/section-account-backup.svg)

# Account and backup

The console has one owner account. This page covers the password, full backups, shareable setups, and restoring either.

> **Your data lives in Docker volumes.** `docker compose down -v`, `docker system prune --volumes` and `docker volume prune -a` delete them, and with them every connector, credential, access key and the root keys. Nothing can bring them back. Export a backup first.

## Account

- **Account** changes the username and the password. A password change asks for the current one, needs at least 10 characters, signs out every other session and keeps this one.
- Wrong current passwords are rate-limited like the login: 5 a minute.

Forgot the password? On the machine that runs the console:

```sh
make reset-password                          # without Docker: connector-engine-selfhosted reset-password
```

It asks for the new password twice, without echo, then prints:

```text
password changed; every console session is signed out
access keys and apps signed in through the browser still work: revoke them on the Access page if you suspect a compromise
```

It reaches the console through a socket that only `docker compose exec` can open, so access to this machine is the authority. The password travels on stdin, never in argv or the environment.

## Export and import

**Account → Export** offers two files.

| Mode | File | Holds | For |
|---|---|---|---|
| Full backup, with credentials | `connector-engine-backup-<UTC time>.cebak` | everything: the engine's rows and keys, the console's state, keys and activity | moving or restoring your own install |
| Shareable setup, without credentials | `connector-engine-setup-<UTC time>.setup.yaml` | each connector's source and choices, key profiles by key name, skills with their pinned digest, the kinds of notification channel | handing your setup to a friend or a teammate |

From the command line:

```sh
make export                  # full backup: asks y/N (YES=1 skips it), then a passphrase of 12+ characters twice
make export MODE=setup       # shareable setup, no credentials
```

```text
wrote /home/you/connector-engine-backups/connector-engine-backup-20261120T180000Z.cebak (the 7 newest backups are kept in /home/you/connector-engine-backups)
```

Without Docker (from the next release), the same files come from `connector-engine-selfhosted export` (add `--setup` for a shareable setup), and restore with `connector-engine-selfhosted import FILE`.

### Full backups

A full backup holds every credential of the install: provider sign-ins and API keys, the access keys and the root keys, the account and its activity. The console asks you to confirm that, then for your password and a passphrase, twice.

**Keep the passphrase safe.** Nobody can recover it, the maintainers included. Without it the file opens for nobody; with it the file opens for anyone. Store the file like a password manager export, and never share it.

File format:

- One header line, then AES-256-GCM ciphertext.
- The header carries the format version, the Argon2id parameters (3 passes, 64 MiB, 4 lanes), a 16-byte salt, the nonce, and a check value that tells a wrong passphrase from a damaged file. It is authenticated as additional data.
- Inside is a tar archive: a manifest (format, engine build, time, counts, the SHA-256 of every file), the engine's snapshot (`GET /v1/snapshot` on its operator listener), the console's state without sessions, its key store, and its activity files.

### Shareable setups

A setup never holds credentials, tokens, API keys, OAuth clients, access keys (only their names, in profiles), root keys, account data, activity or device pairings.

- Every address loses its user name and password, its query and its fragment.
- An address whose path holds a token-like segment (20 or more letters, digits, `-` or `_`, with a digit) is left out.
- The file lists each removal under `stripped`, naming only the site.

## Restore

**Account → Import**, **Restore from a backup** on the first-run screen, or:

```sh
make import FILE=~/connector-engine-backups/connector-engine-backup-20261120T180000Z.cebak
```

Either kind of file works; the console tells them apart by their first bytes.

| File | What happens |
|---|---|
| full backup | checked first (passphrase, integrity, every checksum, the version; a newer format is refused), then you see what it holds and confirm that it **replaces** this install's data |
| setup | shows what it would add, then adds each connector and skill through the normal import; an installed entry keeps its choices, and nothing is deleted. Sign in to each added connector afterwards |

A full restore, step by step:

1. One restore runs at a time. The console stages its keys and activity.
2. The engine stages its keys, replaces its rows in one transaction, and puts its keys in place.
3. Only when the engine succeeds does the console put its own files in place.
4. The root keys are wrapped again with this machine's keychain key.
5. The engine and the console restart. `make import` runs `make unlock` for you; from the console, run `make unlock` yourself. Sign in with the restored account's password.

- A refusal before the engine's commit leaves the install as it was.
- Importing the same backup again gives the same result. If a disk fails right after the commit, restore the same backup again.
- Signed in, a full import asks for your password again. The first-run restore works only while no owner account exists.
- A setup's key profile applies at once to a key of that name, or waits for a key of that name to be made. The connector's page lists the waiting profiles.

## Where state lives

| Volume | Holds |
|---|---|
| `pgdata` | the engine's database: connectors, grants, skills, sealed credentials, audit |
| `engine-data` | the engine's key store: the wrapped root key, the key ring and the local issuer's signing keys |
| `console-data` | the owner account (PBKDF2-SHA256), access keys and agents, OAuth clients, the console's sealed secrets and key store, activity, settings |
| `secrets` | what `init` writes once per install: the operator token and the Postgres passwords; not in a backup, since a new install writes its own |

How the root keys are protected: [the keychain](../self-hosted/keychain.md).
