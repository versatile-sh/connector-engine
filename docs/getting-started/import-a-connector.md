![Import a connector](../assets/section-import-a-connector.svg)

# Import a connector

A connector is a tool server (MCP, REST or gRPC) the engine calls on behalf of your AI apps. You import it once; every access key then gets the functions you allow.

## From a URL

1. Open **Connectors** in the console.
2. Paste the `https` URL of an MCP server.
3. Review the functions and the proposed choice for each: off, on or ask.
4. Confirm. Sign in to the provider if it asks.

Nothing is installed before you confirm.

### A server that asks you to sign in

An MCP server that answers `401` lists its functions only to a signed-in client. The console still imports it: the connector starts with no functions and the sign-in method OAuth, and the functions appear, each set to **ask**, once somebody signs in from the connector page.

The sign-in needs an OAuth client for the server. The console finds one in this order:

1. A client you registered for that server's issuer (a client id and secret typed on the connector page, or set by an admin).
2. The console's client metadata document (CIMD), when the server supports it and the console has a public `https` address. The console serves the document at `/oauth/client.json`.
3. Dynamic client registration (RFC 7591), which registers the console's redirect address and keeps the client, encrypted, for the next sign-in.

When the server allows neither, the connector page says so and asks for a client id and secret. Some providers allow only listed clients; register the console's redirect address (`<console address>/oauth/callback`) with them first.

The console reads the server's discovery documents for this: protected resource metadata (RFC 9728) from the `WWW-Authenticate` header of the `401` or the `/.well-known/oauth-protected-resource` addresses, then authorization server metadata (RFC 8414, OpenID Connect). Every fetch goes through the egress guard, follows no redirect, reads at most 1 MiB and takes `https` addresses only, or `http` on loopback. Tokens are requested for the server's own address (RFC 8707 `resource`) with PKCE `S256`.

## From an install link

A link opens the same review, after sign-in. Directories such as the versatile.sh connector directory use them.

| Link | Opens |
|---|---|
| `http://127.0.0.1:7465/import?mcp=<https MCP URL>` | a review of an MCP server |
| `http://127.0.0.1:7465/import?listing=<https URL of a listing>` | a review of a listing in the Manifest format |
| `http://127.0.0.1:7465/import?listing=<https URL of a skill catalog entry>` | a review of a skill (`kind: skill`) |

A listing link may add `&page=<URL of the entry's page in its directory>`. The line naming where the link came from then opens that page instead of the raw listing. The engine keeps the page only when it is `https` (or `http` on this computer) on the same host as the listing, and ignores it otherwise.

The page shows the origin, title, icon, publisher and version (a skill's licence and short commit), the review, who may load it, the notice that nothing is installed yet, and the **Add** button. On a wide screen the page is two columns: the title, icon and facts on the left, and on the right one card that holds the review, who may load it and the notice, with **Add** at its foot. The card is as tall as its content and never scrolls inside itself; a connector with a very long list of functions makes the page scroll and keeps **Add** in view at the bottom of the window. On a narrow screen the page is one column and the **Add** button is in a bar at the bottom of the page.

## What the engine does on import

- It captures every function and pins it by an RFC 8785 hash.
- A function that changes later, or a new one, is **quarantined** until you accept it. See [connectors](../using/connectors.md#pinning-and-quarantine).
- Upstream connections refuse private addresses unless the profile allows them. The `person` profile does, so a server on `localhost` works: the engine dials `host.docker.internal` in its place.

## From a backup or a shared setup

A setup file from a friend or a teammate adds its connectors and skills through this same import. See [export and import](../using/account-backup.md#export-and-import).

Next: [functions and permissions](../using/permissions.md).
