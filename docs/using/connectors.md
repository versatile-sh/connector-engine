![Connectors, pinned](../assets/section-connectors.svg)

# Connectors

A connector is one tool server behind the gateway. AI apps never reach it directly: every list and every call goes through the engine, which checks permissions, injects the credential and records the call.

## Kinds

| Runtime | Talks to |
|---|---|
| `remote` | an MCP server over HTTP |
| `declarative` | a REST API described by OpenAPI |
| `grpc` | a gRPC service, through reflection or the Buf Schema Registry |
| `skill` | a skill package (files, no server); see [skills](skills.md) |

In a `declarative` function, a value that fills a `{name}` part of the path accepts letters, digits and `- . _ ~` only, and is never empty or only dots. Any other value is refused before a request is sent.

Keys differ only in permissions ([functions and permissions](permissions.md)). Whose credential a call uses is the connector's credential mode.

## Credential mode

In the console, an Owner or Admin sets each connector's mode (`PUT /api/connectors/{id}/credential-mode` with `{"mode": "shared"}` or `{"mode": "per_person"}`). The same setting decides who approves its held calls.

| Mode | The call uses | Engine binding | Who approves |
|---|---|---|---|
| `shared` (default) | the one workspace credential an Owner or Admin signs in with | `credential_policy: shared`, `ask_requester: true`, `approvers: [<managers group>]` | the key's owner, or any Owner or Admin |
| `per_person` | the signed-in user's own credential | `credential_policy: per_user`, `ask_requester: true`, no other approvers | the key's owner only |

- The listing's `owner_kinds_allowed` limits the choice: without `user`, only `shared` is offered; without `workspace`, only `per_person`, and a new connector starts there. Connectors made from a plain MCP address with a header secret allow both.
- In `per_person` mode, "Sign in" and a header secret are stored for the signed-in user only (`owner_kind: user`). Nobody can store a credential for another person. A call from a key whose owner has not signed in gets `credential_required`.
- Changing the mode rebinds every key of the connector at once. A call already held keeps the approvers it was held with. Switching to `per_person` keeps the shared credential stored, unused; switching back uses it again.
- Owners and Admins see how many people signed in (`signed_in_count` of `people_count`).

## Names an app sees

| Item | Listed as |
|---|---|
| function | `<alias>__<name>` |
| prompt | `<alias>__<name>` |
| resource | `<alias>+<uri>` |

The separator (`gateway.separator`, `__`) and the longest name (`gateway.toolNameMax`, 64) are [configuration keys](../reference/config.md).

## Pinning and quarantine

- Each function definition is hashed (RFC 8785 canonical JSON) when it is captured.
- A new function, or a changed description or schema, is **quarantined** until you accept it.
- A permission made against an older hash goes stale and needs confirming again.
- Re-lists run on a schedule (`pinning.relistMin` to `pinning.relistMax`), so a server that changes its tools after install is caught.

In the console, a quarantined function says "The definition changed" on the connector's page; choosing on or ask accepts the current definition. Through the API, with a `tenant_admin` person token:

```sh
vsh admin functions accept <connector id> <function name>
```

## Server instructions, prompts and resources

| Item | Setting in the `person` profile | Behaviour |
|---|---|---|
| server instructions | `gateway.relayInstructions: pinned` | kept with the connector, quarantined until accepted and again on change; once on, apps read them in the engine's own instructions |
| prompts | `gateway.upstreamContent: granted` | proposed off, because a prompt's text arrives in the chat as if you wrote it |
| resources | `gateway.upstreamContent: granted` | proposed on |

Each item is off, on or ask like a function, and quarantined on change. The engine default and the `headless` profile refuse all three.

## Remove a connector

**Remove connector** on the connector's page drops it and its permissions. Export a [setup](account-backup.md#export-and-import) first if you may want it back.
