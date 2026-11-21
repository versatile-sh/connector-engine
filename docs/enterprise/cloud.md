![Deploy to the cloud](../assets/section-cloud.svg)

# Deploy to the cloud

The supported ways to run connector-engine are on your own machine or on a server you control: see [install](../getting-started/install.md) and [run it on a VPS](../self-hosted/vps.md). This page covers the other option, a platform-as-a-service, for the **headless engine** only.

> **Community templates, not tested by maintainers.** Nobody on the project runs Render, Railway or Fly.io. The templates are a starting point you verify yourself.

The person console is not offered on a platform: it keeps local state and a keychain-wrapped key, and runs on your machine or a server behind a TLS proxy ([security model](security-model.md#deploying-safely)).

## Where the templates are

| Path | Holds |
|---|---|
| [`deploy/cloud/`](../../deploy/cloud) | the shared image: the published headless image plus a production config that reads `CN_DATABASE_URL` and `CN_OPERATOR_TOKEN` from the environment |
| [`deploy/providers/`](../../deploy/providers/README.md) | one folder per provider (Render, Railway, Fly.io), each with a short README, and the requirements every deploy must meet |
| `render.yaml` | Render's Blueprint, at the repository root because Render reads it there |

## What every deploy needs

The [provider README](../../deploy/providers/README.md#requirements-for-a-person-or-an-agent-deploying) is the checklist, written so a person or an AI agent can follow it:

- your OIDC issuer in `deploy/cloud/connector-engine.yaml` ([OIDC](oidc.md));
- Postgres 16 with a `NOSUPERUSER NOBYPASSRLS` role ([Postgres](postgres.md#the-role));
- `CN_DATABASE_URL` and `CN_OPERATOR_TOKEN`;
- a persistent volume at `/var/lib/connector-engine` for the root key;
- one instance ([scaling](scaling.md)), port 8080 exposed and 8081 kept private;
- `GET /readyz` answering `204`.

DigitalOcean App Platform has no template: it has no persistent volume, and the root key must survive restarts.

## What was verified

The configuration in `deploy/cloud/` passes `adapters check` and boots against Postgres 16 with such a role: `/readyz` answers `204` and the operator API creates a tenant. With a superuser role it refuses to start, as intended. The platforms were not exercised.
