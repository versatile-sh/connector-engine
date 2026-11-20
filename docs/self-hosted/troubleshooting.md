![When something fails](../assets/section-troubleshooting.svg)

# Troubleshooting

Find the message you see, then run the fix. Start with the logs when nothing matches:

```sh
make logs
```

## The stack

| You see | Cause | Fix |
|---|---|---|
| an AI app gets `the gateway is locked: run make unlock on this computer, then retry` | the engine restarted (reboot, crash, `docker restart`) and waits for its unlock key | `make unlock` |
| `the gateway is not answering; after a restart it stays locked until you run make unlock on this computer, then retry` | the engine is down or still locked while the console is unlocked | `make unlock`; if it persists, `make logs` |
| the engine log says `waiting for the host to deliver the unlock key` | same as above | `make unlock` |
| an approval link answers 503 | the console is locked | `make unlock`, then open the link again |
| `make up` fails with a bind error on port 7465 | another program uses the port | `make up CONSOLE_PORT=9123` and keep it in `selfhosted/deploy/.env` |
| `make up` fails on `include` | Docker Compose older than 2.20 | update Docker Compose |
| `no Go on this host: the keychain is skipped` | Go is not installed, so `bin/vsh` cannot be built | install Go, or accept a plaintext root key (the console warns) |
| `keychain-migrate needs an OS keychain and bin/vsh` | no Secret Service on Linux (`secret-tool` missing), or no Go | install `libsecret-tools` (or `libsecret`) and Go, then retry |

## The console

| You see | Fix |
|---|---|
| forgot the password | `make reset-password` on the machine that runs the console |
| `too many wrong passwords, try again in a minute` | wait a minute: 5 tries a minute |
| the console warns about a plaintext root key | `make keychain-migrate` ([keychain](keychain.md)) |
| an AI app cannot sign in | check its redirect: loopback `http` or an app scheme (`cursor:`, `vscode:`); see [connect an AI app](../getting-started/connect-an-app.md#endpoints-the-app-uses) |

## Connectors

| You see | Fix |
|---|---|
| a function disappeared from the app | it changed upstream and is quarantined: choose on or ask on the connector's page to accept the new definition |
| a server on `localhost` is unreachable | it must listen on an address Docker can reach; the engine dials `host.docker.internal` in place of `localhost` |
| a call answers like an unknown tool | the function is off for this key, or the key cannot see the connector ([permissions](../using/permissions.md)) |

## The engine (headless)

| You see | Fix |
|---|---|
| `store: invalid_argument: database role bypasses row-level security` | connect with a role that is `NOSUPERUSER NOBYPASSRLS` ([Postgres](../enterprise/postgres.md)) |
| `auth.audience and auth.manageAudience default to server.publicURL, which is empty` | set `server.publicURL`, or both audiences ([OIDC](../enterprise/oidc.md)) |
| `reference "env:NAME": the variable is empty` | export the variable a `tokenRef` or `databaseUrlRef` names |

Check both files the way boot does before you deploy:

```sh
connector-engine --config connector-engine.yaml --adapters adapters.yaml adapters check
```

```text
adapters ok
```

## Still stuck

Open an issue with the output of `make ps` and the relevant lines of `make logs`. Report security problems privately ([SECURITY.md](../../SECURITY.md)).
