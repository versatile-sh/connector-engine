# Security policy

## Report a vulnerability

Report it privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability**. Do not open a public issue, pull request or discussion.

Include:

- the affected version or commit;
- the profile (`person` or `headless`) and the adapters in use;
- steps to reproduce, or a proof of concept;
- the impact you expect.

We acknowledge a report within 5 working days and agree on a disclosure date with you. Fixes are released before details are published.

## Supported versions

| Version | Supported |
|---|---|
| latest release | yes |
| `main` | yes, best effort |
| older releases | no; upgrade to the latest release |

Until 1.0, only the latest minor release receives security fixes.

## Threat model summary

The engine is a policy point between AI apps and tool servers. It assumes the agent and the upstream tool server may be hostile, and that the host and its administrator are trusted.

| Asset | Protection |
|---|---|
| Which functions an agent may call | four grant layers, catalog ceilings, a forced ask for destructive functions and for risky exposure; a denied function looks like an unknown one |
| What a function does | definitions pinned by RFC 8785 hash; a change or a new function is quarantined until a person accepts it |
| Approval decisions | an approved call runs once; repeats attach to the same record; chat surfaces accept clicks only from the configured owner id; link tokens are single-use HMACs |
| Upstream credentials and console secrets | envelope encryption with a key per tenant; destroying the key shreds the data; secrets are never returned by an API |
| Tenant isolation | Postgres row-level security per tenant; a superuser database role is refused at boot |
| The local console | listens on `127.0.0.1` only, with a Host allowlist, Origin checks and CSRF tokens; AI apps sign in with OAuth 2.1 and PKCE S256 |
| Call arguments | the engine stores a hash; the console's activity log redacts likely secrets before writing |

Out of scope:

- a compromised host, root on the host, or anyone with access to the Docker socket;
- a running, unlocked install whose process memory is read;
- an agent token holder approving its own call through MCP elicitation, when the owner has turned elicitation on for that app (it is off by default).

Details:

- [selfhosted/docs/SECRETS.md](selfhosted/docs/SECRETS.md): root keys, the host keychain and their threat model.
- [selfhosted/docs/approvals-surfaces.md](selfhosted/docs/approvals-surfaces.md): approval surfaces and their threat model.
