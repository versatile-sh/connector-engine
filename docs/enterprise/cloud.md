![Deploy to the cloud](../assets/section-cloud.svg)

# Deploy to the cloud

One-click deploys of the **headless engine** for teams and products. The person console is not offered: it is a single-owner app that must stay on `127.0.0.1` ([security model](security-model.md#deploying-safely)).

| Platform | How | Status |
|---|---|---|
| Render | [![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/versatile-sh/connector-engine) | `render.yaml`; needs the first release |
| Railway | a template built from `railway.json` | the button comes once the template is published |
| Fly.io | `fly launch` with `fly.toml` | needs the first release |
| DigitalOcean App Platform | not offered | App Platform has no persistent volume, and the engine's root key must survive restarts |

None of these platforms has been tested end to end. The configuration they deploy (`deploy/cloud/`) has been booted against Postgres 16 with a `NOSUPERUSER NOBYPASSRLS` role.

## What every deploy needs

| Need | Why | Provided by |
|---|---|---|
| Postgres 16 with a non-superuser role | the engine refuses a role that bypasses row-level security ([Postgres](postgres.md)) | the platform's database add-on |
| a persistent volume at `/var/lib/connector-engine` | it holds the root key; losing it makes every sealed record unreadable | a disk or volume |
| `CN_DATABASE_URL` | the database URL | the platform, from the database |
| `CN_OPERATOR_TOKEN` | the operator listener's bearer token: a long random string | the platform's secret generator, or you |
| your OIDC issuer | no token verifies without it ([OIDC](oidc.md)) | an edit to `deploy/cloud/connector-engine.yaml` in your fork |
| the published image | `deploy/cloud/Dockerfile` starts from `ghcr.io/versatile-sh/connector-engine-headless:latest` | the first release |

`deploy/cloud/Dockerfile` copies `deploy/cloud/connector-engine.yaml` and `adapters.yaml` into the image and runs as root inside the container, because these platforms mount volumes owned by root and the distroless image has no shell to change that.

## Steps common to all platforms

1. Fork the repository.
2. In your fork, set your issuer in `deploy/cloud/connector-engine.yaml` (`auth.issuers`, and `auth.audience` and `auth.manageAudience` to the values your IdP puts in `aud`).
3. Deploy from your fork with one of the sections below.
4. Check it:

```sh
curl -s -o /dev/null -w '%{http_code}\n' https://<your app>/readyz
```

```text
204
```

The public listener (`:8080`) is the only one the platform exposes. The operator listener (`:8081`) is reachable only on the platform's private network; run `cnctl admin` from a service there, with `CNCTL_URL=http://<private host>:8081` and `CNCTL_TOKEN=$CN_OPERATOR_TOKEN`.

## Render

The button reads `render.yaml`: one web service (Docker, `starter` plan, one instance, a 1 GB disk, health check `/readyz`) and one Postgres database. `CN_OPERATOR_TOKEN` is generated, `CN_DATABASE_URL` comes from the database. Use the button from your fork: replace `versatile-sh` in the button URL with your account.

- The disk needs a paid plan, and a service with a disk deploys with a short stop instead of zero downtime.
- If boot logs `database role bypasses row-level security`, create a role as in [Postgres](postgres.md#the-role) and point `CN_DATABASE_URL` at it.

## Railway

`railway.json` builds `deploy/cloud/Dockerfile`, runs one replica and checks `/readyz`. In the project:

1. Add a PostgreSQL service. Railway's default user is a superuser: create the `cn` role ([Postgres](postgres.md#the-role)).
2. Add a volume mounted at `/var/lib/connector-engine`.
3. Set `CN_DATABASE_URL` (the `cn` URL on the private network), `CN_OPERATOR_TOKEN`, and `PORT=8080`.

## Fly.io

```sh
fly launch --no-deploy --copy-config
fly volumes create engine_data --size 1
fly secrets set CN_OPERATOR_TOKEN="$(openssl rand -hex 32)" CN_DATABASE_URL='postgres://cn:...@.../connector_engine'
fly deploy
```

`fly.toml` builds `deploy/cloud/Dockerfile`, mounts `engine_data` at `/var/lib/connector-engine`, keeps one machine running (`auto_stop_machines = "off"`) and checks `/readyz`. Change `app` and `primary_region` to yours. Bring Postgres from Fly's managed Postgres or another provider, with the `cn` role.

## After the first deploy

- Create a tenant: `cnctl admin tenants create <tenant>`.
- Give roles to your people ([OIDC](oidc.md#roles)).
- Add a `webhook` sink to receive [events](../reference/events.md).
- Back up the database **and** the volume together ([Postgres](postgres.md#backups)).
