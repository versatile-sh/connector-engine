# Status against the specification

Specification: `adaptable-research-engine/studies/connector-engine/03-engine.md` at `d3adb54`. "Run" means a test of `e2e/` executes it against the built `connector-engine` and `cnctl` binaries, on a throwaway `postgres:16-alpine` and live upstreams on loopback (`scripts/e2e.sh`, 20 cases, about 18 s with `-race`). "Built" means code exists and is covered by unit or integration tests with fakes, fixtures or in-process servers only. "Not built" means nothing exists, or only a configuration key.

The e2e suite drives the public surfaces only: `cnctl admin` over HTTP as the operator (operator listener) and as tenant admins and approvers (person tokens), and the agent through the go-sdk MCP client and `cnctl tools` on `/mcp`. Upstreams: an MCP server on the go-sdk (read, write, destructive, unclassified and a customer tool with an output schema; a second one whose tools change between runs), a REST API imported from an OpenAPI file, a gRPC server with reflection, and listing files.

## Phases (§18) and the owner cases

| Case or phase | Level | Test in `e2e/` |
|---|---|---|
| i. Only F for agent X | Run | `owner_i_only_F_for_agent_X`: list is `[create_issue]`, a denied function answers like an unknown one, upstream count unchanged |
| ii. Enabled, ask each time | Run | `owner_ii_all_enabled_ask_each_time`: `pending_approval`, decided by the requester, answered after the worker runs it, upstream called exactly once, the next call asks again |
| iii. Read-only for agent Y | Run | `owner_iii_read_only_for_agent_Y` |
| iv. Ask once per session | Run | `owner_iv_ask_once_per_session`: second call of the session forwarded; another agent and another session ask again |
| v. Workspace cannot widen the ceiling | Run | `owner_v_workspace_ceiling_cannot_be_widened`, with `explainAccess` giving `over_catalog_ceiling` |
| vi. Description change, quarantine, accept, stale grant, `confirmGrant` | Run | `owner_vi_description_change_quarantine_accept_confirm` |
| vii. New function in an update quarantined | Run | `owner_vii_new_function_in_update_is_quarantined` (listing version 2 from a synced file) |
| viii. Unclassified function | Run | `owner_viii_unclassified_function`: held until classified; cancel by the agent |
| Enable all | Run | `owner_enable_all`: read and write run, destructive and unclassified are held (§7.3) |
| Tenancy: listing hidden, two tenants same names, private listing | Run | `tenancy_listing_hidden_for_a_tenant`, `tenancy_two_tenants_same_names_isolated`, `tenancy_private_listing` |
| Phase 4 ideas of 2026-11-18 | Run | `idea_files_listing_import_lands_unreviewed`, `idea_grpc_kind_call_through_reflection_import`, `idea_openapi_import_lands_unreviewed_and_calls_rest`, `idea_overlay_accept_and_restore_change_the_served_description`, `idea_projection_trims_and_result_filter_masks_cpf`, `cnctl_tools_list_search_call_write_the_output_file` |
| Events and restart | Run | `events_webhook_receives_signed_cloudevents` (Standard Webhooks signature checked with plugkit `outbox.Verify`), `restart_state_and_pending_approval_survive` (needs Postgres) |

## What is built, by section

| § | Item | Level | Notes |
|---|---|---|---|
| 3 | Ports | Built | the 16 interfaces in `port/`; conformance suite on `store/memory` and `store/postgres` |
| 3 | `store/postgres` | Run | every e2e case; boot refuses a superuser or BYPASSRLS role (`store/postgres/postgres_test.go`); cross-tenant reads blocked by row-level security (same file); the restart case reopens the same database |
| 3 | `store/memory` | Run | `CN_PG=0 scripts/e2e.sh` runs every case but the restart on it |
| 3 | `secrets/local` | Run | the root key persists across the restart case; `Destroy` (crypto-shredding) only unit-tested (`service/admin_test.go`) |
| 3 | `runtime/remote` | Run | `idea_*`, `owner_*`; session era against the go-sdk server. The stateless era and SSE variants are only in `runtime/remote/remote_test.go` |
| 3 | `runtime/declarative` | Run | `idea_openapi_import_lands_unreviewed_and_calls_rest` (GET, POST, DELETE with a path parameter) |
| 3 | `runtime/grpc` | Run | `idea_grpc_kind_call_through_reflection_import`, plaintext upstream with `grpc.insecure`; descriptor sets and reflection v1alpha are only in `runtime/grpc/grpc_test.go` |
| 3 | `runtime/memory`, `source/memory` | Built | fakes for the unit suites |
| 5.2 | `source/files` | Run | `idea_files_listing_import_lands_unreviewed` (a second sync imports nothing) |
| 5.2 | `source/openapi`, `source/reflection` | Run | the two idea cases above; URL-addressed OpenAPI documents are not run |
| 5.2 | `source/registry` (official v0.1), `source/docker`, `source/bsr` | Built | fixtures only; no call to the real registry, Docker catalog or BSR |
| 4 | Roles, delegated actions | Run | tenant admin, approver (the requester) and the operator through the surfaces; `authz/table` tests for the whole matrix |
| 4 | Tenant removal and the worker fan-out | Built | `service/admin_test.go`, `service/worker`; not run end to end |
| 5.1-5.4 | Install, update, function states | Run | owner cases vi and vii; installs of unreviewed listings land quarantined in every `idea_*` case |
| 5.5 | Catalog visibility, ceiling, offers, private listings | Run | owner case v and the tenancy cases; the V1 to V8 sweep is in `core/evaluate/catalog_test.go` |
| 5.2 | Reviewed listings (`reviewListing`, trust levels) | Built | `core/listing`, `service`; the e2e cases classify with the admin class instead of a review |
| 6 | Grant table, levels, row rules G1 to G10 | Run | owner cases; level `write` and agent level `read` in iii; the rule matrix is in `core/grant` |
| 7 | Evaluation, truth table of §7.2 | Built | `TestTruthTable` in `core/evaluate/evaluate_test.go`; the owner cases above are the end-to-end subset |
| 8 | Approval record, decide, hold window, status, cancel | Run | owner cases ii, iv, viii, the restart case and `cnctl_tools_*` |
| 8.2 | A14 `delivered_at` set only after the gateway wrote the answer | Built | `gateway/deliver.go`; `TestAskEachTimeHoldsThenAnswers` (both eras), `TestAnUndeliveredAnswerIsRepeatedNotForwardedAgain` |
| 8.2 | A8 on a bounded pool, failed items back off, a failed re-list leaves the record approved until stale (A9) | Built | `service/worker`; `TestAFailingItemBacksOffAndDoesNotStarveTimers`, `TestAFailedRelistLeavesTheApprovedRecordForTheNextSweep` |
| 8.3 | Session row ends at the agent token's `exp` (G5) | Built | `TestTheSessionRowEndsWithTheAgentToken` |
| 8.5 | A removed person neither decides nor lists an approval | Built | `TestARemovedApproverNeitherDecidesNorSeesTheApproval`; approvers still share one namespace for people and groups |
| 8.3 | Expiry, the denial brake, repeat rules R1 to R4, `outcome_unknown` | Built | `service/lifecycle_test.go`, `core/approval`; no e2e case waits for a timer. A cancelled request or a failure after the send is `outcome_unknown` (`internal/outcome`) |
| 9 | Pinning, quarantine, `confirmGrant` | Run | owner cases vi and vii |
| 9 | Re-list on a timer, instructions drift | Built | `service`, `service/worker`; the e2e cases force the re-list with `updateConnector`. One re-list per connector at a time, a lost revision race runs again (`TestARelistThatLostARaceRunsAgain`); gRPC reloads its descriptors on every re-list (`TestListFunctionsSeesUpstreamDrift`); a live `tools/list` response that was not captured is an error |
| 9 | Connector handles | Built | started per key outside the service lock, restarted after `unavailable`, stopped after 15 minutes idle (`service/handles_test.go`) |
| 9.1 | Description overlays | Run | `idea_overlay_accept_and_restore_change_the_served_description` (propose, accept, restore, clear); `suggestOverlay` has no built-in model adapter |
| 9.1 | Keep an overlay at acceptance, restore against the current upstream hash | Built | `acceptFunction` with `keep_overlay`; `TestAnUpstreamChangeKeepsOrSupersedesTheOverlayAtAcceptance` |
| 10 | Gateway: list, call, names, denial as unknown | Run | every case; both MCP eras are only tested in `gateway/gateway_test.go` |
| 10 | `find` and `call` | Run | `find` through `cnctl tools search`; the collapse above the thresholds is only unit-tested (`gateway`) |
| 10.5 | Response projection | Run | `idea_projection_trims_and_result_filter_masks_cpf` and the CLI case, including `invalid_path` |
| 10 | Result filter, built-in regex and secrets | Run | the same case masks a CPF with `resultFilter.adapter: builtin`; the filter being off is not run |
| 11 | Credential broker, API keys, OAuth | Built | `credential/` tests; no e2e case injects a credential or runs a connect flow |
| 12 | Events: outbox, CloudEvents, Standard Webhooks | Run | install, grant and call events reach a verifying receiver; the other published types are only unit-tested (`catalog/`, `service/admin_test.go`) |
| 13.1 | HTTP operations and `cnctl admin` | Run | 30 of the 81 operations are called in the cases |
| 13.1 | gRPC operations, the OpenAPI document | Built | `server/parity_test.go` proves each operation has every form; the e2e config does not open the gRPC listener |
| 13.2 | Management MCP `/mcp/manage` | Built | `server/mcpapi`: read-only by default, `decideApproval` never; not called in e2e |
| 14 | Configuration | Run | the e2e files override the keys of approvals, sources, runtime and filter; every key and default is in `config/connector-engine.yaml`, covered by `config/config_test.go` |
| 14 | Inert keys: `gateway.relayInstructions`, `gateway.denyAs`, `audit.arguments`, `audit.hashChain`, `pinning.hashIcons`, `approval.inChat.classes`, `listing.requireSignature`, `secrets.dataKeyCache`, `credentials.registrationOrder`, `runtime.container.*`, `grpc.unaryOnly` | Not built | validated, never read; any value but the default is refused at boot (`TestKeysThisBuildDoesNotActOnAcceptOnlyTheirDefault`) |
| 15 | Controls for the threat table | Built | token never forwarded or returned, found in decoded JSON strings and scrubbed from error details (`internal/tokenscan`, `gateway`), denial looks unknown (Run, case i) |
| 15.2 | The exposure check (`exposure.onTrifecta`) | Built | `core/exposure`, `service/exposure.go`: warn and `force_ask`; a re-list refreshes it in the background; event 13 when it turns true or false (`service/exposure_test.go`) |
| 3 | `ApprovalChannel` other than events, client registration by CIMD and DCR, shared reviewed catalog, ACI.dev converter, Presidio, container runtime, Cedar, OpenBao | Not built | out of scope in `docs/PLAN.md` §1 |

## Measurements (§17)

M14 to M17 are out of scope for this build. M18 is done. No latency or load figure was measured by the e2e suite.

A real agent program, partly M14 and M15: `CN_CLAUDE=1 go test -run TestClaudeCode ./e2e` drives Claude Code 2.1.327 (`claude -p`, model `haiku`, `--strict-mcp-config`) against `/mcp` with a per-run agent token. Run 5 times on 2026-11-18, the last 3 with assertions on the tool calls Claude made (`stream-json`): all green. It lists only the granted functions; a read is forwarded once; a write in mode `ask` answers `pending_approval`, Claude polls `approval_status` while a person decides 5 s later, and the upstream runs exactly once; an ungranted function is not available and never reaches the upstream. The first two runs failed only on assertions over Claude's free text, never on engine behaviour. Not covered: Codex and other agent programs, the stateless MCP era from a real client, latency.

## Findings of the e2e wave

| Finding | Fix |
|---|---|
| The gRPC runtime could not dial a plaintext upstream: `app` never set `Insecure` | key `grpc.insecure` (default false), read by `app/build.go` |
| `createGrant` for an agent row accepted a connector that does not exist, or belongs to another tenant, and stored an orphan row | `service/bindings.go` refuses unless the connector lookup was a permission failure; `service/grantconnector_test.go` |

## Known flake

`runtime/grpc` failed once in a full `go test -race ./...` run on 2026-11-18 (and `internal/grpcdesc` once with "reflection: EOF" while another `go test` process ran). It did not reproduce in 5 isolated runs, 3 loaded runs, 2 more full runs and `-count=30` of both packages. Suspected: a reflection stream closed by a test server shutting down under load. Not fixed.

## Local console and packaging

| Item | Level | Notes |
|---|---|---|
| `connector-console` (owner account, sessions, Host/Origin/CSRF guards, import by MCP URL or listing link, per-function choice, agent tokens, approvals, `/mcp` proxy) | Run | `console/console_test.go` drives it in process against the engine on the memory store; `scripts/console-check.sh` drives the `person` compose profile on Postgres with `cnctl tools` through `/mcp` |
| Credentials from the console (header secret, OAuth with a pre-registered client) | Built | calls `createCredential`, `putClientRegistration`, `beginConnect`, `completeConnect`; no run against a real provider |
| `headless` profile | Built | `adapters check` only; it needs a real JWKS issuer to run |
