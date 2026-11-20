# The console

The console is the local web app of the `person` profile. It talks to the engine over HTTP only. Start it with `make up` and open `http://127.0.0.1:<CONSOLE_PORT>` (default 7465).

Its design system (tokens, typography, components, responsive rules) is in [`console/web/DESIGN.md`](../console/web/DESIGN.md).

## Port and host

- `CONSOLE_PORT` is the only port setting. Put it in `selfhosted/deploy/.env` to keep it.
- It sets the published port, the Host allowlist (`127.0.0.1`, `localhost` and `[::1]` at that port) and the OAuth redirect `http://127.0.0.1:<port>/oauth/callback`.
- Outside Docker:

```sh
connector-console --listen 127.0.0.1:<port> --engine http://127.0.0.1:8080 \
  --engine-operator http://127.0.0.1:8081 --operator-token-file <file> --state <file>
```

## Connecting AI apps

AI apps follow the MCP authorization spec: the app registers itself, opens the console in your browser, you allow it once, and it gets a token and a refresh token.

| Endpoint | Role |
|---|---|
| `/.well-known/oauth-protected-resource[/mcp]` | names the console as the authorization server |
| `/.well-known/oauth-authorization-server` | describes it |
| `/oauth/register` | dynamic client registration (RFC 7591) |
| `/oauth/authorize` | PKCE S256 only |
| `/oauth/token` | `authorization_code` and rotating `refresh_token` |

- Accepted redirects: loopback `http` on any port, and app schemes such as `cursor:` or `vscode:`. The only `https` redirect is Antigravity's own `https://antigravity.google/oauth-callback`, listed exactly; the code is useless without the app's PKCE verifier.
- Allowing an app creates an access key named after it, so grants, the key list and Revoke work as for any key.
- Access tokens are engine agent tokens that live one hour. The console keeps registered clients and the SHA-256 of refresh tokens (90 days) in its state file.
- Clients without OAuth (your scripts, `cnctl`) use an access key made on the **Access** page, which shows the exact command or config for each client.
- Opening `/mcp` in a browser leads to the Access page.

## Install links

| Link | Opens |
|---|---|
| `/import?mcp=<https MCP URL>` | a review of an MCP server |
| `/import?listing=<https URL of a listing>` | a review of a listing in the Manifest format |
| `/import?listing=<https URL of a skill catalog entry>` | a review of a skill (`kind: skill`) |

The review shows the origin, title, icon, publisher and each function's proposed choice, after sign-in. Nothing is installed before you confirm.

## Approvals

A function set to "ask" holds the call until a person decides. Besides the console's Approvals page, an answer can come from:

- the AI app itself (MCP elicitation, turned on per app);
- a desktop popup (`make notify`);
- Discord, Slack or Telegram with buttons, or a webhook (ntfy, Discord, Slack).

Set them up under **Notifications**; nothing is sent until you add one. Design and threat model: [approvals-surfaces.md](approvals-surfaces.md).

## Skills

- After you confirm a skill link, each file is fetched over public https, checked against its sha256 and pinned to `source.commit`.
- A branch name, `..`, an absolute path, or a size over the limits (256 files, 1 MiB a file, 4 MiB a skill) is refused.
- The review shows the licence, the source at its pinned commit, the file list with sizes, and a warning when the skill carries scripts.
- The **Skills** tab sets each skill off, on or ask per access key. Keys made later start off.
- AI apps see two tools, `skill_list` and `skill_load`, and a short index of their skills in the server instructions. The gateway serves skill files as text and never runs them.
- A skill without a redistributable licence is `metadata_only`: `skill_load` answers with its source instead of its files.
- With `gateway.skills: tools+prompts`, every skill is also a prompt, so an app can offer it as a slash command.

## Server instructions, prompts and resources

| Item | Setting in the `person` profile | Behaviour |
|---|---|---|
| server instructions | `gateway.relayInstructions: pinned` | kept with the connector, quarantined until a person accepts them, quarantined again on change; once on, apps read them in the engine's own instructions |
| prompts | `gateway.upstreamContent: granted` | listed as `<alias>__<name>`; proposed off, because a prompt's text arrives in the chat as if you wrote it |
| resources | `gateway.upstreamContent: granted` | listed as `<alias>+<uri>`; proposed on |

Each item is off, on or ask like a function, and quarantined on change. The engine default and the `headless` profile refuse all three.

## Activity

- **Activity** lists every call an app made through the console's `/mcp`, per hour and per connector. A call opens with its arguments and a result excerpt.
- Records go to `activity/YYYY-MM-DD.jsonl` beside the state file, mode 0600, kept 30 days. The engine itself keeps only a hash of the arguments.
- Arguments are recorded by default; result excerpts (4 KB) only when turned on. Likely secrets (token, password, secret, authorization, API key, bearer and key-shaped values) are replaced before anything is written.
- "Delete all activity" removes the files and hides older engine call records from the page.

## Account

- **Account** changes the username and the password. A password change asks for the current one, needs at least 10 characters, signs out every other session and keeps this one. Wrong current passwords are rate-limited like the login: 5 a minute.
- Forgot the password? `make reset-password` on the machine that runs the console asks for a new one twice, without echo. It reaches the console through a socket that only `docker compose exec` can open, so access to this machine is the authority. The password travels on stdin, never in argv or the environment. Every session is signed out.

## Backups

Account → **Export** offers two files.

| Mode | File | Holds | For |
|---|---|---|---|
| Full backup, with credentials | `connector-engine-backup-<UTC time>.cebak` | everything: the engine's rows and keys, the console's state, keys and activity | moving or restoring your own install |
| Shareable setup, without credentials | `connector-engine-setup-<UTC time>.setup.yaml` | each connector's source and choices, the skills with their catalog address, pinned digest and default access, the kinds of notification channel | handing your setup to a friend or a teammate |

A **full backup** holds every credential of the install: provider sign-ins and API keys, the access keys and the root keys, the account and its activity. The console asks you to confirm that, and asks for your password and for a passphrase, twice. `make export` prints the same warning and asks y/N (`YES=1` skips the question). It writes to `~/connector-engine-backups/` and keeps the 7 newest.

**Keep the passphrase safe.** Nobody can recover it, the maintainers included. Without it the file opens for nobody. With it the file opens for anyone. Store the file like a password manager export, and never share it.

The file is plain bytes: a header line, then AES-256-GCM ciphertext.
- The header carries the format version, the Argon2id parameters (3 passes, 64 MiB, 4 lanes), a 16-byte salt, the nonce, and a check value that tells a wrong passphrase from a damaged file.
- The header is authenticated as additional data.
- Inside is a tar archive with a manifest (format, engine build, time, counts, the SHA-256 of every file), the engine's snapshot from its operator listener (`GET /v1/snapshot`), the console's state without sessions, its key store, and its activity files.

Account → **Import**, or **Restore from a backup** on the first-run screen, or `make import FILE=...`, reads either kind of file and tells them apart by their first bytes.
- A **full backup** is checked first: the passphrase, the integrity, every checksum and the version. A file from a newer format is refused. Next you see what it holds (connectors, skills, keys) and confirm that it replaces this install's data.
  - One restore runs at a time. The console stages its keys and activity. Then the engine stages its keys, replaces its rows in one transaction, and puts its keys in place. Only when the engine succeeds does the console put its own files in place.
  - A refusal anywhere up to the engine's commit leaves the install as it was: a wrong file, keys that do not open their state, rows the database refuses. After the commit only file renames on the same volume remain. The engine's root key follows the crash recovery of a rotation. A disk that fails exactly there can leave the keys behind the rows: restore the same backup again.
  - The root keys are written in this machine's form: under the person profile they are wrapped again with this machine's keychain key.
  - The engine and the console then restart on the restored keys. Run `make unlock` to finish (`make import` does it for you), and the console page moves on by itself. Sign in with the password of the restored account.
  - Importing the same backup again gives the same result.
- Signed in, a full import asks for your password again. The first-run restore works only while no owner account exists.
- A **setup** shows what it would add, then adds each connector and skill through the normal import: the server is probed, functions are pinned, and a skill whose digest changed is refused. An entry that is already installed keeps its choices, and nothing is deleted. Afterwards, sign in to each added connector.

A setup never holds credentials, tokens, API keys, OAuth clients, access keys, root keys, account data, activity or device pairings.
- Every address loses its user name and password, its query and its fragment.
- An address whose path holds a token-like segment (20 or more letters, digits, `-` or `_`, with a digit) is left out.
- The file lists each removal under `stripped`, naming only the site. A skill imported before the console recorded catalog addresses is left out and listed there too.

## Keys and state

| Item | Where |
|---|---|
| operator token, Postgres passwords | written on first boot to the `secrets` volume |
| owner account (PBKDF2-SHA256) | the `console-data` volume |
| notifier tokens and other console secrets | sealed by `selfhosted/console/sealed` (envelope encryption, keys in `console-data/secrets`); the state file holds only sealed records |
| the key that signs approval links | sealed the same way; while the console is locked, a link answers 503 until `make unlock` |
| engine and console root keys | wrapped by an unlock key in the OS keychain (macOS Keychain, Secret Service, DPAPI), delivered by `make unlock`; without a keychain they stay plaintext files and the console warns |

`PROJECT` names the compose project and the keychain item. Delivery, migration and threat model: [SECRETS.md](SECRETS.md).

| Command | What it does |
|---|---|
| `make keychain-migrate` / `keychain-revert` / `keychain-status` | move a plaintext `root.key` under the keychain (verified, idempotent), back again, or report |
| `make keychain-recovery` / `keychain-import` | print the unlock key for a password manager, or restore it from stdin |
| `make cli ARGS="tools list"` | `cnctl` from the image against the console's `/mcp`; set `CN_AGENT_TOKEN` |
| `make admin ARGS="tenants create t2"` | `cnctl admin` against the operator listener with the generated operator token |
