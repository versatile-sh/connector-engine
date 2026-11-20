# Railway

> **Community template, not tested by maintainers.** Read the [requirements](../README.md#requirements-for-a-person-or-an-agent-deploying) first.

[`railway.json`](railway.json) builds `deploy/cloud/Dockerfile`, runs one replica and checks `/readyz`.

1. Create a service from your fork. In its settings, set the config-as-code path to `/deploy/providers/railway/railway.json`.
2. Add a PostgreSQL service. Its default user is a superuser: create the `cn` role.
3. Add a volume mounted at `/var/lib/connector-engine`.
4. Set `CN_DATABASE_URL` (the `cn` URL on the private network), `CN_OPERATOR_TOKEN` and `PORT=8080`.
