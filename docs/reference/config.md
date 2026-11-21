![Configuration keys](../assets/section-config.svg)

# Configuration

Two YAML files configure the engine. Unknown keys are errors; a missing key keeps its default.

| File | Flag | Environment | Holds |
|---|---|---|---|
| `connector-engine.yaml` | `--config` | `CN_CONFIG` | every setting; empty takes the defaults |
| `adapters.yaml` | `--adapters` | `CN_ADAPTERS_FILE` | the adapter for each port; empty takes the defaults and the `CN_<PORT>` variables |

**Source of truth:** [`config/connector-engine.yaml`](../../config/connector-engine.yaml) lists every key with its default. This page explains the keys you are most likely to change.

```sh
connector-engine --config connector-engine.yaml --adapters adapters.yaml adapters check
```

```text
adapters ok
```

Warnings print one per line before `adapters ok`, for example:

```text
warning port secrets: memory keeps the root key in plaintext; acknowledge plaintext_root_key to run with it
```

## References

Secrets are never written in the files. A key ending in `Ref` takes a reference:

| Form | Reads |
|---|---|
| `env:NAME` | the environment variable `NAME`; empty is an error |
| `file:/path` | the file's content, trimmed |

## Keys most often changed

| Key | Default | Meaning |
|---|---|---|
| `mode` | `production` | `dev` relaxes production checks; never deploy it |
| `server.listen` | `:8080` | public HTTP: `/mcp`, `/v1`, `/mcp/manage`, probes |
| `server.grpcListen` | `:9090` | gRPC; `""` turns it off |
| `server.operatorListen` | `""` (off) | the operator API |
| `server.publicURL` | `""` | the address clients use; the default for both audiences |
| `server.snapshot` | `false` | `GET`/`PUT /v1/snapshot` on the operator listener, for backups; returns plaintext keys; `PUT` answers the restored rows per kind and its warnings |
| `auth.issuers` | `[]` | trusted OIDC issuers ([OIDC](../enterprise/oidc.md)) |
| `auth.audience`, `auth.manageAudience` | `server.publicURL` | `aud` of agent and person tokens |
| `auth.agentTokenMaxLifetime` | `1h` | the longest agent token accepted |
| `auth.operator.tokenRef` | `""` | the operator bearer token |
| `bootstrap.roles` | `[]` | roles granted at boot |
| `secrets.dir` | `/var/lib/connector-engine/secrets` | where the root key and key ring rest |
| `permission.defaultMode` | `deny` | a function with no grant |
| `approval.holdWindow` | `20s` | how long a held call waits before `pending_approval` |
| `approval.timeout` | `10m` | when an unanswered approval expires |
| `pinning.relistMin`, `pinning.relistMax` | `30s`, `5m` | the re-list schedule that catches changed tools |
| `gateway.skills` | `off` | `tools` or `tools+prompts` to serve skills |
| `gateway.relayInstructions` | `never` | `pinned` relays server instructions after review |
| `gateway.upstreamContent` | `refuse` | `granted` offers upstream prompts and resources |
| `runtime.allowPrivateHosts` | `false` | let connectors reach private addresses |
| `runtime.loopbackHost` | `""` | host dialled in place of `localhost` |
| `retention.*` | `0s` (keep) | prune delivered events, idempotency keys, audit and calls every `retention.every` |

## Behaviours worth knowing

| Setting | Behaviour |
|---|---|
| egress guard | upstream connections (MCP servers, OAuth issuers, sources) refuse non-public addresses unless `runtime.allowPrivateHosts` is set. They ignore `HTTP_PROXY` and `HTTPS_PROXY`, because through a proxy the guard would check the proxy's address |
| loopback host | when `runtime.loopbackHost` is set, it is dialled in place of a `localhost`, `127.0.0.0/8` or `::1` host while the URL and its `Host` header stay as typed |
| secrets selector | `ports.secrets` in `adapters.yaml` is the only selector of the secrets adapter. The older `secrets.adapter` key still loads with a deprecation warning, and boot refuses it when it names another adapter |
| acknowledgements | production refuses risky settings unless `adapters.yaml` lists them in `acknowledge`: `local_token_issuer`, `long_lived_agent_tokens`, `trusted_proxy`, `manage_mcp_mutations`, `plaintext_root_key`, `private_egress` |

## The profiles

| Profile | Files |
|---|---|
| `headless` | [`deploy/profiles/headless/`](../../deploy/profiles/headless) |
| `person` | [`selfhosted/deploy/profiles/person/`](../../selfhosted/deploy/profiles/person) ([what it changes](../self-hosted/person.md#profile-settings)) |
| cloud | [`deploy/cloud/`](../../deploy/cloud) ([community templates](../enterprise/cloud.md)) |
| dev | [`examples/dev/`](../../examples/dev) |
