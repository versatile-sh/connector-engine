import { errorCode, errorStatus, type LinkFacts } from "../api";

/** The token a join or reset link carries in its fragment, which never reaches a server log; nothing when the fragment is empty. */
export const tokenOf = (hash: string): string | undefined => hash.replace(/^#/, "").trim() || undefined;

/** Whether a refusal says the link no longer works: used, revoked, expired or never real. */
export const isGone = (error: unknown) => errorCode(error) === "link_gone" || errorStatus(error) === 410;

/** What the /invite page shows: the link being read, a link that no longer works, a refusal to show with a retry, or the form to join or to set a new password. */
export type LinkView = { kind: "reading" } | { kind: "gone" } | { kind: "failed"; error: unknown } | { kind: "join" | "reset"; facts: LinkFacts };

/** The view of a link from its token and what reading it answered; a missing token is a dead link. */
export function linkViewOf(token: string | undefined, facts: LinkFacts | undefined, error: unknown): LinkView {
  if (!token) return { kind: "gone" };
  if (error) return isGone(error) ? { kind: "gone" } : { kind: "failed", error };
  if (!facts) return { kind: "reading" };
  return { kind: facts.purpose === "reset" ? "reset" : "join", facts };
}
