import type { Capability, Role, Session } from "../api";

/** Who looks at the console: their person id and role, whether the install has more than one user, and what the server says they may do. */
export type Viewer = { person: string | undefined; role: Role | undefined; team: boolean; can(capability: Capability): boolean };

/** Whose rows a list shows: the viewer's own, or everyone's. */
export type Scope = "mine" | "everyone";

/** The viewer a session describes; a session that names no capability may do nothing, and one with no count of people is a one-user install. */
export function viewerOf(session: Session | undefined): Viewer {
  const can = new Set(session?.can ?? []);
  return { person: session?.user?.person, role: session?.user?.role, team: (session?.people ?? 1) > 1, can: (capability) => can.has(capability) };
}

/** Whether a list offers Mine and Everyone: only once a second user exists, and only to a viewer who may see everyone's rows. */
export const offersScope = (viewer: Viewer, any: Capability) => viewer.team && viewer.can(any);

/** The scope a page shows: everyone when the address asks for it and the viewer is offered it, else the viewer's own rows. */
export const scopeOf = (query: URLSearchParams, offered: boolean): Scope => (offered && query.get("scope") === "everyone" ? "everyone" : "mine");

/** The page's address with `scope` set, the default left out, keeping the rest of the query. */
export function scopedHref(path: string, query: URLSearchParams, scope: Scope): string {
  const next = new URLSearchParams(query);
  if (scope === "everyone") next.set("scope", "everyone");
  else next.delete("scope");
  const text = next.toString();
  return text ? `${path}?${text}` : path;
}

/** The role the shell shows beside Account, only once a second user exists. */
export const roleBadgeOf = (viewer: Viewer): Role | undefined => (viewer.team ? viewer.role : undefined);
