# Render

> **Community template, not tested by maintainers.** Read the [requirements](../README.md#requirements-for-a-person-or-an-agent-deploying) first.

The Blueprint is [`render.yaml`](../../../render.yaml) at the **repository root**, because Render reads Blueprints from there. It defines:

- a Docker web service from `deploy/cloud/Dockerfile`, `starter` plan, one instance, health check `/readyz`;
- a 1 GB disk at `/var/lib/connector-engine` (a disk needs a paid plan, and deploys stop the old instance first);
- a Postgres 16 database; `CN_DATABASE_URL` comes from it and `CN_OPERATOR_TOKEN` is generated.

Steps: fork, set your issuer, then create a Blueprint from your fork in the Render dashboard. If boot logs `database role bypasses row-level security`, create the `cn` role and point `CN_DATABASE_URL` at it.
