import type { Person, Role } from "../api";
import type { Viewer } from "./viewer";

/** The roles from the most to the least able, as the invite and the role choice list them. */
export const ROLES: readonly Role[] = ["owner", "admin", "member"];

/** What the viewer may do to one person's row: give them another role, make them a reset link, or remove them. */
export type RowAction = { kind: "role"; role: Role } | { kind: "reset" } | { kind: "remove" };

/** What one row offers, in the order the overflow menu lists it: the other roles and Remove to a viewer who manages people (Remove never on their own row), and Reset password where the server says the viewer may. */
export function rowActionsOf(viewer: Viewer, person: Person): RowAction[] {
  const manage = viewer.can("people.manage");
  const roles: RowAction[] = manage ? ROLES.filter((role) => role !== person.role).map((role) => ({ kind: "role", role })) : [];
  const reset: RowAction[] = person.may_reset ? [{ kind: "reset" }] : [];
  const remove: RowAction[] = manage && !person.you ? [{ kind: "remove" }] : [];
  return [...roles, ...reset, ...remove];
}

/** A row action as the value of an overflow `Select`. */
export const actionValue = (action: RowAction) => (action.kind === "role" ? `role:${action.role}` : action.kind);

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
