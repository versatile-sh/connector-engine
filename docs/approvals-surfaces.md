# Approval surfaces for the local install

How a held call reaches the owner where they already are, and how the owner's answer comes back. This applies to the `person` profile only: the console and a host helper. The engine is not changed. The `headless` profile and products that embed the engine see nothing new.

## 1. The problem

A function set to "ask" holds the call (§8). Today the owner decides in the console's Approvals page at `127.0.0.1:<port>`. The agent gets `pending_approval` after the hold window (`approval.holdWindow`, 20 s) and polls `approval_status`. Opening the console for every call is tedious. The owner wants to answer in four places:

1. In the AI app itself, through MCP elicitation.
2. In a desktop notification that opens a small approval page.
3. In a chat app with real buttons: Discord, Slack and Telegram.
4. Through notify-only pushes (ntfy, Discord or Slack incoming webhooks) with an "Open in console" link.

The owner also wants fewer questions. Every surface offers three answers:

- "Allow once".
- "Allow for this session".
- "Always allow this function".

## 2. What the engine already gives, and what we use

| Engine contract | Used for |
|---|---|
| `GET /v1/approvals:watch?after=<seq>` (SSE, `id`/`event`/`data`, tenant approval events) | the feed of requested, decided, expired and cancelled approvals |
| `GET /v1/approvals?state=pending&agent=&session=` | seeding after a restart; finding the approval of an in-flight call |
| `GET /v1/approvals/{id}` (`ApprovalDetail`, with arguments opened for the approver) | the prompt text and the argument summary on the local page |
| `POST /v1/approvals/{id}:decide` with `approve`, `approve_for_session` or `deny`, and `channel` | every decision, recorded with the surface that made it |
| mode `ask_once` (§8 A4): `approve_for_session` writes a session row | "Allow for this session" |
| §8.6 `Lookup` on (session, call_hash): an identical call attaches to the open record, or is answered by the terminal undelivered one | getting the result of exactly the call that was decided |
| the `/mcp` endpoint and both MCP eras | elicitation, from the console's existing `/mcp` proxy |
| the signed `connector.approval.requested` webhook | not used: the watch reaches the console without a sink configured in the engine. A product that runs a webhook sink can drive the same `surface` package from it |

Everything new lives in three places:

- `surface/`: the relay and the channel adapters. It does not import `service`, `core`, `store` or `gateway`.
- `console/`: the decision use case, the settings, the `/approve` page API and the elicitation proxy.
- `cli/notify`: the `cnctl notify` host helper.

## 3. Options considered

| Option | Verdict |
|---|---|
| A. An `ApprovalChannel` inside the engine (§3, mapped as "form elicitation"), so the gateway elicits itself | Rejected for this work. It modifies the engine core, and the owner's premise forbids that |
| B. Lower `approval.holdWindow` in the person profile so `pending_approval` comes back at once, and elicit on it | Rejected. Clients without elicitation would lose the hold, so a quick answer from a popup or chat would no longer come back in the same call |
| C. The console's `/mcp` proxy becomes MCP-aware for `tools/call` only, finds the approval of the in-flight call, elicits on the client, decides through the API, then re-sends the identical call | **Chosen** |
| D. Chat bots in a separate `cmd/approval-relay` container | Rejected. The console already holds the owner's state, the person token and the decision rules. A second process would need copies of all three |
| E. Chat bots inside the console, and a host helper for desktop popups | **Chosen**. The console runs in Docker and cannot show a host notification |
| F. Chat through incoming webhooks plus a public callback URL | Rejected. It needs a public URL in front of the gateway |

## 4. The design

```
                        engine (unchanged)
   /mcp  /v1/approvals:watch  /v1/approvals/{id}  :decide  /v1/grants
     ^            |                  ^               ^
     |            v                  |               |
  +--+------------+------------------+---------------+------------+
  | console (person profile, docker, 127.0.0.1:<port>)            |
  |                                                               |
  |  elicit proxy --+                                             |
  |  /approve page -+--> Decide(id, choice, channel)  <---+       |
  |  approvals page-+        (console/decide.go)          |       |
  |                                                       |       |
  |  surface.Relay  <-- Feed (watch)                      |       |
  |    | Notify / Settle            Click ----------------+       |
  |    +-- discord (gateway ws)   slack (socket mode)             |
  |    +-- telegram (long poll)   webhook (ntfy, discord, slack)  |
  |    +-- device hub --SSE /api/notify/stream--+                 |
  +---------------------------------------------+-----------------+
                                                |  bearer device key
                                     host: cnctl notify  ->  notify-send / osascript / toast
                                                |  click
                                     browser: /approve/<id>?t=<single-use link token>
```

### 4.1 One decision use case

`Console.DecideChoice(ctx, id, choice, channel, note)` is the only place that turns an answer into engine calls. The console Approvals page, the `/approve` page, the relay and the elicitation proxy all call it.

| Choice | Engine calls |
|---|---|
| `once` | `:decide` `approve` |
| `session` | `:decide` `approve_for_session`. Offered only when the record's `mode` is `ask_once` |
| `always` | `SetAccess(connector, function, on)`, then `:decide` `approve`. The function's access flips to On in the console |
| `deny` | `:decide` `deny` |

`DecideApproval` answers a lost race with the current record and no error. The use case compares the returned state and decider with the requested ones and reports `already_decided` with the actual outcome. A surface never claims a decision it did not make.

From this change on, the console writes "Ask" as mode `ask_once` instead of `ask`. The two are the same to the approver except that `ask_once` also allows `approve_for_session`. When the exposure check forces `ask` (§15.2, step 13), the record carries `ask` and the session answer is not offered. Rows written earlier stay `ask` until the owner touches the function again.

A session is the `session` claim of the agent token. The console mints one per access key, and one per hourly OAuth access token. "Allow for this session" therefore lasts until the app's current token renews: at most one hour for an app signed in through the browser. The UI says so.

### 4.2 The relay and the universal channel interface (`surface/`)

```go
type Channel interface {
    Kind() string
    Notify(ctx context.Context, p Prompt) (Ref, error)
    Settle(ctx context.Context, ref Ref, p Prompt, o Outcome) error
    Listen(ctx context.Context, click func(context.Context, Click) (Outcome, error)) error
}
```

- `Notify` posts the prompt and returns the platform's reference to it, such as a message id or a channel and timestamp.
- `Settle` edits that message to the outcome: who decided, through which channel, and when. It also covers expired and cancelled.
- `Listen` runs the channel's inbound connection until `ctx` ends. A notify-only channel returns `nil` at once.
- On a click, the adapter does three things in order:
  1. It checks the platform user against the configured owner id.
  2. It acknowledges within the platform deadline.
  3. It calls `click`, then shows the returned outcome, or the error, to the clicker.
- Buttons carry `<choice>:<key>`, where `key` is a 12-character relay nonce, never the approval id. It fits Telegram's 64-byte `callback_data`, Discord's 100-character `custom_id` and Slack's `action_id`. A click on a key the relay no longer knows answers "expired, open the console".
- `Relay` consumes a `Feed`. For `requested` it asks the `Desk` for a `Prompt` and fans it out to every channel. For `settled` it calls `Settle` everywhere it notified. For a click it calls `Desk.Decide` with the channel kind and `<kind>:<platform user id>` as the note.

A new channel is one file implementing `Channel`, plus one entry in the console's channel factory.

| Kind | Inbound | Clicks | Limits |
|---|---|---|---|
| `discord` | Gateway websocket (outbound): HELLO, IDENTIFY with intents 0, heartbeat, reconnect with backoff; `INTERACTION_CREATE` | buttons; the interaction is acked with type 6 within 3 s, then the original message is edited | the bot must share a server with the owner to send a DM |
| `slack` | Socket Mode: `apps.connections.open` with the app-level token, then the websocket; each envelope is acked at once | Block Kit buttons; `chat.update` after the decision | the app needs `chat:write` and `im:write` |
| `telegram` | `getUpdates` long polling | inline keyboard; `answerCallbackQuery` with the outcome text, then `editMessageText` | the owner sends `/start` to the bot once |
| `ntfy` | none | none: a "View" action opens the console | the topic is readable by anyone who knows its name, so the message carries no arguments and no link token |
| `discord_webhook`, `slack_webhook` | none | none: an "Open in console" link | an incoming webhook has no way back to a loopback console |

Websockets use `golang.org/x/net/websocket`, which is already in the module graph. No new dependency is added.

Messages name the connector, the function, the app and the expiry. They never include arguments: those reach a third-party platform. The arguments appear only on the local `/approve` page and in the console.

### 4.3 The feed (`console/feed.go`)

- A dedicated streaming client reads `/v1/approvals:watch` with the person token. It does not use `Engine.do`, which has a 90 s timeout and reads the whole body.
- It reconnects with `after=<last sequence>` and mints the token again on a 401.
- The cursor is kept in the console state. On the very first start there is no cursor, and the relay does not replay history as notifications: a `requested` event is notified only when its approval is pending at that moment.
- After a restart, the pending list is announced once. The relay remembers notified ids, so nothing is announced twice.
- The relay runs only when at least one channel or paired device is configured.

### 4.4 Desktop popups (`cnctl notify`)

The console cannot reach the host's notification service. The host helper connects out to the console instead:

1. In Notifications, "Pair this computer" creates a device key. It is shown once, and the state keeps only its SHA-256.
2. `cnctl notify --console http://127.0.0.1:7465 --key-file ~/.config/versatile/notify.key` opens `GET /api/notify/stream` with the key as a bearer and reconnects on loss.
3. For each prompt the console mints a link token for that device and sends it in the stream. The helper shows a native notification:
   - Linux: `notify-send --action` through D-Bus;
   - macOS: `terminal-notifier -open`, or a `display dialog` through `osascript`;
   - Windows: a toast with protocol activation, through PowerShell.
4. A click opens `/approve/<id>?t=<token>` in the browser.
5. The page shows the connector, its icon, the function, the app, the expiry and an argument summary, with Allow once, Allow for this session, Always allow and Deny.

### 4.5 MCP elicitation in the `/mcp` proxy (`console/elicit.go`)

It is off by default. The owner turns it on per app (access key or OAuth app) in Notifications. When it is off for the calling app, `/mcp` is today's `ReverseProxy`, unchanged behind `gateMCP` and `refuseRevoked`.

For a `tools/call` from an app that is turned on and that declared the `elicitation` form capability:

1. The proxy forwards the call to the engine as usual, with the inbound request context.
2. While the engine holds the call, the proxy looks for the call's approval: a pending record of the token's agent and session, of the called function, created after the call started, and not claimed by another in-flight call. Exactly one match binds. With none or several, it waits for the engine's `pending_approval` answer, which names the id exactly.
3. It asks the client with the text of the engine's record for that id: "Allow <connector>: <function>?" with one field, Allow once, Allow for this session (only for `ask_once`) or Deny. Declining denies. Cancelling leaves the call pending for the other surfaces.
   - Protocol before 2026-07-28: the console captured the `elicitation` capability from `initialize`, keyed by `Mcp-Session-Id`. It answers the POST as an SSE stream, writes `elicitation/create` with a console-issued id on it, and takes the client's JSON-RPC response POST itself (202, never forwarded). If the record is decided elsewhere first, it sends `notifications/cancelled`.
   - Protocol 2026-07-28 and later: the console reads the capability from the request's `_meta` and answers `resultType: input_required` with the elicitation and a `requestState`. The `requestState` is an HMAC over the approval, agent, session and a digest of the call, and it expires with the approval. The client's retry carries `inputResponses`. The console checks the state, decides, and removes both fields before forwarding.
4. The result is the engine's own answer to this call. Either the held request returns it, or the proxy re-sends the identical call, which §8.6 `Lookup` attaches to the same record by call hash. A wrong match can therefore never return another call's result.
5. Destructive functions, by their annotations, are never elicited. They stay on the console, popup and chat surfaces.

**Verdict: elicitation lives outside the engine.** The proxy needs only the public `/mcp` contract, the approvals API and §8.6. The evidence is in `console/elicit_test.go`: go-sdk clients of both eras answer the elicitation, the approval is decided, and the upstream runs once. The cost is two pieces of coupling, both stated here. First, the proxy parses MCP framing for `tools/call`, `initialize` and JSON-RPC responses. Second, binding before the hold window ends uses a list query, with the exact id as the fallback. If a future engine elicits by itself (option A, behind `approval.inChat`), the proxy feature is turned off and nothing else changes.

### 4.6 The `/approve` page and link tokens

- `GET /api/approvals/{id}` returns the approval view (§5). It takes the console session, or `?t=<link token>`.
- `POST /api/approvals/{id}/decide` takes `{choice, token?}`. With a session it needs CSRF. With a token it needs no session. The Guard's Origin and Host checks apply either way.
- A link token is `base64url(approval|device|expiry|nonce).HMAC-SHA256`, keyed by a random key in the state file. It decides that one approval only. It is spent on first use, and it expires with the approval.
- A GET never decides.
- Links in ntfy and webhook messages never carry a token: they open the page, which asks for sign-in.

## 5. Console API added

| Route | Auth | Body / answer |
|---|---|---|
| `GET /api/approvals/{id}` | session or `?t=` | `ApprovalView` |
| `POST /api/approvals/{id}/decide` | session + CSRF, or `token` | `{choice: once\|session\|always\|deny, token?}` returns `{outcome: ApprovalView, already_decided: bool}` |
| `GET /api/notifications` | session | `{channels: [ChannelView], devices: [DeviceView], apps: [{name, elicit}]}`; never a secret |
| `POST /api/notifications/channels` | session + CSRF | `{kind, label, settings}` returns `ChannelView` |
| `DELETE /api/notifications/channels/{id}` | session + CSRF | removes the channel and its secret |
| `POST /api/notifications/channels/{id}/test` | session + CSRF | sends a test message, returns `{ok, error?}` |
| `PUT /api/notifications/apps/{name}` | session + CSRF | `{elicit: bool}` |
| `POST /api/notifications/devices` | session + CSRF | `{name}` returns `{device: DeviceView, key, command}`; the key is shown once |
| `DELETE /api/notifications/devices/{id}` | session + CSRF | revokes the device key at once |
| `GET /api/notify/stream` | bearer device key | SSE: `prompt` and `settled` events for the helper |

`ApprovalView`: `id`, `state`, `mode`, `connector {id, alias, title, icon}`, `function`, `function_title`, `kind` (the function's nature: read, write or destructive), `app`, `session`, `created_at`, `expires_at`, `arguments` (the JSON the engine opened, local page only), `may_have_run`, `reaches_outside`, `choices`, `decided_by`, `decided_at`, `channel`.

`ChannelView`: `id`, `kind`, `label`, `interactive`, `owner` (the configured owner id), `secret_hint` (the last 4 characters), `created_at`, `status` (`connected`, `connecting`, `error`, `notify_only`), `error`.

## 6. Threat model

Assets: the owner's decisions, the gateway, the bot tokens and webhook URLs, and the call arguments.

| Threat | Control |
|---|---|
| The gateway is exposed to reach a chat callback | Every chat inbound path is outbound from the console (Gateway websocket, Socket Mode, long polling). Nothing new listens. The console still publishes only `127.0.0.1:<port>` |
| Someone else in the Discord server or Slack workspace clicks Allow | Adapters accept clicks only from the configured owner user id, and answer others "not allowed". Messages go to the owner's DM |
| A forged or replayed popup link decides | The HMAC link token is bound to one approval and one device. It is single-use and expires with the approval. A GET never decides. It is never put in a third-party message |
| A malicious web page posts a decision (CSRF) or rebinds DNS | The existing Guard (Host allowlist, Origin on every mutation), plus CSRF for session callers |
| A stolen device key | It only receives prompts and their link tokens, and only for approvals made while it is connected. It is revocable at once, and the state keeps its hash only |
| **An agent approves its own call through elicitation** | Whoever holds an agent token can declare `elicitation` and answer "accept": a script, a hook, or a non-interactive client. So elicitation is off by default and enabled per app by the owner. It never offers "Always", and never covers destructive functions. Each decision is recorded with channel `mcp-elicitation` and the app in the note. The UI warns that an app's hooks can auto-answer |
| A wrong elicitation binding returns another call's result | The prompt comes from the engine's record for the decided id. The result comes from re-sending this call, matched by call hash (§8.6) |
| Tokens or webhook URLs leak | They are kept in the console state file, mode 0600, and never returned by the API (only the last 4 characters). They are never logged: adapter errors are scrubbed of the token and URL. Removing a channel deletes them |
| Arguments leak to a chat platform | Chat and push messages carry connector, function, app and expiry only |
| A pending approval lingers | Approvals expire (`approval.timeout`, 10 min). Expired and cancelled outcomes edit every message, so no stale Allow stays clickable. A late click answers "expired" |
| The relay floods after a restart | Persisted cursor, seeding from the pending list, and remembered notified ids |

## 7. Not covered

- Message references are kept in memory. After a console restart, the old chat messages are not edited when their approvals settle, and a click on them answers "expired, open the console".
- A held call is marked delivered (A14) when the console receives the engine's answer, not when the client does. A client that drops in that window loses the answer. Retrying the identical call returns it again while `resultRetention` lasts.
- MCP URL-mode elicitation is not used, and Cursor's form-mode bugs are not worked around.
- Claude Code, VS Code, Cursor and Codex were not driven against a real console. The tests use the go-sdk client.
