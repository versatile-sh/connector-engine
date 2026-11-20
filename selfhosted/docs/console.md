# The console

The console is the local web app of the `person` profile. It talks to the engine over HTTP only. Start it with `make up` and open `http://127.0.0.1:<CONSOLE_PORT>` (default 7465).

## Port and host

- `CONSOLE_PORT` is the only port setting. Put it in `deploy/.env` to keep it.
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
