/** Where the inline revoke of one access key stands: nothing asked (focus back on the key it left), the question shown, or the key going away. */
export type RevokeState = { kind: "idle"; returnTo?: string } | { kind: "confirming"; name: string } | { kind: "revoking"; name: string };

/** What moves the revoke flow: the Revoke button, Cancel, the confirmation, and the server's answer. */
export type RevokeEvent = { type: "ask"; name: string } | { type: "cancel" } | { type: "confirm" } | { type: "revoked" } | { type: "failed" };

/** The next step of the revoke flow; one key at a time, and nothing interrupts a revoke on its way. */
export function revokeStep(state: RevokeState, event: RevokeEvent): RevokeState {
  switch (event.type) {
    case "ask":
      return state.kind === "revoking" ? state : { kind: "confirming", name: event.name };
    case "cancel":
      return state.kind === "confirming" ? { kind: "idle", returnTo: state.name } : state;
    case "confirm":
      return state.kind === "confirming" ? { kind: "revoking", name: state.name } : state;
    case "revoked":
      return state.kind === "revoking" ? { kind: "idle" } : state;
    case "failed":
      return state.kind === "revoking" ? { kind: "confirming", name: state.name } : state;
  }
}
