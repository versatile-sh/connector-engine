import type { ApprovalChoice, CredentialMode, RowOwner } from "../api";
import { DEFAULT_CHOICES, offeredChoices } from "../approve/choices";
import type { Viewer } from "../people/viewer";

/** How a held call's row reads for the viewer: whose call it waits on when it is someone else's, the answers the server offers, whether it may be cancelled, and the person who decides alone because the call uses their own sign-in. */
export type HeldView = { waitingFor: string | undefined; decide: ApprovalChoice[]; cancel: boolean; decidesAlone: string | undefined };

/** What a held call says about who it waits on and what may answer it. */
export type Held = { owner?: RowOwner | undefined; choices?: readonly string[] | undefined };

/** The view of one held call: the server's `choices` decide the buttons, and `cancel` among them offers Cancel this call; without `choices` the viewer's own call keeps today's answers and someone else's offers none. A call on a connector where each person signs in names its owner as the one who decides. */
export function heldViewOf(approval: Held, viewer: Viewer, mode: CredentialMode | undefined): HeldView {
  const owner = approval.owner;
  const other = owner && viewer.person !== undefined && owner.person !== viewer.person ? owner.username || owner.person : undefined;
  const offered: readonly string[] = approval.choices ?? (other ? [] : DEFAULT_CHOICES);
  return { waitingFor: other, decide: offeredChoices(offered), cancel: offered.includes("cancel"), decidesAlone: other && mode === "per_person" ? other : undefined };
}
