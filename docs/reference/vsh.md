![The command line, vsh](../assets/section-vsh.svg)

# vsh

`vsh` is the command line of connector-engine. It drives the operator and management API over HTTP, and acts as one agent over MCP.

```text
vsh admin [--url URL] [--token TOKEN] <group> <verb> [path values] [--field value] [--json BODY]
vsh admin call <operationId> [path values] [--json BODY]
vsh tools list [--out FILE]
vsh tools search QUERY [--limit N] [--out FILE]
vsh tools call NAME [--args JSON] [--fields /a,/b] [--out FILE]
vsh skills export --to DIR [--skill NAME] [--prune]
```

The self-hosted build (`selfhosted/cmd/vsh`, the one binary of the `person` archive, also in the image) adds:

```text
vsh up [--open] [--port N] [--data DIR] [--no-keychain]
vsh stop | status [--data DIR]
vsh export [--setup] [--out FILE] [--yes] | import FILE [--yes]
vsh reset-password [--user NAME]
vsh uninstall --keep-data | --purge [--yes] [--no-keychain]
vsh keychain get|ensure|import|delete [--name NAME]
vsh notify [--console URL] [--key-file FILE]
```

Management, as the signed-in user of the console:

```text
vsh status [--data DIR]
vsh login [--user NAME] [--password-stdin] | logout
vsh connectors [list] | show NAME | import URL|FILE [--mcp] [--local] [--yes] | remove NAME [--yes]
vsh keys [list] | create NAME [--ttl 720h] | revoke NAME [--yes]
vsh people [list] | invite owner|admin|member | role USERNAME ROLE | remove USERNAME [--yes]
vsh access CONNECTOR [FUNCTION] on|off|ask [--key NAME] [--kind read|write|destructive|undeclared]
vsh access CONNECTOR --key NAME follow|custom
vsh approvals [list] | approve ID | deny ID
vsh activity [--connector NAME] [--key NAME] [--since 1h] [--follow]
vsh skills list
```

Every management command takes `--json` and `--url`.

The headless archive ships a core-only `vsh` (admin, tools, skills) with no self-hosted code, next to the `connector-engine` engine.

## Help and exit codes

`vsh --help`, `vsh <command> --help` and `vsh <command> <subcommand> --help` print one format: a `Usage:` line, a sentence, and the commands or flags. Flags may come before, between or after the arguments; `--` ends them. An error prints `vsh: <what went wrong>` and, when there is one, `next: <what to run>`.

| Code | Meaning |
|---|---|
| 0 | done |
| 1 | the command failed |
| 2 | wrong usage: an unknown command, flag or argument |
| 3 | nothing to talk to: no install runs on the data directory, the console does not answer, or no OS keychain for `vsh keychain` |
| 4 | not allowed: not signed in, the session ended, a wrong password, or the signed-in user may not do this |

## Sign in and manage

The management commands talk to the console's HTTP API, the one the web console uses, so its checks, roles, locks and audit apply unchanged. `vsh` acts as the user who signed in: a user whose role does not allow a change gets exit code 4 and the console's reason.

The console is found in this order: `--url`, `$VSH_URL`, the install without Docker running on the default data directory (`$CN_DATA_DIR`), then `http://127.0.0.1:7465`.

```sh
vsh login
```

```text
Username: me
Password:
signed in to http://127.0.0.1:7465 as me; the session is kept in /home/me/.config/vsh/sessions/session-f448b069f1615604.json
```

- `vsh login` takes the username and password of the web console and opens an ordinary console session. The password is typed with echo off, or read from the first line of stdin with `--password-stdin`; it is never an argument.
- The session is kept in the OS keychain (service `vsh`, one item per console address; never the `connector-engine` items of the unlock keys), or, without a keychain or with `$VSH_NO_KEYCHAIN` set, in a file readable only by you under `$VSH_CONFIG_DIR` or the user config directory (`~/.config/vsh/sessions/` on Linux). `vsh` never prints it.
- `vsh logout` ends the session on the console and forgets it here. `vsh reset-password` signs out every session of that user, `vsh`'s included.
- `vsh keys create` prints the new access key once, on stdout, because that key is what you give to the agent. The session itself is never printed.

`vsh status` shows the install, the console and who is signed in:

```text
install  running   pid: 41907   version: dev   data: /home/me/.local/share/connector-engine
console  http://127.0.0.1:7465   agent endpoint: http://127.0.0.1:7465/mcp
user     me
```

### connectors

```sh
vsh connectors import --mcp --local http://127.0.0.1:7999/mcp --yes
vsh connectors
vsh connectors show check-notes
```

```text
Check notes (mcp) from 127.0.0.1:7999
  write        ask   create
  undeclared   on    mystery
  destructive  ask   purge
  read         on    search
installed Check notes
NAME         TITLE        FUNCTIONS  KEYS  STATE
check-notes  Check notes  4          0     enabled
```

`import` reads a listing URL, a listing file on this machine (at most 60 KiB, sent to the console as `listing_text`), or an MCP server's address with `--mcp` (`--local` allows one on this machine or its network). A skill comes from its catalog URL, never a file. A connector installed from a file has no listing address, so a setup export points at its server's address instead, as it does for any connector without a recorded listing. It shows what the console proposes and asks before it installs, unless `--yes`. A connector that needs a sign-in is signed in to from the console's Connectors page. `remove` asks before it deletes the connector and its credentials.

### keys

```sh
vsh keys create claude-code
vsh keys
vsh keys revoke claude-code --yes
```

```text
access key of claude-code, shown this once; give it to the agent as CN_AGENT_TOKEN with CN_ENDPOINT=http://127.0.0.1:7465/mcp
eyJhbGciOi…
NAME         EXPIRES           VIA
claude-code  2026-12-20 20:45  key
```

### people

```sh
vsh people invite member
vsh people
vsh people role bob admin
vsh people remove bob --yes
```

`invite` prints a link once, on stdout: it works once, for 72 hours, on the console's own address. Only an Owner invites, changes roles or removes people. Removing a person signs them out and stops their keys now.

### access

```sh
vsh access check-notes --kind write ask
vsh access check-notes search ask
vsh access check-notes --key claude-code custom
vsh access check-notes purge off --key claude-code
vsh access check-notes --key claude-code follow
```

```text
check-notes: ask
check-notes search: ask
check-notes for claude-code: custom
check-notes purge for claude-code: off
check-notes for claude-code: follow
```

- Without `--key`, it sets the connector's own access, which every key that follows the connector gets.
- With `--key`, it sets that key's override, or with `follow` or `custom` the key's profile on the connector.
- Without a function it sets every function, or with `--kind` every function of that kind, in one request.

### approvals and activity

```sh
vsh approvals
vsh approvals approve 18eb5f5700a4eb5f6bfadba9a5091f44
vsh activity --since 10m
vsh activity --follow --key claude-code
```

```text
ID                                KEY          CONNECTOR    FUNCTION  EXPIRES
18eb5f5700a4eb5f6bfadba9a5091f44  claude-code  check-notes  search    20:55:15
approved 18eb5f5700a4eb5f6bfadba9a5091f44
2026-11-20 20:45:15  claude-code  check-notes  search  ok  1ms
```

`--follow` asks the console every `--every` (2s) and prints each new call, and a call again when its outcome changes, until interrupted.

## The full-screen vsh

`vsh` with no command in a terminal, or `vsh ui`, opens a full-screen view of the same console API, as the signed-in user (run `vsh login` first). It fits 80×24 and uses the extra room of a wider terminal.

| Screen | Keys |
|---|---|
| all | `tab` / `shift+tab` or `1`–`4` switch screens, `q` quits, `esc` goes back or quits, `ctrl+c` quits |
| Connectors | `↑` `↓` move, `/` filters, `enter` opens a connector |
| a connector | its functions by read, write, destructive and undeclared; `←` `→` or `space` set Off, On or Ask; `[` `]` switch between Default and each key; `f` makes a key Follow or Custom; `r` all read On, `w` all write Ask, `x` everything Off, each in one request |
| Keys | `n` creates a key and shows it once, `d` revokes one after a `y` |
| Approvals | `a` approves, `d` denies |
| Activity | the calls of the last hour, newest last, every 2 seconds |

A change shows at once and is sent to the console; when the console refuses it, the screen goes back to what it was and the footer gives the reason. Approvals and activity refresh every 2 seconds.

## Lifecycle without Docker

`vsh up` runs the engine and the console in its own process, in the foreground, with a control socket in the data directory that `stop`, `status`, `export`, `import`, `reset-password` and `uninstall` talk to. See [the person profile](../self-hosted/person.md#without-docker).

## admin

Reads `VSH_ADMIN_URL` and `VSH_ADMIN_TOKEN`, or `--url` and `--token`. The operator listener and token run installation-wide operations (tenants, adapters, event types); tenant data (connectors, grants, approvals, calls, roles) needs the public listener and a person token whose role allows it, or the answer is `403 permission_denied: <who> may not <permission> on <scope>`.

```sh
export VSH_ADMIN_URL=http://127.0.0.1:8081 VSH_ADMIN_TOKEN=dev
vsh admin tenants create t1
```

```text
{
  "id": "t1",
  "state": "active",
  "key_ref": "local:t1:1",
  "created_by": "system",
  "created_at": "2026-11-20T21:09:45.216608135Z"
}
```

| Group | Verbs |
|---|---|
| `listings` | `list get create lint review quarantine deprecate withdraw restore offered starter` |
| `sources` | `sync list` |
| `calls` | `list` |
| `secrets` | `rotate` |
| `issuers` | `client` |
| `connectors` | `list get install enable disable update level remove` |
| `functions` | `list accept classify enable disable projection` |
| `bindings` | `list put delete level` |
| `grants` | `list create delete confirm` |
| `approvals` | `list get decide cancel` |
| `overlays` | `list suggest propose accept restore clear` |
| `tenants` | `create remove` |
| `roles` | `list put delete` |
| `groups` | `list put delete` |
| `operations` | `get` |
| `adapters` | `list` |
| `events` | `list get` |

`vsh admin --help` prints this list from the binary. `call <operationId>` reaches every operation of the [API](api.md), including those without a group.

In the `person` profile, `mise run admin -- tenants create t2` runs it from the image with the generated operator token.

## tools

Reads `CN_ENDPOINT` (the `/mcp` address) and `CN_AGENT_TOKEN` (an access key or agent token). An MCP client for one agent.

```sh
export CN_ENDPOINT=http://127.0.0.1:7465/mcp CN_AGENT_TOKEN=<access key>
vsh tools search "create issue" --limit 3
vsh tools call github__create_issue --args '{"title":"Hello"}' --out result.json
```

- It writes the full answer to `--out` and prints a summary.
- A held call prints its `approval_id`; poll it with `vsh tools call approval_status --args '{"approval_id":"<id>"}'`.
- In the `person` profile, `mise run cli -- tools list` runs it from the image.

## skills export

Writes every skill granted to the agent into `DIR/<skill>/`. See [skills](../using/skills.md#export-skills-to-an-apps-folder).

## keychain and notify

| Command | Does |
|---|---|
| `vsh keychain ensure --name connector-engine` | creates the unlock key in the OS keychain if missing |
| `vsh keychain get --name connector-engine` | prints it (used by `mise run unlock` on a pipe) |
| `vsh keychain import --name connector-engine` | reads it on stdin |
| `vsh keychain delete --name connector-engine` | removes it |
| `vsh notify` | desktop popups for calls waiting for approval (`mise run notify`) |

`--name` is the compose project (`PROJECT`, default `connector-engine`). See [the keychain](../self-hosted/keychain.md).
