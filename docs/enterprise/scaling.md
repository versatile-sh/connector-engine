![Scaling and replicas](../assets/section-scaling.svg)

# Scaling and replicas

Run **one engine instance per database**. That is the shape every test and every profile uses. Scale it up (CPU, memory, Postgres) before you scale it out.

## What one instance runs

| Part | Work |
|---|---|
| listeners | `/mcp`, `/v1`, gRPC, the operator API |
| the worker | approval timers and sweeps, re-lists of connectors, retention pruning |
| the relay | delivers the event outbox to the sinks, in order, at least once |

## More than one replica

Not verified. Read this before you try it:

- **Migrations** are safe: they are serialised by a Postgres advisory lock.
- **The relay** reads undelivered outbox rows without claiming them. Two replicas can deliver the same event twice. Delivery is at least once already, so consumers must deduplicate by the CloudEvent `id`; with replicas, duplicates become routine.
- **The worker** has no leader election. Every replica runs the same timers and sweeps; each state change is a guarded transaction, but this path has no multi-replica test.
- **The secrets directory** (`secrets.dir`) must be the same on every replica: each one that starts on an empty directory writes its own root key, and the others cannot open its records. Every secrets adapter keeps the root key in that directory ([keychain](../self-hosted/keychain.md)).

Need more throughput today? Give the instance and Postgres more CPU and memory, and keep one engine.

## Health checks

| Probe | Answers |
|---|---|
| `GET /livez` | 204 while the process serves |
| `GET /readyz` | 204 when the store answers, 503 otherwise |

Use `/readyz` for load balancers and platform health checks.

## Rolling updates

A volume-backed instance (Render disk, Fly volume, Railway volume) cannot run two copies during a deploy, so the platform stops the old one first. Expect a short gap; AI apps retry. Use a maintenance window for upgrades with migrations.
