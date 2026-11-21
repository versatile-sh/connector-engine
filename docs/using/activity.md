![Every call, on record](../assets/section-activity.svg)

# Activity

The **Activity** page lists every call an AI app made through the console's `/mcp`, per hour and per connector. Open a call to see its arguments and a result excerpt.

## What is recorded

| Item | Default |
|---|---|
| arguments | recorded |
| result excerpt (first 4 KB of each answer) | off; turn on **Record result excerpts** under **What is kept** on the Activity page |
| likely secrets (token, password, secret, authorization, API key, bearer and key-shaped values) | replaced before anything is written |

- Records go to `activity/YYYY-MM-DD.jsonl` beside the console's state file, mode 0600, kept as long as **Keep calls for** says (30 days unless you change it).
- The engine itself keeps only a hash of the arguments, plus its audit rows and call records (90 days in the `person` profile, `retention.audit` and `retention.calls`).
- **Delete all activity** removes the files and hides older engine call records from the page. Only an Owner can delete activity, and only Owners and Admins change what is kept.
- The page loads the newest 50 calls and **Load more calls** reads the next 50; the filters (range, connector, key, outcome, search) run on the console, so the counts, the chart and the connector list cover every matching call, not only the loaded ones. The list refreshes every 10 seconds while only the first page is loaded.
- With several users, each sees the calls of their own keys; Owners and Admins can also see everyone's ([roles and ownership](../self-hosted/person.md#roles-and-ownership)).

## How long calls are kept

**Keep calls for** under **What is kept** takes one of five choices:

| Choice | What happens |
|---|---|
| Not kept | nothing is recorded, and what was recorded is deleted |
| 7 days, 30 days (default), 90 days | older calls are deleted by day file, checked every hour, and hidden from the page the moment they pass the limit |
| Forever | nothing is deleted unless you delete it |

The setting only governs the console's own records and the engine rows shown beside them. The engine keeps its call and audit records by its profile (`retention.audit` and `retention.calls`, 90 days in the `person` profile); the headless defaults do not change. The console shows no engine row older than its own limit.

## Delete a period

**Delete a period** opens a window with either **Older than** (1, 7, 30 or 90 days) or **Between days** (two days you pick). It counts what the period holds before it deletes, says "N calls will be deleted" and deletes only on **Delete these calls**. Captured calls go from the files, and the engine rows of that period stop showing here. Only an Owner can do this; the console logs each deletion (period and number of captured calls) in its own log.

```sh
vsh activity prune --older-than 30d
```

`vsh` counts first, asks `Delete 12 calls older than 30 days? [y/N]`, and `--yes` skips the question. It takes `30d` or a duration such as `720h`.

## Headless

The engine publishes every call as a [CloudEvent](../reference/events.md): `connector.call.started`, `finished`, `failed`, `denied` and `outcome_unknown`. Send them to your own store with the `webhook` sink. A person token with audit rights (`tenant_admin`) lists recent calls:

```sh
VSH_ADMIN_URL=http://127.0.0.1:8080 VSH_ADMIN_TOKEN=<person token> vsh admin calls list
```

```text
{
  "items": null
}
```

Without the right role the answer names what is missing, for example `403 permission_denied: ana may not audit.read on t/t1`.
