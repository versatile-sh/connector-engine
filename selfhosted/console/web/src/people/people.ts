import type { PendingLink, Person, Role } from "../api";
import type { Viewer } from "./viewer";

/** The roles from the most to the least able, as the invite and the role choice list them. */
export const ROLES: readonly Role[] = ["owner", "admin", "member"];

/** What the viewer may do to one person's row: give them another role, make them a reset link, or remove them. */
export type RowAction = { kind: "role"; role: Role } | { kind: "reset" } | { kind: "remove" };

/** Whether the viewer may make a reset link for `person`: never for themselves; an Owner for anyone else, an Admin only for a Member. The server checks the same ladder; this only decides what the row offers. */
export function mayReset(viewer: Viewer, person: Person): boolean {
  if (person.you || !viewer.can("people.reset")) return false;
  return viewer.role === "owner" || person.role === "member";
}

/** What one row offers, in the order the overflow menu lists it: the other roles and Remove to a viewer who manages people (Remove never on their own row), and Reset password where the ladder allows. */
export function rowActionsOf(viewer: Viewer, person: Person): RowAction[] {
  const manage = viewer.can("people.manage");
  const roles: RowAction[] = manage ? ROLES.filter((role) => role !== person.role).map((role) => ({ kind: "role", role })) : [];
  const reset: RowAction[] = mayReset(viewer, person) ? [{ kind: "reset" }] : [];
  const remove: RowAction[] = manage && !person.you ? [{ kind: "remove" }] : [];
  return [...roles, ...reset, ...remove];
}

/** A row action as the value of an overflow `Select`. */
export const actionValue = (action: RowAction) => (action.kind === "role" ? `role:${action.role}` : action.kind);

/** Whether the viewer may revoke a waiting link: any with `people.manage`, and the reset links they made themselves. */
export const mayRevoke = (viewer: Viewer, link: PendingLink) => viewer.can("people.manage") || (link.purpose === "reset" && link.created_by === viewer.person);

const LOOPBACK = /^(localhost|127(\.\d{1,3}){3}|\[::1\])$/i;

/** Whether a link points at this machine only, so nobody else can open it. */
export function isLoopbackLink(link: string): boolean {
  try {
    return LOOPBACK.test(new URL(link).hostname);
  } catch {
    return false;
  }
}

/** How long until `at` from `now`, rounded up, in days from a day up, else in hours, at least one. */
export function timeLeft(at: string, now: number): { unit: "day" | "hour"; count: number } {
  const hours = Math.max(1, Math.ceil((Date.parse(at) - now) / 3_600_000));
  return hours >= 24 ? { unit: "day", count: Math.ceil(hours / 24) } : { unit: "hour", count: hours };
}

/** The name of a person id among the people, or the id itself when that person is gone. */
export const nameOf = (people: readonly Person[], person: string) => people.find((p) => p.person === person)?.username ?? person;
