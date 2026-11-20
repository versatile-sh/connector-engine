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

## Access keys on a connector

A connector keeps one credential that every key shares; keys differ only in permissions. The **Profile** picker above a connector's functions shows **Default (all keys)**, the connector's own off, on or ask that every key follows, or one key's profile. `?profile=<key>` in the address opens that key's.

| Mode | The key | Engine rows |
|---|---|---|
| Off | cannot see the connector: no tool, prompt or resource is listed, and a call answers like an unknown tool | no binding |
| Follow default | gets the default choices and follows every change to them | a binding and the agent wildcards (`*`, `instructions:*`, `prompt:*`, `res:*`) |
| Custom | narrows some functions and items below the default; every other row follows it | the same, plus an agent-scoped `fn:` or item row for each narrowing |

- An override can only narrow. The engine takes the stricter of the workspace row and the key's row, so a key never gets more than the default; choices above it show but cannot be taken. Choosing the default's own value, or **Reset to default**, removes the override.
- An override is kept while the default drops below it, and applies again when the default comes back.
- A row whose engine answer differs from the console's choice says so, with the engine's reason (for example `forced_ask` from the exposure check). The console reads it from `/v1/access:explain`.
- Off unbinds the key, which drops its overrides; turning it back to custom starts from the default.
- **Every key** sets all keys to off or the default in one request, which reports each key's result; either drops every key's overrides.
- Off or the default asks first, in place, when it would drop overrides; with none to lose it applies at once.
- **New keys start off** makes keys made from then on start off on this connector; by default they follow it. Renewing a key keeps its profile on every connector.
- **Always allow this app** on a held call allows that function for the calling key only: every other key, and every key made later, keeps the function's former access as its own override, then the default rises to on and the calling key's override is removed. Setting the function's default again lifts the limit for keys made later. The pins are ordinary overrides: resetting a key to the default, copying a profile onto it, or turning it off and back on removes them, and the key then follows the raised default. A failure shows on the approval page, which keeps reading the call for three minutes after an allow; the chat surfaces have already said "Always allowed" by then, and the failure is kept in memory only, until the console restarts.
- The **Access** page lists, for each key, every connector with its mode and a link to its profile.
- Many at once: tick functions (or select all the filter shows), then set them to off, on or ask, or back to the default in a key's profile, in one request that says which were not changed. The **More…** menu holds the class presets (all reads on, all changes ask, all deletes off) for the profile shown, and copies a key's profile: from another key onto the one shown, or from the default onto several keys at once.

| Console endpoint | Does |
|---|---|
| `PUT /api/connectors/{id}/functions` | `{"names": [...], "access": ...}` sets many functions for the default in one request; answers each name's result, and one entry per key it failed to give the new item rows |
| `PUT /api/connectors/{id}/profiles` | `{"mode": "off" \| "follow" \| "custom", "keys": [...]}`, every key when `keys` is empty; answers each key's result |
| `PUT /api/connectors/{id}/profiles/{key}` | `{"names": [...], "access": ...}` narrows functions or items for one key, or `{"names": [...], "reset": true}` returns them to the default; answers each name's result |
| `POST /api/connectors/{id}/profiles:copy` | `{"from": key, "keys": [...]}` gives other keys one key's mode and overrides; answers each key's result |
| `GET /api/connectors/{id}/profiles/{key}` | the engine's answer for that key on each function and item |
| `PUT /api/connectors/{id}/new-keys` | `{"off": true}` makes new keys start off |
| `GET /api/slots` | the permission sets waiting to be given to a key, each with its mode and overrides on the connectors still installed |
| `POST /api/slots/{name}/claim` | `{"key": ...}` gives a key made already the set's profile on each of its connectors, then forgets the set |
| `DELETE /api/slots/{name}` | discards a permission set |
| `POST /api/agents`, `POST /api/oauth/requests/{id}` | also take `"slot": name`: the key made by hand, or by an app's consent, gets that set's permissions and the set is consumed |

Batches name at most 1000 functions, items or keys; a larger one is refused before anything is written. Writes to one connector run one at a time, including install, removal and revoking a key, and a request that stops waiting gives up its turn.

A key's profile is applied closed: the key is unbound, its overrides are written, and it is bound last, so a failure halfway leaves it off rather than wider, and making the key again applies the whole profile.

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
| Shareable setup, without credentials | `connector-engine-setup-<UTC time>.setup.yaml` | each connector's source and choices, whether its new keys start off and each key's profile there, which the importer receives as reusable permission sets named after the keys; the skills with their catalog address, pinned digest and default access, the kinds of notification channel | handing your setup to a friend or a teammate |

A setup carries **permissions as reusable sets**; a backup carries **your exact keys**.

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
  - The review shows each new connector's host, whether it is `local` (it reaches a private or loopback address on this machine), whether its new keys start off, and each choice the file sets above what this console proposes, with the value it gets. Every choice is capped at this console's proposal, local connectors included, and the result lists what was lowered.
  - Keys made already start off on the connectors and skills a setup adds, unless the review's **Give my existing keys the new connectors** is on. The review asks remote connectors for their proposals only for a signed-in owner.
  - The file names each key without the OAuth client id an app's key may carry. Each key of the file becomes a **permission set** (a key slot): its name, such as `claude-code`, and its mode and overrides on each connector the import added, never a credential. Nothing binds by name: a key here named `claude-code` is a different key. Per set the review chooses **Keep** as a set (the default), **Apply** to an existing key picked from a list, or **Discard**. A set whose name is taken gets the first free `-2`, `-3` suffix. `make import` takes the defaults: every set kept, existing keys off, choices capped.
  - A kept set is claimed later: on the consent page when an app connects (**Use permissions from**), when making a key by hand on Access, or with **Apply to key…** in Access's **Unclaimed permission sets** and on each connector's page. Claiming gives the key exactly the set's profiles, within the connector's choices and its new-key limits, and consumes the set; **Discard** drops it. Removing a connector drops it from every set.
  - A full backup restores the keys exactly as they were and the sets as they were; it creates none. Profiles that an earlier console kept waiting for a key name become sets when the state file or an old backup is read.

A setup never holds credentials, tokens, API keys, OAuth clients, access keys (only their names, as the names of permission sets), root keys, account data, activity or device pairings.
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
