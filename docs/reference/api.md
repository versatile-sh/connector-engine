![Operator API](../assets/section-api.svg)

# API

Every management operation comes from one operation table (`server/table.go`), and three surfaces are generated from it, so they never drift:

| Surface | Where | Contract |
|---|---|---|
| HTTP | `/v1/...` on the public listener (person tokens) and the operator listener (operator token) | [`api/openapi.yaml`](../../api/openapi.yaml) (OpenAPI 3.1) |
| gRPC | `server.grpcListen` | [`api/connectorengine/v1/connectorengine.proto`](../../api/connectorengine/v1/connectorengine.proto) |
| management MCP | `/mcp/manage` | the same operations as MCP tools; read-only by default (`manage.mcp.operations`) |

The agent endpoint `/mcp` is separate: it serves the functions granted to one agent, not management operations.

`go generate ./server` rewrites the OpenAPI file and the proto from the table.

## Conventions

| Convention | Detail |
|---|---|
| authentication | `Authorization: Bearer <token>`: a person JWT on the public listener, the operator token on the operator listener |
| errors | `application/problem+json` with a stable error code |
| idempotency | mutations accept an `Idempotency-Key` header; a retry with the same key runs once |
| custom verbs | actions are `POST /v1/<resource>/{id}:<verb>`, such as `:decide`, `:accept`, `:sync` |
| long work | some mutations answer `202` with an operation; poll `GET /v1/operations/{id}` |

## Example

```sh
curl -s -H "Authorization: Bearer $CN_OPERATOR_TOKEN" http://127.0.0.1:8081/v1/approvals?state=pending
curl -s http://127.0.0.1:8080/v1/event-types | head -c 200
```

`/v1/event-types` is public: it lists the [event](events.md) types with their JSON Schemas.

## Groups of operations

| Area | Paths |
|---|---|
| catalog and listings | `/v1/listings`, `/v1/catalog/*`, `/v1/sources` |
| tenants and offers | `/v1/tenants/{tenant}`, `/v1/tenants/{tenant}/offers` |
| connectors and functions | `/v1/connectors`, `/v1/connectors/{id}/functions`, `/v1/connectors/{id}/instructions:accept` |
| bindings and grants | `/v1/connectors/{id}/bindings/{agent}`, `/v1/grants`, `/v1/access:explain` |
| approvals | `/v1/approvals`, `/v1/approvals/{id}:decide`, `/v1/approvals/{id}:cancel`, `/v1/pauses` |
| credentials | `/v1/credentials`, `/v1/connectors/{id}/credentials:connect`, `/v1/issuers/client` |
| calls and sessions | `/v1/calls`, `/v1/sessions/{id}:end`, `/v1/agents/{agent}/functions` |
| people and roles | `/v1/roles`, `/v1/groups/members`, `/v1/people/{id}`, `/v1/agents/{id}` |
| platform | `/v1/adapters`, `/v1/operations/{id}`, `/v1/event-types`, `/v1/secrets:rotate` |

From the command line, every operation is reachable through [`cnctl admin`](cnctl.md#admin).
