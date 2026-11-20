import type { Access, Connector } from "../api";

/** How many of a connector's functions, or of a skill's keys, sit at each access. */
export const accessCounts = (choices: readonly { access: Access }[]): Record<Access, number> => {
  const counts: Record<Access, number> = { off: 0, on: 0, ask: 0 };
  for (const choice of choices) counts[choice.access] += 1;
  return counts;
};

/** The access every choice shares, or nothing when they differ or there is none. */
export const commonAccess = (choices: readonly { access: Access }[]): Access | undefined => {
  const first = choices[0]?.access;
  return first && choices.every((choice) => choice.access === first) ? first : undefined;
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
