import type { Approval, ApprovalChoice, CredentialMode } from "../api";
import { DEFAULT_CHOICES, offeredChoices } from "../approve/choices";
import type { Viewer } from "../people/viewer";

/** How a held call's row reads for the viewer: whose call it waits on when it is someone else's, the answers the server offers, whether it may be cancelled, and the person who decides alone because the call uses their own sign-in. */
export type HeldView = { waitingFor: string | undefined; decide: ApprovalChoice[]; cancel: boolean; decidesAlone: string | undefined };

/** The view of one held call: the server's `choices` decide the buttons (today's list when it names none), `cancel` among them offers Cancel this call, and a call on a connector where each person signs in names its owner as the one who decides. */
export function heldViewOf(approval: Approval, viewer: Viewer, mode: CredentialMode | undefined): HeldView {
  const owner = approval.owner;
  const other = owner && viewer.person !== undefined && owner.person !== viewer.person ? owner.username || owner.person : undefined;
  const offered: readonly string[] = approval.choices ?? DEFAULT_CHOICES;
  return { waitingFor: other, decide: offeredChoices(offered), cancel: offered.includes("cancel"), decidesAlone: other && mode === "per_person" ? other : undefined };
}
