![Every call, on record](../assets/section-activity.svg)

# Activity

The **Activity** page lists every call an AI app made through the console's `/mcp`, per hour and per connector. Open a call to see its arguments and a result excerpt.

## What is recorded

| Item | Default |
|---|---|
| arguments | recorded |
| result excerpt (first 4 KB of each answer) | off; turn on **Record result excerpts** under **What is kept** on the Activity page |
| likely secrets (token, password, secret, authorization, API key, bearer and key-shaped values) | replaced before anything is written |

- Records go to `activity/YYYY-MM-DD.jsonl` beside the console's state file, mode 0600, kept 30 days.
- The engine itself keeps only a hash of the arguments, plus its audit rows and call records (90 days in the `person` profile, `retention.audit` and `retention.calls`).
- **Delete all activity** removes the files and hides older engine call records from the page. Only an Owner can delete activity, and only Owners and Admins change what is kept.
- With several users, each sees the calls of their own keys; Owners and Admins can also see everyone's ([roles and ownership](../self-hosted/person.md#roles-and-ownership)).

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
