![Functions and permissions](../assets/section-permissions.svg)

# Functions and permissions

Every function of every connector is **off**, **on** or **ask**, for each access key. This page covers the choices, per-key profiles, and the rules the engine adds on top.

## The three choices

| Choice | The AI app |
|---|---|
| Off | does not see the function; a call answers like an unknown tool |
| On | calls it directly |
| Ask | the call waits until you decide; see [approvals](approvals.md) |

A denied function looks exactly like one that does not exist, so an app cannot probe what is there.

## Rules the engine adds

| Rule | Effect |
|---|---|
| destructive functions | "on" becomes "ask once" (or "ask"): you approve it once per session |
| exposure check | a key that holds private data, reads untrusted content and has an outbound path is forced to "ask" |
| catalog ceiling | a listing's visibility caps what any key gets |

The full evaluation order is in [ARCHITECTURE.md](../../ARCHITECTURE.md#grants).

## Per-key profiles

The **Profile** picker above a connector's functions shows **Default (all keys)** or one key's profile. `?profile=<key>` in the address opens that key's.

| Mode | The key | Engine rows |
|---|---|---|
| Off | cannot see the connector: no tool, prompt or resource is listed | no binding |
| Follow default | gets the default choices and follows every change to them | a binding and the agent wildcards (`*`, `instructions:*`, `prompt:*`, `res:*`) |
| Custom | narrows some functions below the default; every other row follows it | the same, plus an agent-scoped row for each narrowing |

- An override can only **narrow**. The engine takes the stricter of the default and the key's row, so a key never gets more than the default.
- Choosing the default's own value, or **Reset to default**, removes the override.
- An override is kept while the default drops below it, and applies again when the default comes back.
- A row whose engine answer differs from your choice says so, with the engine's reason (for example `forced_ask` from the exposure check).
- **New keys start off** makes keys made from then on start off on this connector. By default they follow it. Renewing a key keeps its profile.
- **Always allow** on a held call also removes the calling key's override of that function.
- The **Access** page lists, for each key, every connector with its mode and a link to its profile.

## Many at once

Tick functions (or select all the filter shows), then set them to off, on or ask in one request. The **More…** menu holds class presets (all reads on, all changes ask, all deletes off) and copies one key's profile onto others.

The console API behind these actions:

| Endpoint | Does |
|---|---|
| `PUT /api/connectors/{id}/functions` | `{"names": [...], "access": ...}` sets many functions for the default |
| `PUT /api/connectors/{id}/profiles` | `{"mode": "off" \| "follow" \| "custom", "keys": [...]}`; every key when `keys` is empty |
| `PUT /api/connectors/{id}/profiles/{key}` | `{"names": [...], "access": ...}` narrows for one key, or `{"names": [...], "reset": true}` returns to the default |
| `POST /api/connectors/{id}/profiles:copy` | `{"from": key, "keys": [...]}` copies one key's mode and overrides |
| `GET /api/connectors/{id}/profiles/{key}` | the engine's answer for that key on each function and item |
| `PUT /api/connectors/{id}/new-keys` | `{"off": true}` makes new keys start off |
| `DELETE /api/connectors/{id}/waiting/{key}` | forgets a setup's profile waiting for a key not made yet |

Each answers the result per name or per key, so a partial change says what was not changed.

## Headless

In the `headless` profile there is no console: your product writes the same rows (bindings and grants) through the API, with a person token that holds `tenant_admin`. See [cnctl](../reference/cnctl.md) (`bindings`, `grants`) and [the headless engine](../enterprise/headless.md).
