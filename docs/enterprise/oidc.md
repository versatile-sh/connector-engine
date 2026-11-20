![Bring your own identity](../assets/section-oidc.svg)

# OIDC and tokens

In the `headless` profile, every request carries a JWT from your own identity provider. The engine verifies it against the issuer's JWKS; it never sees passwords.

## Configure the issuer

```yaml
# connector-engine.yaml
auth:
  audience: urn:mycompany:connector-engine:mcp
  manageAudience: urn:mycompany:connector-engine
  issuers:
    - {name: "https://login.mycompany.com", url: "https://login.mycompany.com/.well-known/jwks.json"}
  operator: {tokenRef: "env:CN_OPERATOR_TOKEN"}
```

```yaml
# adapters.yaml
ports:
  auth: {use: builtin:jwks}
```

| Key | Meaning |
|---|---|
| `auth.issuers[].name` | must equal the token's `iss` |
| `auth.issuers[].url` | the issuer's JWKS URL |
| `auth.audience` | the `aud` of agent tokens, for `/mcp`; defaults to `server.publicURL` |
| `auth.manageAudience` | the `aud` of person tokens, for `/v1` and `/mcp/manage`; defaults to `server.publicURL` |
| `auth.agentTokenMaxLifetime` | the longest agent token accepted (`exp - iat`), default `1h` |
| `auth.operator.tokenRef` | the operator listener's bearer token, `env:NAME` or `file:/path` |

## Claims

| Claim | Person token | Agent token |
|---|---|---|
| `iss`, `aud`, `iat`, `exp` | required | required |
| `sub` | the person id | the agent's subject |
| `tenant` | required | required |
| `workspace` | optional | required |
| `agent` | absent | required: its presence makes the token an agent token |
| `session` | optional | required |
| `agent_owner`, `on_behalf_of` | optional | optional: the person the agent acts for |

A token without `tenant`, `iat` or `exp`, with an unknown `iss`, or an agent token living longer than `auth.agentTokenMaxLifetime`, is refused with 401.

## Roles

A person token says who; roles say what they may do. Give roles at boot, or through the API with a `tenant_admin` token:

```yaml
bootstrap:
  roles:
    - {tenant: acme, person: alice, role: tenant_admin, scope: t/acme}
```

```sh
vsh admin roles put --tenant acme --person bob --role approver --scope t/acme
```

```text
{
  "tenant": "acme",
  "person": "bob",
  "role": "approver",
  "scope": "t/acme"
}
```

## Try it without an IdP

In dev mode, the local issuer mints tokens at `POST /local/token`, authorised by the operator token:

```sh
curl -s -X POST -H "Authorization: Bearer $CN_OPERATOR_TOKEN" http://127.0.0.1:8080/local/token \
  -d '{"sub":"x","tenant":"t1","workspace":"w1","agent":"x","session":"s1"}'
```

```text
{"token":"eyJhbGciOiJFZERTQSIs..."}
```

Production refuses the local issuer unless `adapters.yaml` acknowledges `local_token_issuer`; the `person` profile does, because its console is the issuer.
