import type { Access, Profile, ProfileMode } from "../api";

const RANK: Record<Access, number> = { off: 0, ask: 1, on: 2 };

/** Whether `access` gives more than `most`, so a key held to `most` cannot take it. */
export const wider = (access: Access, most: Access) => RANK[access] > RANK[most];

/** What one key gets for a function or item whose default is `base`, and whether the key narrows it there; a narrowing the default has since passed gives the default. */
export const profileAccess = (profile: Profile, name: string, base: Access): { access: Access; overridden: boolean } => {
  if (profile.mode === "off") return { access: "off", overridden: false };
  const own = profile.mode === "custom" ? profile.overrides[name] : undefined;
  if (own === undefined) return { access: base, overridden: false };
  return { access: wider(own, base) ? base : own, overridden: true };
};

/** How many functions and items a profile narrows. */
export const overrideCount = (profile: Profile) => (profile.mode === "custom" ? Object.keys(profile.overrides).length : 0);

/** How many of these keys would lose overrides if set to off or the default. */
export const keysWithOverrides = (profiles: readonly Profile[]) => profiles.filter((profile) => overrideCount(profile) > 0).length;

/** The mode every key shares when it is off or follow, or nothing when they differ, one is custom, or there is none. */
export const commonMode = (profiles: readonly Profile[]): Exclude<ProfileMode, "custom"> | undefined => {
  const first = profiles[0]?.mode;
  return first && first !== "custom" && profiles.every((profile) => profile.mode === first) ? first : undefined;
};

/** The address of a connector's page, showing one key's profile when one is named. */
export const profileHref = (connector: string, agent?: string) => `/connectors/${encodeURIComponent(connector)}${agent ? `?profile=${encodeURIComponent(agent)}` : ""}`;
