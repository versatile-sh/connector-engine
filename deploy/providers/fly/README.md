# Fly.io

> **Community template, not tested by maintainers.** Read the [requirements](../README.md#requirements-for-a-person-or-an-agent-deploying) first.

[`fly.toml`](fly.toml) keeps one machine running, mounts the volume `engine_data` at `/var/lib/connector-engine` and checks `/readyz`. Change `app` and `primary_region`. From the repository root:

```sh
fly launch --no-deploy --copy-config --config deploy/providers/fly/fly.toml
fly volumes create engine_data --size 1 --config deploy/providers/fly/fly.toml
fly secrets set CN_OPERATOR_TOKEN="$(openssl rand -hex 32)" CN_DATABASE_URL='postgres://cn:...@.../connector_engine' --config deploy/providers/fly/fly.toml
fly deploy --config deploy/providers/fly/fly.toml --dockerfile deploy/cloud/Dockerfile
```

Bring Postgres from Fly's managed Postgres or another provider, with the `cn` role.
