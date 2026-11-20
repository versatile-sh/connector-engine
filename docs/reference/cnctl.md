![The command line, cnctl](../assets/section-cnctl.svg)

# cnctl

`cnctl` is the command line of connector-engine. It drives the operator and management API over HTTP, and acts as one agent over MCP.

```text
cnctl admin [--url URL] [--token TOKEN] <group> <verb> [path values] [--field value] [--json BODY]
cnctl admin call <operationId> [path values] [--json BODY]
cnctl tools list [--out FILE]
cnctl tools search QUERY [--limit N] [--out FILE]
cnctl tools call NAME [--args JSON] [--fields /a,/b] [--out FILE]
cnctl skills export --to DIR [--skill NAME] [--prune]
```

The self-hosted build (`selfhosted/cmd/cnctl`, shipped in the `person` archive and image) adds:

```text
cnctl keychain get|ensure|import|delete --name NAME
cnctl notify [--console URL] [--key-file FILE]
```

## admin

Reads `CNCTL_URL` and `CNCTL_TOKEN`, or `--url` and `--token`. The operator listener and token run installation-wide operations (tenants, adapters, event types); tenant data (connectors, grants, approvals, calls, roles) needs the public listener and a person token whose role allows it, or the answer is `403 permission_denied: <who> may not <permission> on <scope>`.

```sh
export CNCTL_URL=http://127.0.0.1:8081 CNCTL_TOKEN=dev
cnctl admin tenants create t1
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

`cnctl admin` with no group prints this list from the binary. `call <operationId>` reaches every operation of the [API](api.md), including those without a group.

In the `person` profile, `make admin ARGS="tenants create t2"` runs it from the image with the generated operator token.

## tools

Reads `CN_ENDPOINT` (the `/mcp` address) and `CN_AGENT_TOKEN` (an access key or agent token). An MCP client for one agent.

```sh
export CN_ENDPOINT=http://127.0.0.1:7465/mcp CN_AGENT_TOKEN=<access key>
cnctl tools search "create issue" --limit 3
cnctl tools call github__create_issue --args '{"title":"Hello"}' --out result.json
```

- It writes the full answer to `--out` and prints a summary.
- A held call prints its `approval_id`; poll it with `cnctl tools call approval_status --args '{"approval_id":"<id>"}'`.
- In the `person` profile, `make cli ARGS="tools list"` runs it from the image.

## skills export

Writes every skill granted to the agent into `DIR/<skill>/`. See [skills](../using/skills.md#export-skills-to-an-apps-folder).

## keychain and notify

| Command | Does |
|---|---|
| `cnctl keychain ensure --name connector-engine` | creates the unlock key in the OS keychain if missing |
| `cnctl keychain get --name connector-engine` | prints it (used by `make unlock` on a pipe) |
| `cnctl keychain import --name connector-engine` | reads it on stdin |
| `cnctl keychain delete --name connector-engine` | removes it |
| `cnctl notify` | desktop popups for calls waiting for approval (`make notify`) |

`--name` is the compose project (`PROJECT`, default `connector-engine`). See [the keychain](../self-hosted/keychain.md).
