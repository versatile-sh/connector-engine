![Postgres and tenants](../assets/section-postgres.svg)

# Postgres and tenants

The `postgres` store holds every row, the audit log and the event outbox, with row-level security per tenant. The profiles run `postgres:16-alpine`.

## The role

The engine refuses a role that could bypass row-level security. A superuser or a `BYPASSRLS` role stops boot with:

```text
store: invalid_argument: database role bypasses row-level security
```

Create a plain role that owns the database:

```sql
CREATE ROLE cn LOGIN PASSWORD '<a long random password>' NOSUPERUSER NOBYPASSRLS;
CREATE DATABASE connector_engine OWNER cn;
```

Managed providers often hand out a superuser (`postgres`) by default. Run the two statements as that user, then give the engine the `cn` URL.

## Connect

```yaml
# adapters.yaml
ports:
  store: {use: builtin:postgres, config: {databaseUrlRef: "env:CN_DATABASE_URL"}}
```

```sh
export CN_DATABASE_URL='postgres://cn:<password>@db.internal:5432/connector_engine?sslmode=require'
```

`databaseUrlRef` takes `env:NAME` or `file:/path`; the compose profiles use a file written by `init`.

## Migrations

The engine applies its embedded migrations at start, serialised by an advisory lock, so two instances starting together do not race. There is no separate migrate step.

## Tenants

Each tenant has its own key-encryption key; removing a tenant destroys the key, so its sealed data cannot be opened again.

```sh
cnctl admin tenants create acme
cnctl admin tenants remove acme
```

## Retention

The default and `headless` keep everything. Prune history with:

```yaml
retention:
  every: 1h
  deliveredEvents: 168h
  idempotencyKeys: 168h
  audit: 2160h
  calls: 2160h
```

## Backups

Back up the database with your usual tooling (`pg_dump`, provider snapshots) **and** the secrets directory (`secrets.dir`) together. Rows without the root key are sealed data nobody can read.
