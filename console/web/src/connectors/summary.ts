import type { Access, Connector } from "../api";

/** How many of a connector's functions sit at each access. */
export const accessCounts = (connector: Connector): Record<Access, number> => {
  const counts: Record<Access, number> = { off: 0, read: 0, write: 0, ask: 0 };
  for (const fn of connector.functions ?? []) counts[fn.access] += 1;
  return counts;
};

/** What a connector still needs to be called: nothing declared, a stored credential, an API key or header, or a sign-in. */
export type AuthState = "none" | "set" | "key" | "signIn";

/** Reads the auth state from what the listing declares and what is stored. */
export const authState = (connector: Connector): AuthState => {
  const declared = connector.auth ?? [];
  if (declared.length === 0) return "none";
  if ((connector.credentials ?? []).length > 0) return "set";
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
