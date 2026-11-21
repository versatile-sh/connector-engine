![Approvals and notifications](../assets/section-approvals.svg)

# Approvals and notifications

A function set to **ask** holds the call until a person decides. This page shows where you can answer and what each answer does.

## The Approvals page

The **Approvals** page keeps the calls that wait for you in its main column, each with the function, its connector, a preview of its arguments, when it was asked, when it runs out and the answers below; with none waiting it says so in one card. Under them it lists the last ten decisions (approved, denied, expired, who answered and when), and beside them the connectors and functions currently set to **Ask**, each linking to its connector, and where a held call reaches you: this console, the AI app, the computers paired for popups and the chat channels, with a link to **Notifications**. A Member sees only their own calls; with several users an Owner or Admin can switch to everyone's.

## What the app sees

1. The gateway holds the call for up to `approval.holdWindow` (20 s).
2. Answered in time: the call runs and the app gets its result in the same request.
3. Not answered: the app gets `pending_approval` with an id. It polls `approval_status`, or drops the call with `approval_cancel`.
4. An unanswered approval expires after `approval.timeout` (10 min).

An approved call runs once. A repeated identical call in the same session attaches to the open approval instead of opening a new one.

A call that reached the server is never sent again. If the engine cannot record its result, the app gets `outcome_unknown`. While that call is unresolved, an identical repeat allowed only by "Allow for this session" waits for a person again.

## The answers

| Answer | Effect |
|---|---|
| Allow once | this call runs |
| Allow for this session | this call and the same function for the rest of the session, labelled with how long ("Allow for up to 8 hours") |
| Always allow this app | the function is allowed for the calling key only: every other key keeps its former access as its own override, then the default rises to "on" ([details](permissions.md#always-allow-this-app)); offered to Owners and Admins only |
| Deny | the call is refused |

### Who decides

With more than one console user, who may answer follows the connector's [credential mode](connectors.md#credential-mode): on a `shared` connector the key's owner or any Owner or Admin, on a `per_person` connector the key's owner only, since the call uses their own sign-in. The engine enforces it and refuses anyone else with `not_an_approver`. Owners and Admins may cancel anyone's held call (`POST /api/approvals/{id}/cancel`); the console offers it as the `cancel` choice.

## Where you can answer

| Surface | Set up |
|---|---|
| the console's **Approvals** page | nothing to set up |
| a desktop popup | run `make notify` on the machine |
| Discord, Slack or Telegram, with buttons | **Notifications** in the console |
| a webhook (ntfy, Discord or Slack incoming webhooks), with an "Open in console" link | **Notifications** in the console |
| the AI app itself (MCP elicitation) | turned on per app in **Notifications**; off by default |

Nothing is sent until you add a surface.

```sh
make notify
```

`make notify` runs `vsh notify --console http://127.0.0.1:<CONSOLE_PORT>`: it shows a desktop notification for each call waiting for you and opens a small approval page.

- Chat surfaces accept clicks only from the owner id you configure, and keep call arguments out of third-party messages.
- Each console user has their own notifications: their channels, their paired computers and the elicitation setting of their own keys. Nobody else sees or changes them.
- A call waiting for approval goes only to the channels and computers of the key's owner.
- A chat channel answers for the console user who added it: a click from its configured owner id decides as that user. In a shared chat everyone there sees the prompts, and whoever holds that owner id answers for that user, so add the channel in a private chat with the bot. A click for a call that user may not decide is refused with "this call is not yours to answer".
- Elicitation lets the app ask you inside the chat. Turn it on only for apps you answer by hand: an app with hooks or scripts that answer prompts could approve its own calls. Destructive functions are never asked there, and Always allow this app is never offered there.

Design and threat model: [approval surfaces](../self-hosted/approval-surfaces.md).

## Headless

Your product decides approvals through the API and the `connector.approval.*` events, with a person token that holds an approving role (`approver` or `tenant_admin`):

```sh
export VSH_ADMIN_URL=http://127.0.0.1:8080 VSH_ADMIN_TOKEN=<person token>
vsh admin approvals list
vsh admin approvals decide <id> --decision approve      # or approve_for_session, deny
```

With nothing waiting, the list prints:

```text
{
  "items": null
}
```

See [events](../reference/events.md) and [vsh](../reference/vsh.md).
