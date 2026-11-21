![Functions and permissions](../assets/section-permissions.svg)

# Functions and permissions

Every function of every connector is **off**, **on** or **ask**, for each access key. This page covers the choices, per-key profiles, and the rules the engine adds on top.

## The three choices

| Choice | The AI app |
|---|---|
| Off | does not see the function; a call answers like an unknown tool |
| On | calls it directly, unless a review or its server declares it destructive (see [function classes](#function-classes)) |
| Ask | the call waits until you decide; see [approvals](approvals.md) |

A denied function looks exactly like one that does not exist, so an app cannot probe what is there.

## Rules the engine adds

| Rule | Effect |
|---|---|
| destructive functions | "on" becomes "ask once" (or "ask"): you approve it once per session; the row says "asks once a session" |
| exposure check | a key that holds private data, reads untrusted content and has an outbound path is forced to "ask" |
| catalog ceiling | a listing's visibility caps what any key gets |

The full evaluation order is in [ARCHITECTURE.md](../../ARCHITECTURE.md#grants).

## Function classes

The engine treats each function as read, write or destructive.

| Where the class comes from | Turning it on or asking |
|---|---|
| a review of the listing at the function's hash | keeps the reviewed class |
| the server declares `destructiveHint: true` | keeps destructive: on asks once a session, and the row says "asks once a session" |
| neither | sets read when the server's `readOnlyHint` says so, else write |

- On the first start after this version, every class below a reviewed or declared class is raised once, including one set through the engine API; lower it again through the API if that was intended. Each raise drops the session answers of that function and is listed under **Changes** on the People page. A connector the console cannot read then is skipped with a warning in its log, and later starts raise nothing.

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
- The **Access** page lists, for each key, every connector with its mode and a link to its profile.

## Always allow this app

**Always allow this app** on a held call allows that function for the calling key only:

- every other key, and every key made later, keeps the function's former access as its own override;
- then the default rises to on, and the calling key's override is removed.

Setting the function's default again lifts the limit for keys made later. The pins are ordinary overrides: resetting a key to the default, copying a profile onto it, or turning it off and back on removes them, and the key then follows the raised default.

A failure shows on the approval page, which keeps reading the call for three minutes after an allow. The chat surfaces have already said "Always allowed" by then, and the failure is kept in memory only, until the console restarts.

## Permission sets

A [shareable setup](account-backup.md#shareable-setups) carries each key's profile as a **permission set** (a key slot), named after the key, such as `claude-code`. Nothing binds by name: a key here with the same name is a different key.

- Claim a set when an app connects (**Use permissions from** on the consent page), when making a key by hand on **Access**, or with **Apply to key…** under **Unclaimed permission sets** on Access and on each connector's page.
- Claiming gives the key exactly the set's profiles, within the connector's choices and its new-key limits, and consumes the set. **Discard** drops it. Removing a connector drops it from every set.

## Many at once

Tick functions (or select all the filter shows), then set them to off, on or ask in one request. The **More…** menu holds class presets (all reads on, all changes ask, all deletes off) and copies one key's profile onto others.

The console API behind these actions:

| Endpoint | Does |
|---|---|
| `PUT /api/connectors/{id}/functions` | `{"names": [...], "access": ...}` sets many functions for the default; also answers one entry per key it failed to give the new item rows |
| `PUT /api/connectors/{id}/profiles` | `{"mode": "off" \| "follow" \| "custom", "keys": [...]}`; every key when `keys` is empty |
| `PUT /api/connectors/{id}/profiles/{key}` | `{"names": [...], "access": ...}` narrows for one key, or `{"names": [...], "reset": true}` returns to the default |
| `POST /api/connectors/{id}/profiles:copy` | `{"from": key, "keys": [...]}` copies one key's mode and overrides |
| `GET /api/connectors/{id}/profiles/{key}` | the engine's answer for that key on each function and item |
| `PUT /api/connectors/{id}/new-keys` | `{"off": true}` makes new keys start off |
| `GET /api/slots` | the permission sets waiting to be given to a key, each with its mode and overrides on the connectors still installed |
| `POST /api/slots/{name}/claim` | `{"key": ...}` gives a key made already the set's profile on each of its connectors, then forgets the set |
| `DELETE /api/slots/{name}` | discards a permission set |
| `POST /api/agents`, `POST /api/oauth/requests/{id}` | also take `"slot": name`: the key made by hand, or by an app's consent, gets that set's permissions and the set is consumed |

Each answers the result per name or per key, so a partial change says what was not changed.

- Batches name at most 1000 functions, items or keys; a larger one is refused before anything is written.
- Writes to one connector run one at a time, including install, removal and revoking a key; a request that stops waiting gives up its turn.
- A key's profile is applied closed: the key is unbound, its overrides are written, and it is bound last. A failure halfway leaves it off rather than wider, and making the key again applies the whole profile.

## Headless

In the `headless` profile there is no console: your product writes the same rows (bindings and grants) through the API, with a person token that holds `tenant_admin`. See [vsh](../reference/vsh.md) (`bindings`, `grants`) and [the headless engine](../enterprise/headless.md).
