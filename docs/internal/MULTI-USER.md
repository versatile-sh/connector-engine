# Multi-user console

Status: design, approved in principle on 2026-11-20. Nothing here is built yet. Implementation starts from `main` after `feat/key-slots` merges, as the PRs in [Phased plan](#15-phased-plan).

The question: what if more than one person uses one self-hosted console? Today the console has one `Owner` account and acts on the engine as one person. This document makes it a console for several people: a household, a small team or a company that self-hosts. The engine core and the headless profile are not changed.

## 1. Goals and non-goals

Goals:

- The Owner invites people with single-use, expiring links. There is never an open "create account" screen after the first run.
- Three roles: **Owner** (everything), **Admin** (connectors, skills and policy), **Member** (uses the gateway with their own keys, approves their own calls and sees their own activity).
- Each person has their own access keys, approvals, activity, notification channels and provider sign-ins.
- Connectors are shared. Whether a connector uses one org-wide credential or each person's own sign-in is a per-connector setting.
- Removing a person revokes their keys at once.
- Every engine write is attributed to the person who made it.
- An install with one person looks and works exactly as today.

Non-goals:

- Impersonation ("view as Bob"). Admins see others' data, labelled. They never act as someone else.
- SSO, SCIM, groups of people, or more than one workspace. The engine supports groups and workspaces. The console keeps using one workspace.
- Self-service sign-up of any kind.
- Approval of one person's call by another person when the call uses that person's own credential (see [§7.2](#72-who-decides)).

## 2. What the engine already gives

The design rests on engine contracts that exist today. No engine change is needed.

| Engine fact | Where | Used for |
|---|---|---|
| The local issuer mints any claims for the operator: `sub`, `on_behalf_of`, `agent_owner` | `POST /local/token`, `auth/local` | a person token per console user; agent tokens that name the key's owner |
| Roles per person over a scope; `tenant_admin`, `workspace_admin`, `agent_configurer` | `authz/table/roles.go`, `actionRoles` in `authz/table/table.go`, `service/roles.go` | the engine half of each console role |
| `bootstrap.roles` re-puts each listed row at every boot | `app.bootstrap` | the console's service person |
| `Binding.CredentialPolicy`: `shared` (workspace credential) or `per_user` (the credential of `on_behalf_of`) | `port/enums.go`, `resolveCredential` in `service/credentials.go` | per-connector credential ownership |
| `AuthSpec.OwnerKindsAllowed` on a listing | `port/listing.go` | which credential modes a connector allows |
| OAuth connect for `owner: user` is bound to the person who began it | `BeginConnect`/`CompleteConnect` | each person signs in to their own Google |
| `Binding.AskRequester` routes a held call to its `on_behalf_of` person; `Binding.Approvers` adds persons or groups | `approval.Approvers` | approvals go to the key's owner, and on shared-credential connectors also to the managers group |
| Groups per person; an approver key matches the decider's person or any of their groups | `PutGroupMember`, `approverKeys` in `service/approvals.go` | one engine group `managers` holds every Owner and Admin |
| `DecideApproval` requires the decider to be a named approver and records `DecidedBy` | `service/approvals.go`, `core/approval` | own approvals only, attributed |
| `RemovePerson` tombstones a person; step 1 denies every call whose `on_behalf_of` or `agent_owner` is tombstoned; it revokes the person's credentials, roles and pending approvals | `service/bindings.go`, `tombstoned` in `authz/table/facts.go` | removal stops a person's keys at once, at the engine |
| Grant layers combine strictest-wins: the agent layer only narrows the workspace layer | `core/evaluate` steps 6 to 9 | a per-key profile can never widen policy |

## 3. Invariants

| # | Invariant | Protected by |
|---|---|---|
| U1 | At least one Owner exists once the first run is over. The last Owner can be neither removed nor demoted. | `State`, in the same atomic write as the change |
| U2 | No account is created except by the first run (zero users) or by accepting a live invite. | `State.AcceptInvite`, `State.CreateFirstOwner` |
| U3 | An invite or reset link works once, before it expires, and only while its creator still holds the role that allowed it. | `State.AcceptInvite`, `State.ResetWithLink` |
| U4 | Every session belongs to exactly one existing user. A request reads that user's role at request time. | `Sessions`, the `authed` middleware |
| U5 | Every access key has exactly one owner, set when the key is made and never changed. | `State.PutAgent` |
| U6 | An agent token's `on_behalf_of` and `agent_owner` are its key owner's person. | `Console.agentToken` |
| U7 | Every engine write runs as the person of the user who asked for it, or as the service person for identity plumbing only ([§6.3](#63-who-the-console-acts-as)). | `Engine.Call` refuses a context without an actor |
| U8 | A person id is opaque, stable across renames and never reused. | id generation |
| U9 | A Member's action can change only that Member's own keys, approvals, sign-ins, channels and password. | the capability check plus the ownership check ([§5](#5-roles-and-permissions)) |

## 4. Model

### 4.1 User

```go
type Role string // "owner" | "admin" | "member"

type User struct {
	Person     string    // engine person id, the key of State.Users
	Username   string    // 1 to 64 characters, unique case-insensitively, renamable
	Role       Role
	Salt, Hash string    // PBKDF2-SHA256, as today
	Iterations int
	CreatedAt  time.Time
	InvitedBy  string    // person id, empty for the first Owner
}
```

The person id is the user's identity everywhere: sessions, keys, captures, channels and engine rows. New users get `p-` plus 16 random hex characters. The first Owner of an install keeps the configured `--person` (default `owner`), so existing tokens and rows stay theirs ([§12](#12-migration-and-compatibility)).

`Role` is a closed set with an order: `member < admin < owner`. It is parsed once at the edge (`ParseRole`); an unknown value is refused.

### 4.2 Invite and reset link

One record serves two purposes: joining, and setting a new password for an existing user.

```go
type Purpose string // "join" | "reset"

type Invite struct {
	ID        string    // random, shown to its creator, used to revoke
	Hash      string    // SHA-256 of the token; the token itself is never stored
	Purpose   Purpose
	Role      Role      // join: the role the new user gets
	Person    string    // reset: the user whose password it sets
	CreatedBy string    // person id
	CreatedAt time.Time
	ExpiresAt time.Time // CreatedAt + 72h
}
```

- The token is 32 random bytes, base64url. The console shows the link once: `<public URL>/invite#<token>`. The token sits in the fragment, so it never reaches a server log, a proxy log or a `Referer`. There is no email: the creator shares the link out of band.
- At most 20 links wait at a time. Expired ones are dropped on every write, as sessions are today.
- Only Owners create join links, for any role.
- Reset links follow the role ladder, so a reset can never take over an equal or higher account: an Owner may create one for any other user, and an Admin only for a Member. Nobody creates one for themselves; a user changes their own password in Account. A new reset link for a user replaces their previous one.

### 4.3 Session

`StoredSession` gains `Person`. A login always starts a new session id, as today. The session holds no role: `authed` loads the user on each request, so a demotion or a removal takes effect on the next request.

### 4.4 Audit entry

```go
type AuditEntry struct {
	At     time.Time
	Actor  string            // person id
	Action string            // "invite.created", "role.changed", ...
	Target string            // person id, invite id, connector id
	Detail map[string]string // old and new role, key count; never a secret
}
```

The console keeps the newest 500 entries in its state file. Each entry is written in the same atomic write as the change it records, so the log cannot miss a change or record one that did not happen. Engine-side acts are also in the engine's audit, under the acting person ([§6.3](#63-who-the-console-acts-as)).

The audited actions are: `user.created` (first run, invite accepted), `invite.created`, `invite.revoked`, `reset.created`, `reset.used`, `role.changed`, `user.removed`, `username.changed`, `password.changed`, `password.reset` (host socket), `backup.exported`, `backup.restored`, `setup.imported`, `slot.applied`, `slot.discarded` and `connector.credentials_changed`.

### 4.5 State file

```go
type stateFile struct {
	Owner  *Owner            `json:"owner,omitempty"` // read once for migration, never written again
	Users  map[string]User   `json:"users,omitempty"`
	Invites map[string]Invite `json:"invites,omitempty"`
	Audit  []AuditEntry      `json:"audit,omitempty"`
	LegacyPerson string      `json:"legacy_person,omitempty"` // owner of records made before multi-user
	// existing fields; Agent, StoredChannel and Device gain an Owner (person id)
	Credentials map[string]CredentialMode `json:"credential_modes,omitempty"`
	...
}
```

`State` stays the one aggregate. Its `update` is already the consistency boundary: one lock, a copy, one atomic file write. U1, U2, U3 and U5 are checked inside `update`, so no caller can skip them.

### 4.6 First run

First run is "no users". `POST /api/setup` creates the first Owner with the configured `--person`, signs them in and closes first run in the same write. A second setup gets `409`, as today. The first-run restore of a backup also works only while no user exists.

The first run is open to whoever reaches the console first. That is acceptable on loopback, today's default. An install reachable from a network must finish the first run before it is exposed ([§13.9](#139-exposure-beyond-loopback)).

## 5. Roles and permissions

### 5.1 Capabilities

The console checks one table of capabilities. Each route declares one. The table is the single source: `GET /api/session` returns the signed-in user's capabilities, so the web app hides what the user cannot do and never re-implements the rules.

| Capability | Owner | Admin | Member |
|---|---|---|---|
| `people.manage`: invite, revoke invites, change roles, remove people | yes | | |
| `people.read`: see people, invites and the audit log | yes | yes | |
| `people.reset`: create a reset link for a user of a lower role ([§4.2](#42-invite-and-reset-link)) | yes | yes, Members only | |
| `backup.full`: export or restore a full backup | yes | | |
| `connectors.manage`: import, remove, function access, credential mode, shared sign-in, new-key default | yes | yes | |
| `skills.manage` | yes | yes | |
| `setup.manage`: export or import a setup file | yes | yes | |
| `activity.settings`: what is recorded | yes | yes | |
| `activity.delete` | yes | | |
| `keys.any`: see, narrow or revoke anyone's key, apply a slot to any key, discard a slot, approve "always" | yes | yes | |
| `approvals.any`: see everyone's pending calls, cancel one, decide one on a shared-credential connector | yes | yes | |
| `activity.any`: see everyone's activity | yes | yes | |
| `keys.own`: make, renew, narrow and revoke own keys; connect apps; claim a slot for an own key | yes | yes | yes |
| `approvals.own`: decide own calls once or for the session | yes | yes | yes |
| `activity.own`, `notifications.own`, `signin.own`, `account.own` | yes | yes | yes |

### 5.2 Ownership checks

A capability opens a route. An ownership check then picks the rows. Routes that name a key (`/api/agents/{name}`, `/api/connectors/{id}/profiles/{agent}`, `/api/skills/{id}/keys/{agent}`), an approval, a capture, a channel or a device load the row's owner and refuse `404` unless it is the user, or the user holds the matching `*.any` capability. `404`, not `403`, so a Member cannot probe other people's key names.

One function answers both questions:

```go
// may reports whether user may act on a row owned by owner with capability own, or with any when it is someone else's.
func may(u User, own, any Capability, owner string) bool
```

### 5.3 Where each rule is enforced

The console is the policy enforcement point for console roles and ownership. Engine roles are a coarse second line that separates Members from managers. The table shows why the console cannot leave the Owner/Admin line to the engine: importing a connector needs `listing.create.private`, `catalog.write` and `client.register`, which only `tenant_admin` holds, and `tenant_admin` also holds `role.manage`.

| Console action | Engine actions | Least engine role | Enforced by |
|---|---|---|---|
| import, remove a connector | `listing.create.private`, `catalog.write`, `connector.install`, `connector.manage` | `tenant_admin` | engine (Member refused), console (Owner vs Admin irrelevant) |
| shared OAuth client of an issuer | `client.register` | `tenant_admin` | engine (Member refused) |
| function access (workspace rows) | `grant.workspace`, `function.manage` | `workspace_admin` | engine (Member refused) |
| shared credential (headers, OAuth `owner: workspace`) | `credential.workspace` | `workspace_admin` | engine (Member refused) |
| invite, role change | `role.manage` | `tenant_admin` | console (Admin refused); the role row is written by the service person |
| remove a person | `person.remove`, `agent.remove` | `workspace_admin` | console (Admin refused) |
| make, renew, revoke own key | `binding.write`, `grant.agent` | `agent_configurer` | console (ownership); engine refuses users without the role |
| narrow own key | `grant.agent` | `agent_configurer` | console (ownership) |
| own provider sign-in, own API key | `credential.own` on self | none (self action) | engine |
| decide own approval | `approval.decide`, named approver | none (tenant action) | engine |
| decide anyone's approval on a shared-credential connector | `approval.decide`, approver through the `managers` group | none, plus group membership | engine |
| cancel anyone's approval | `approval.cancel` | `workspace_admin` | engine (Member refused) |
| "always allow" from an approval | `grant.workspace` | `workspace_admin` | console hides it for Members; engine refuses |
| reads: connectors, grants, calls, approvals, credentials, explain | `connector.read`, `audit.read`, `approval.read`, `connector.manage` | varies | service person; the console filters by ownership ([§6.3](#63-who-the-console-acts-as)) |

So the engine roles are:

| Console role | Engine role row |
|---|---|
| Owner | `tenant_admin` over `t/<tenant>` |
| Admin | `tenant_admin` over `t/<tenant>` |
| Member | `agent_configurer` over `t/<tenant>/w/<workspace>` |

Every Owner and Admin is also a member of the engine group `managers` (flag `--managers-group`). The reconcile ([§6.4](#64-reconcile)) keeps that group in step with the roles, in the same fail-closed order.

Accepted trade-off. A Member's engine role covers every agent of the workspace, not only their own. Only a console bug could use that, because person tokens never leave the console process. Even then the damage is bounded: agent rows only narrow the workspace policy ([§2](#2-what-the-engine-already-gives)), and a key is usable only with its token, which only its owner holds. The worst case is that one Member narrows or unbinds another Member's key: an availability problem, never a confidentiality problem or a gain in capability. [§14](#14-engine-changes) lists the optional engine change that would scope the role to one key.

## 6. Engine identity

### 6.1 Persons

Each user is one engine person, `User.Person`. The console mints a person token for that id through `/local/token`, with the operator token it already holds. A user never sees or holds a person token.

### 6.2 The service person

The console needs an identity that is not a human, for two reasons:

- Some writes have no session user: writing role rows and `managers` group members when an invite is accepted, and the boot reconcile.
- Reads must cover rows a Member's token cannot read, such as `GET /v1/calls`, which needs `audit.read`.

The service person is `console` (flag `--service-person`, env `CONSOLE_SERVICE_PERSON`). The person profile's `bootstrap.roles` makes it `tenant_admin`:

```yaml
bootstrap:
  roles:
    - {tenant: local, person: console, role: tenant_admin, scope: t/local}
```

The row `{person: owner, role: tenant_admin}` leaves the person profile. Existing installs keep that row in their database, because the engine never deletes bootstrap rows. The console's reconcile then owns it. If the row stayed in the config, `app.bootstrap` would give a removed or demoted first Owner their engine role back at every restart. Removing it from the config ends that.

The service person never decides an approval, never signs in to a provider and never owns a key. Only the console holds its tokens.

### 6.3 Who the console acts as

The rule:

- **Writes and decisions run as the person of the user who asked.** The engine's audit, `DecidedBy`, `CreatedBy` on grants and the binding events name that person. This removes the "always the same actor" blocker.
- **Background writes** run as the person the write is for: a refresh-token renewal or an expiry renewal acts as the key's owner, "always allow" after a run acts as the Admin who chose it, and a channel or device decision acts as the channel's owner.
- **Role rows and the reconcile** run as the service person. The console's audit names the human who caused them.
- **Reads** run as the service person. The console filters them by ownership ([§5.2](#52-ownership-checks)). This puts read scoping in one place, because the engine has no "own activity" read for a Member.

#### Interface: design it twice

The question is how an engine call learns its actor. Today `Engine.Call` uses the one `Engine.Person`.

| | A. Actor in the context | B. `Engine.As(person)` returns a bound copy |
|---|---|---|
| Interface | `WithActor(ctx, person)`; `Engine.Call` (writes) needs one, `Engine.Read` uses the service person | every caller holds a bound engine |
| Change size | the `authed` middleware and the few background entry points set the actor; the ~60 functions below them are untouched, since every one already takes `ctx` | either a new parameter through every `Console` method that reaches the engine, or a per-request `Console` copy (it holds mutexes) |
| Failure if forgotten | `Engine.Call` returns an error: fail closed | does not compile |
| Locality | identity is set at the edge and read in one place | identity is visible at every call site |

**Chosen: A.** Identity is request-scoped data, which is what context values are for. The cost of B is a parameter threaded through the whole console for no extra safety, since A fails closed. One test asserts that a write with no actor is refused and makes no engine request.

Inside `Engine`, the one cached person token becomes a cache keyed by person, holding at most one live token per user. It is minted again on `401`, as today.

### 6.4 Reconcile

`reconcileRoles(ctx)` runs as the service person at start and after a restore. It lists the tenant's role rows and the members of the `managers` group, computes the wanted rows from `State.Users` ([§5.3](#53-where-each-rule-is-enforced)), puts the missing ones and deletes the rows of console-managed persons that should not be there. Rows for persons the console does not know (an operator's own rows) are left alone, except the legacy `owner` row when no Owner user is `owner`.

A role change writes in the order that fails closed:

- **Raising** a role writes the console state first, then the engine row. If the engine write fails, the console shows the new role but the engine refuses the actions. The People page shows "not applied yet, retry", and the next reconcile repairs it.
- **Lowering** a role or removing a person writes the engine first, then the console state. If the console write fails, the engine already refuses.

A partial failure therefore always leaves the engine less privileged than the console.

## 7. Keys and approvals

### 7.1 Key ownership

- `Agent` gains `Owner` (person id). `PutAgent` keeps the owner of an existing key and refuses a different one (U5).
- Key names stay one namespace per install, because the agent id is engine-wide: it keys bindings, grants, revocations, setup profiles and activity. When a user picks a taken name, the console refuses with "this name is taken" and proposes the next free one (`laptop-2`). Prefixing names with usernames was rejected: usernames rename, and the prefix would leak into tool names and setup files.
- Agent tokens are minted with `on_behalf_of` and `agent_owner` set to the key owner's person (U6). Bindings are written with `agent_owner` set to the same person, so the engine's `RemovePerson` marks them `OwnerRemoved`.
- `agentFor` (an app connecting through the browser) must also check that a reused name belongs to the consenting user. Today it reuses a key by OAuth client id alone, so a second person consenting for the same client would get the first person's key.
- Per-key profiles (#54) stay as they are. A Member narrows only their own keys. Admins narrow any key.
- Key slots (`feat/key-slots`, [§16](#16-key-slots)) are shared, narrow-only permission templates. A slot is never owned. It becomes part of a key only when someone claims it, and the key it lands on keeps its own owner.

### 7.2 Who decides

Who may decide follows the connector's credential mode ([§8](#8-credentials-per-connector)), because the mode says whose name the call runs in:

| Credential mode | The call acts as | Approvers written on every binding | Who decides |
|---|---|---|---|
| `per_person` | the key's owner, with their own provider sign-in | `ask_requester: true`, `approvers: []` | the key's owner only |
| `shared` | the organisation, with its one credential | `ask_requester: true`, `approvers: ["managers"]` | the key's owner, or any Owner or Admin |

- The engine enforces the table: `approval.Decide` accepts only a named approver, and a decider matches the `managers` group through their memberships. `DecidedBy` is always the deciding person.
- The approvers are copied onto each held call when it is created. Changing a connector's mode rebinds its keys, so calls held after the change follow the new rule, and calls already held keep the rule they were held under.
- Owners and Admins see every pending call, labelled with whose it is, and can **cancel** any of them (the engine's `approval.cancel`). On a `per_person` connector they cannot approve: approval there is consent to use someone else's own credential, so it stays with that person.
- "Always allow this function" writes a workspace row (`SetAccess`). Members are not offered it, and the server refuses it from a Member. Members get "Allow once", "Allow for this session" and "Deny".
- Elicitation in the app decides as the key owner, since the app holds that owner's key.
- Notifications go to the key owner's channels only. Managers decide shared-credential calls from the Approvals page ("Everyone"), so a busy team does not notify every manager for every call.

### 7.3 Channels and devices

`StoredChannel` and `Device` gain `Owner`. Each user configures their own notification channels and pairs their own devices. The approvals feed keeps one watch, as the service person. For each approval it notifies the channels of the key's owner (the approval's `on_behalf_of`). A decision from a channel or a desktop link acts as the channel's or the device's owner, and the engine refuses it when that owner is not an approver.

A chat channel answers for its owner, so a channel set up in a shared chat lets everyone in that chat answer for its owner. The Notifications page says so when a chat channel is added.

## 8. Credentials per connector

Each connector has a credential mode, set by an Owner or Admin:

| Mode | UI label | Engine `credential_policy` on every binding of the connector | Who signs in |
|---|---|---|---|
| `shared` | "One account for everyone" | `shared` (the workspace's credential) | an Owner or Admin, once |
| `per_person` | "Each person signs in" | `per_user` (the credential of `on_behalf_of`) | each person, for themselves |

The same mode sets the binding's approvers ([§7.2](#72-who-decides)). One setting answers both "whose credential" and "who may approve", so the two can never disagree.

- The mode is stored in `State.Credentials`, per connector, like `NewKeysOff`. `bind` reads it. Changing it rebinds every key of the connector under the connector's lock (`PutBinding` replaces the policy).
- The default is `shared`, which is what every connector does today. A household never sees the setting until a second user exists.
- `OwnerKindsAllowed` on the listing limits the choice. Without `user`, only `shared` is offered. Without `workspace`, only `per_person` is offered, and a new connector starts there.
- In `per_person` mode, "Sign in" on the connector page begins `credentials:connect` with `owner: user` as the signed-in user. The pending OAuth state records the user. The callback completes it only for the session of that same user; the engine also checks `pend.Person`. Header secrets and API keys are stored with `owner_kind: user` and the user's person.
- A call with no credential for its person gets the engine's `credential_required` with `connect: true`. Activity shows "sign-in needed", and the key owner's connector page shows the Sign in button.
- Owners and Admins see "3 of 5 people signed in". Nobody can sign in, or store an API key, for another person.

Switching a connector from `shared` to `per_person` keeps the shared credential stored but unused. Switching back uses it again. The connector page says which credential is in use.

## 9. Activity

- `Capture` gains `Person`, taken from the agent token's `on_behalf_of` when the call is captured. Scoping is decided at write time and never changes, so the activity of a revoked key or a removed person keeps its owner.
- Captures made before multi-user have no `Person`. They belong to `State.LegacyPerson`, the first Owner.
- The engine's call rows (`/v1/calls`) are read as the service person, for the agents the viewer may see: their own keys, or all for `activity.any`.
- Members cannot clear activity. "Delete all activity" is Owner only, because the log is also the team's record.

## 10. People lifecycle

### 10.1 Invite and accept

1. An Owner creates an invite: role, then a copy box with the link, "works once, expires on <date>".
2. The invitee opens `/invite#<token>`. The page posts the token to `POST /api/invite/inspect` and shows "alice invited you as Member", or "this invite no longer works, ask alice for a new one".
3. The invitee picks a username and a password and posts `POST /api/invite/accept`. In one `State.update` the console checks U2 and U3, checks the username, creates the user, deletes the invite and writes `user.created` to the audit. Then it signs the user in with a new session.
4. The console writes the user's engine role as the service person (raising, so state first).

### 10.2 Reset a password

1. An Owner, or an Admin for a Member, chooses "Reset password" on the person's row. The console shows the link once, with its expiry.
2. The person opens it. The page posts the token to `POST /api/invite/inspect` and shows "alice made this link for bob to set a new password".
3. The person sets a new password with `POST /api/invite/accept`. In one `State.update` the console checks U3 and that the creator still outranks the target, sets the password, ends all the target's sessions, deletes the link and writes `reset.used`. Then it signs the person in with a new session.

A reset changes only the password. The person id, keys, sign-ins and activity stay. `make reset-password` on the host stays the break-glass route, for an Owner who is locked out ([§11](#11-account)).

### 10.3 Change a role

`PUT /api/people/{person}/role`, Owner only. The change follows the fail-closed order of [§6.4](#64-reconcile). U1 refuses demoting the last Owner, including demoting yourself.

### 10.4 Remove a person

`DELETE /api/people/{person}`, Owner only. U1 refuses removing the last Owner. Every step is idempotent, so an Owner's retry finishes a partial removal.

1. Engine `RemovePerson`, as the acting Owner. From this moment step 1 denies every call carrying the person, and their credentials, roles and pending approvals are gone.
2. `RevokeAgent` for each of their keys, which records the revocation and unbinds the key. The console's `/mcp` proxy refuses the keys' tokens, and refresh grants stop renewing.
3. In one `State.update`: delete the user, their sessions, their channels, their devices, their refresh grants and the invites they created, and write `user.removed` with the key count.

The confirmation names the consequences: "Bob is signed out, Bob's 3 keys stop working now, and Bob's sign-ins to providers are deleted. Bob's activity stays."

A removed person cannot come back with the same identity. A new invite makes a new person id, so the engine's tombstone never blocks the new account.

## 11. Account

- Every user changes their own username and password. A password change asks for the current one and ends that user's other sessions only. Today `SetPassword` ends every session in the install.
- A user who forgot their password gets a reset link from someone above them ([§10.2](#102-reset-a-password)).
- `make reset-password USER=<username>` resets one user's password through the host socket, as today. Access to the machine is the authority. It is the break-glass route for an Owner, including the last Owner, who has nobody above them.
- Consent for an app connecting through the browser asks for the signed-in user's own password. Today `ownerConfirms` asks for the Owner's.

## 12. Migration and compatibility

On the first start of the new console, `OpenState` migrates in memory, and the next write persists the result:

| Before | After |
|---|---|
| `owner: {username, hash}` | `users[<--person>] = {role: owner, ...}`; `legacy_person = <--person>` |
| sessions without a person | owned by the first Owner, so nobody is signed out |
| agents, channels and devices without an owner | owned by the first Owner |
| captures without a person | read as the first Owner's |
| no credential modes | every connector `shared`, its behaviour today |

- Existing agent tokens already carry `on_behalf_of: owner`. The first Owner is person `owner`, so these tokens keep working and their approvals still reach the right person.
- Backups. `decodeState` accepts both shapes and migrates the old one, so old backups restore into the new console. A backup made by the new console has `users` and no `owner`, so an older console refuses it ("it has no owner account"). This is a one-way door, and the backup format version changes so the refusal names it.
- Setup files. They carry key slots as `feat/key-slots` defines them ([§16](#16-key-slots)). `credentials: shared | per_person` is added per connector, optional, default `shared`. Setups never hold users, invites, owners of keys or sign-ins.
- A full backup restores the exact keys, each with its owner, and the unclaimed slots as they were; it makes no new slots. Waiting profiles from before key slots migrate to slots as `feat/key-slots` defines; a slot has no owner, so this migration needs nothing from multi-user.
- Flags. `--person` now names the first Owner's person. `--service-person` (default `console`) is new and must match `bootstrap.roles`. `--managers-group` (default `managers`) is new. The person profile and its compose file change together.
- The headless profile and `cmd/connector-engine` are untouched.

## 13. Security

### 13.1 Invite brute force

The token has 256 bits and only its SHA-256 is stored, so guessing is infeasible and a state file leak exposes no link. `inspect` and `accept` share a limit of 10 failures a minute for the whole console (`429` with `Retry-After`), so failed attempts are slow and visible in the log. Invites and reset links expire after 72 hours, work once and die when their creator loses the role that allowed them.

### 13.2 Password policy

One constructor for every password: the first run, an accepted invite, a password change and a host reset all reuse today's `newOwner` checks (renamed `newCredentials`): at least 10 characters and PBKDF2-SHA256 with 600,000 iterations. Usernames are trimmed and unique by case-insensitive compare. The login finds the user by the folded name.

### 13.3 Lockout

Today one global window counts every wrong password in the install, so any member could lock the Owner out.

- The limit becomes 5 failures a minute per username, plus a ceiling of 30 a minute for the whole console that bounds an attack spread over many names.
- Failures for unknown usernames are counted under the folded name, in a map bounded to 1,000 entries.
- An unknown username still computes a PBKDF2 hash, against a fixed dummy salt, so response time does not reveal which usernames exist. The error is always "wrong username or password".
- Lockout always expires. There is no permanent lock that an attacker could use to shut someone out.

### 13.4 Session fixation

Every login, every accepted invite and every used reset link starts a new random session id; the server never adopts an id from the client. A password change ends the user's other sessions. A used reset link and a removal end all of them. The role is read live (U4), so a session never carries a stale privilege.

### 13.5 CSRF

No change: the per-session CSRF token, `SameSite=Strict`, and the guard's Origin check on every mutating request. `invite/inspect` and `invite/accept` are unauthenticated POSTs. The guard's Origin check covers them, and the invite token itself authenticates them.

### 13.6 Privilege escalation paths

| Path | Closed by | Test |
|---|---|---|
| A Member calls an Owner or Admin route | capability table, deny by default | table-driven over every route and role |
| A route added later without a capability | a test that walks the mux and fails on any route missing from the capability table or the public allowlist | the same test |
| A Member acts on another's key, approval, capture, channel or device by id | ownership check, `404` | per resource |
| A Member uses "always allow" to write a workspace row | not offered; refused server-side | approval test |
| A Member approves someone else's call | engine: named approver only | e2e |
| A manager approves a call that uses someone's own credential | `per_person` bindings name no approvers besides the key owner | e2e |
| A Member applies a slot to another person's key | ownership check; applying to any key needs `keys.any` | slot test |
| A reset link takes over an equal or higher account | an Admin resets only Members; nobody resets themselves; the link dies if its creator loses the role | reset test |
| A Member reuses another person's browser-connected key | `agentFor` checks the owner | consent test |
| An Admin changes roles or removes people | `people.manage` is Owner only; role rows go only through the service person in Owner flows | route test |
| An invite grants more than its creator holds | only Owners invite; the invite dies if its creator is no longer an Owner | invite test |
| A reset link, or an invite, is replayed | deleted in the same write that uses it | invite and reset tests |
| Stale role in a session after demotion | role read per request | session test |
| A restore replaces the users | `backup.full` is Owner only; the first-run restore works only with zero users | backup test |
| A credential planted for another person | the console only stores credentials for the acting user | credential test |
| An OAuth callback finished by another user | the console's pending state records the user; the engine checks `pend.Person` | OAuth test |
| A removed user's open session or key | sessions deleted; engine tombstone; console revocation | e2e |

### 13.7 Audit of admin actions

Console-only acts are in the console audit ([§4.4](#44-audit-entry)), written atomically with the change. Engine acts are in the engine audit under the acting person. The People page shows the newest console entries to Owners and Admins. A full backup carries both.

### 13.8 Secrets

The audit, the capability list and `GET /api/people` never carry a password hash, a link token or a session id. A link token is shown once and never logged.

### 13.9 Exposure beyond loopback

Several people usually means the console is reached from other machines. The defaults stay loopback. For a team:

- Set `CONSOLE_PUBLIC_URL` to the address people use. The Host allowlist, the OAuth redirect and invite links all follow it.
- Put TLS in front, through a reverse proxy. When the public URL is `https`, the session cookie gets `Secure`.
- Finish the first run before exposing the console.

The invite dialog warns when the public URL is a loopback address, because nobody else can open such a link.

## 14. Engine changes

None are needed. Two generic, optional changes would move checks from the console into the engine. Neither is in the plan; each waits for a concrete need:

- **Agent-scoped binding writes.** `PutBinding` and `DeleteBinding` would also authorize `binding.write` at `authz.Agent(t, w, agent)`, as `grantAuth` already falls back for `grant.agent`. A Member's engine role could then cover only their own keys, closing the residual risk in [§5.3](#53-where-each-rule-is-enforced).
- **Self-explain.** `ExplainAccess` would also allow a person asking about a subject whose `on_behalf_of` is themselves. Member reads of their own key's effects could then run as the Member.

## 15. Phased plan

Each PR merges on its own and leaves a one-user install behaving as today. Every PR runs the race suite, `golangci-lint`, `scripts/check-layering.sh` and the web checks.

| PR | Scope | Acceptance |
|---|---|---|
| 1. Users and sessions | `User`, `Role`, `State.Users`, migration of `owner`, sessions bound to a person, `authed` loads the user, U1 in `State`, `SetPassword` per user, `decodeState` for both shapes, `reset-password USER=` | an old state file fixture migrates and its session still works; an old backup restores; demoting or removing the last Owner is refused; one user's password change keeps the others' sessions |
| 2. Acting identity | actor in the context, `Engine.Call` / `Engine.Read`, per-person token cache, service person flag and profile row, `reconcileRoles` with the `managers` group, `Agent.Owner`, agent tokens and bindings name the key owner, a slot claimed at consent or key creation lands on the claimer's key, decisions as the user | a fake engine records the `sub` of each write and sees the user's person; a write with no actor is refused; `DecidedBy` is the deciding user; reconcile repairs a deleted role row and a missing group member; a slot chosen at consent gives the consenting user's key its profile |
| 3. Capabilities and ownership | the capability table, route declarations, `may`, `Capture.Person`, scoped keys, approvals and activity, "always" refused for Members, `agentFor` owner check, slot apply and discard rules | a table test over every route and role; the mux-walk test; a Member gets `404` on another's key, approval and capture; a Member applies a slot to their own key and gets `404` on another's |
| 4. Invites and people | invite create, list, revoke, inspect, accept; reset links; role change; remove; console audit; per-username lockout; `Secure` cookie | the two-user end-to-end test below; an Admin's reset link for an Owner or another Admin is refused; a used reset link ends the target's sessions and keeps their keys |
| 5. Credentials and approvers per connector | `State.Credentials`, rebinding on change with the mode's approvers, `OwnerKindsAllowed`, per-user OAuth state, per-user header secrets, setup field | in `per_person` mode two users' calls use their own credentials and only the key owner approves; in `shared` mode an Admin approves a Member's call; a call before sign-in gets `credential_required`; switching modes keeps the shared credential |
| 6. Channels and devices per user | `Owner` on channels and devices, feed routing to the approvers' channels, decisions as the channel owner | an approval for Bob reaches only Bob's channel; Alice's device link cannot decide Bob's call |
| 7. Web | underline tabs on Account, People tab, invite dialog, `/invite` page, role badge, Mine/Everyone switch, Member read-only states, connector sign-in card | component tests for each; vocabulary test; one-user snapshot of every page unchanged |

The in-process end-to-end test of PR 4 runs a real engine, like `console_test.go`, with an Owner and a Member:

- each makes a key; each sees only their own key in `GET /api/agents`, and the Owner sees both under "everyone";
- each calls a tool; each sees only their own capture in Activity;
- on a `shared` connector, a held call from the Member's key is approvable by the Member and by the Owner, and `DecidedBy` names whoever decided;
- on a `per_person` connector, a held call from the Member's key is approvable by the Member, is refused for the Owner (`not_an_approver`), and the Owner can cancel it;
- the Member gets `403` on importing a connector, changing function access, inviting, creating a reset link, changing a role, exporting a full backup, discarding a slot and "always allow", and `404` on the Owner's key, approval and capture;
- the Owner removes the Member: the Member's session gets `401`, the Member's key token gets `401` at `/mcp`, and a call through the engine directly is denied with `person_removed`;
- the Owner cannot demote or remove themselves as the last Owner.

## 16. Key slots

`feat/key-slots` lands before this work. Its model, as the coordinator described it:

- A shared setup carries key profiles as **key slots**: named, narrow-only permission templates with per-connector modes and overrides, never credentials.
- On import, the importer picks per slot: keep it as a slot (the default), apply it to an existing key, or discard it. Nothing binds by name any more.
- A slot is claimed on the consent page (`/connect`: "Use permissions from: [slot] / Default", which consumes the slot), on manual key creation, or from "Unclaimed permission sets" on the Access page (apply to a key, or discard).
- Waiting profiles migrate to slots. A full backup restores the exact keys and the unclaimed slots as they were, and makes none.

Multi-user adds one rule to each way in:

| Act | Who may | The key's owner |
|---|---|---|
| keep, apply or discard slots while importing a setup | `setup.manage` (Owner, Admin) | unchanged when applied to an existing key |
| claim a slot on the consent page | the consenting user (`keys.own`) | the consenting user: the new app's key is theirs (U5) |
| claim a slot on manual key creation | the creating user (`keys.own`) | the creating user |
| apply an unclaimed slot to an existing key | `keys.own` for an own key; `keys.any` for anyone's | unchanged |
| discard an unclaimed slot | `keys.any` (Owner, Admin) | none |

- A slot has no owner. Unclaimed slots are listed to every user, since any user may claim one for a key of their own.
- Applying a slot never changes a key's owner and, because slots only narrow, never widens what the key can do.
- Claiming or applying writes the key's agent rows as the acting user ([§6.3](#63-who-the-console-acts-as)), so the engine's audit names who did it. The console audit records `slot.applied` and `slot.discarded`.
- Slot names are template names, not key names, so they do not take a name in the key namespace ([§7.1](#71-key-ownership)).

PR 2 checks this section against the merged key-slots code.

## 17. UI

The console's [DESIGN.md](../../selfhosted/console/web/DESIGN.md) applies throughout: one `PANEL` per page, containers aligned with containers, Hick (at most three primary actions per view, the rest in one overflow `Select`), Miller (filters past six rows, counts in headings).

**One user sees no change.** No role badge, no Mine/Everyone switch, no person columns. The only new surface is the People tab, with "Invite someone". Multi-user chrome appears once a second user exists.

### 17.1 Account

- Account gets underline tabs: **Profile** (username, password), **People** (Owner and Admin), **Backups** (Owner). Members see Profile alone, with no tab bar. The tabs are `role="tablist"` with arrow-key movement, and each tab is a path (`/account`, `/account/people`, `/account/backups`). The Tabs recipe goes into DESIGN.md in PR 7, unless it lands there first.
- **People**: a `PANEL` titled "People" with the mono count and one primary action, "Invite". One row per person: the username, "you" on your own row, a role `Tag`, the number of keys and the joined date. Owners also get a role `Select` and "Remove", which turns into the `InlineConfirm` that names the consequences ([§10.4](#104-remove-a-person)). "Reset password" shows on the rows the viewer may reset ([§4.2](#42-invite-and-reset-link)) and opens a `Sheet` with the link in a `CopyBox`. Owner rows offer the role `Select`, "Reset password" and "Remove" in one overflow `Select`, so a row keeps a single visible action. Pending invites follow in a `WELL`: role, "expires in 2 days", "Revoke". The newest audit entries come last, inside a `Disclosure` ("Recent changes").
- **Invite** (`Sheet`): one `OPTION_CARD` per role, each with its line ("Admin: manages connectors, skills and policy"; "Member: uses the gateway with their own keys"). Then the `CopyBox` with the link, the expiry, and a `Notice`: "Send it privately. It works once."

### 17.2 Invite and reset pages

A `Frame` page outside the sections, `/invite`, serves both purposes. For an invite: the `Display` headline ("Join this gateway", with its one lime word), "alice invited you as Member", the username field, the password field twice, and one primary button. For a reset: "Set a new password", "alice made this link for bob", the password field twice, and one primary button. A dead link shows an `EmptyState`: "This link no longer works", "Ask alice for a new one."

### 17.3 Scoping and clarity

- **Role badge**: a mono `Tag` with the role beside the Account link in the shell, once a second user exists.
- **Mine / Everyone**: Access, Approvals and Activity get a `Segmented` "Mine | Everyone" for users with the `*.any` capability, defaulting to Mine. With Everyone chosen, the count in the `PageTitle` reads "everyone · 12", and each row carries its owner's username as a `Tag`. Activity's filter row gains a person `Select`.
- An approval for someone else shows "waiting for bob". On a `shared` connector it offers "Allow once", "Allow for this session", "Deny" and "Cancel this call"; on a `per_person` connector it offers only "Cancel this call", with the caption "bob decides: it uses bob's own sign-in".
- **Connectors for Members**: function access shows as read-only (the `Segmented` disabled, with the caption "Set by your admins"). The per-key profile of the Member's own keys stays editable. The sign-in card shows the Member's own state: "You are signed in" or "Sign in with Google".
- **Connectors for Owners and Admins**: a "Who signs in" `Choice` ("One account for everyone" / "Each person signs in"), whose note says who approves in each mode ("Owners and Admins can also approve calls" / "Only the key's owner approves calls"), and "3 of 5 people signed in".
- **Consent and key creation**: "Use permissions from" lists the unclaimed slots and "Default", as `feat/key-slots` draws it; the key made is the signed-in user's.
- **Notifications** is titled "Your notifications" once a second user exists.

## 18. Decisions

| Id | Decision | Alternatives rejected, and why |
|---|---|---|
| MU-1 | Invites only: single-use, 72 h, SHA-256 stored, token in the fragment | an open sign-up page (the request); emailed links (no mail in a self-hosted install) |
| MU-2 | The console enforces roles and ownership; engine roles separate Members from managers | Admin as `workspace_admin`: cannot import connectors; Member with no engine role: cannot bind their own keys |
| MU-3 | A dedicated service person `console` for role rows and reads; the first Owner keeps person `owner` | the first Owner as the service identity: their demotion or removal would be undone at every boot; a new person id for the first Owner: existing tokens carry `on_behalf_of: owner` |
| MU-4 | The actor travels in the context; writes fail closed without one | `Engine.As(person)`: threads a parameter through the whole console for no extra safety |
| MU-5 | Key names stay one namespace per install | per-user prefixes: usernames rename and the prefix leaks into tool names |
| MU-6 | Who approves follows the credential mode: on `per_person` connectors only the key owner; on `shared` connectors also any Owner or Admin, through the engine group `managers`. Owners and Admins can always see and cancel | managers approve everywhere: consent to use someone's own credential stays with that person; only the key owner everywhere: a `shared` call acts as the organisation, so its managers may answer for it; a separate approver setting: it could disagree with whose credential the call uses |
| MU-7 | Credential mode per connector, default `shared` | per install: companies want both; per key: too fine to explain |
| MU-8 | Console audit as a bounded list in the state file, written atomically with the change | a separate JSONL log: can miss or invent an entry on a crash between the two writes |
| MU-9 | No engine change; two optional ones recorded in [§14](#14-engine-changes) | change the engine first: no current need, and the headless product would carry the risk |
| MU-10 | Reset links are invites with purpose `reset`: same hashing, expiry and single use; an Owner resets anyone else, an Admin only Members; `make reset-password` stays the break-glass route | email resets: no mail in a self-hosted install; Admins resetting Admins: a lateral takeover |
| MU-11 | A key slot has no owner; claiming it puts it on the claimer's key, and applying it never changes a key's owner | slots owned by their importer: a setup is shared policy, not a person's |

## 19. Open questions

None. The key-slots model ([§16](#16-key-slots)), reset links (MU-10) and approvals on shared-credential connectors (MU-6) were decided by the coordinator on 2026-11-20.
