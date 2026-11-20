/** What decides whether the first-run guide shows: its data loaded, the owner's dismissal kept by the server, and how many steps are done. */
export type GuideFacts = { ready: boolean; dismissed: boolean; done: number; total: number };

/** The guide shows only while it has something left to do: loaded, never dismissed, and a step still open; finishing every step hides it by itself. */
export const guideVisible = ({ ready, dismissed, done, total }: GuideFacts) => ready && !dismissed && done < total;
