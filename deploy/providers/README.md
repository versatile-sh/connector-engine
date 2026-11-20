# Provider templates

> **Community templates, not tested by maintainers.** Nobody on the project runs these platforms. They deploy the **headless engine** only. For one person, or a small team on a server you control, follow [Run it on a VPS](../../docs/self-hosted/vps.md) instead.

Every template builds [`deploy/cloud/Dockerfile`](../cloud/Dockerfile): the published `ghcr.io/versatile-sh/connector-engine-headless:latest` image plus [`deploy/cloud/connector-engine.yaml`](../cloud/connector-engine.yaml) and [`adapters.yaml`](../cloud/adapters.yaml). The image exists from the first release (v0.1.0).

| Provider | Files | Notes |
|---|---|---|
| Render | [`render/`](render/README.md), `render.yaml` at the repository root | Render reads its Blueprint from the root |
| Railway | [`railway/`](railway/README.md) | set the config-as-code path in the service settings |
| Fly.io | [`fly/`](fly/README.md) | pass the config and the Dockerfile on the command line |

## Requirements, for a person or an agent deploying

Do every step; the engine refuses to start, or loses data, without them.

1. **Fork, then set the issuer.** In `deploy/cloud/connector-engine.yaml`, replace `https://issuer.example.com` with your OIDC issuer and its JWKS URL, and set `auth.audience` and `auth.manageAudience` to the `aud` values your IdP issues.
2. **Postgres 16 with a plain role.** The engine refuses a superuser or a `BYPASSRLS` role (`store: invalid_argument: database role bypasses row-level security`). As the provider's admin user, run:
   ```sql
   CREATE ROLE cn LOGIN PASSWORD '<long random password>' NOSUPERUSER NOBYPASSRLS;
   CREATE DATABASE connector_engine OWNER cn;
   ```
3. **Environment variables.**

   | Variable | Value |
   |---|---|
   | `CN_DATABASE_URL` | `postgres://cn:<password>@<host>:5432/connector_engine` |
   | `CN_OPERATOR_TOKEN` | a long random string, for example `openssl rand -hex 32`; keep it secret |
   | `PORT` | `8080`, where the platform needs to know the listening port |

4. **A persistent volume at `/var/lib/connector-engine`.** It holds the root key. Without it, a restart makes every sealed record unreadable. The image runs as root so it can write to a volume the platform mounts as root.
5. **One instance.** Replicas are not supported ([scaling](../../docs/enterprise/scaling.md)).
6. **Expose port 8080 only.** The operator listener (8081) stays on the private network.
7. **Check readiness.** `GET /readyz` answers `204` once the database answers:
   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<your app>/readyz
   ```
   ```text
   204
   ```

The configuration in `deploy/cloud/` has been booted against Postgres 16 with such a role; the platforms themselves have not. Background: [Deploy to the cloud](../../docs/enterprise/cloud.md).
