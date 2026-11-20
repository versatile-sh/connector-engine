import type { Access, Connector } from "../api";

/** How many of a connector's functions sit at each access. */
export const accessCounts = (connector: Connector): Record<Access, number> => {
  const counts: Record<Access, number> = { off: 0, on: 0, ask: 0 };
  for (const fn of connector.functions ?? []) counts[fn.access] += 1;
  return counts;
};

/** Whether a stored credential, written `kind:status`, is the one that works. */
export const isActive = (credential: string) => credential.endsWith(":active");

/** What a connector still needs to be called: nothing declared, nothing more (an active credential), an API key or header, or a sign-in. */
export type AuthState = "none" | "set" | "key" | "signIn";

/** Reads the auth state from what the listing declares and what is stored; a credential that needs re-auth, failed or was revoked counts as missing. */
export const authState = (connector: Connector): AuthState => {
  const declared = connector.auth ?? [];
  if (declared.length === 0) return "none";
  if ((connector.credentials ?? []).some(isActive)) return "set";
  return declared.includes("oauth2_user") ? "signIn" : "key";
};

/** The host a connector's server answers on, when it has a remote one. */
export const serverHost = (connector: Connector) => {
  try {
    return connector.remote_url ? new URL(connector.remote_url).host : undefined;
  } catch {
    return undefined;
  }
};
