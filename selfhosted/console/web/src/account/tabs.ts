import type { Capability } from "../api";
import type { Viewer } from "../people/viewer";

/** A view of the Account page: how the user signs in, the people of the console, and the backups. */
export type AccountTab = "profile" | "people" | "backups";

const TABS: readonly { tab: AccountTab; path: string; needs?: readonly Capability[] }[] = [
  { tab: "profile", path: "/account" },
  { tab: "people", path: "/account/people", needs: ["people.read"] },
  { tab: "backups", path: "/account/backups", needs: ["backup.full", "setup.manage"] },
];

/** The tabs the viewer may open, in order: a tab opens with any of the capabilities it needs; Profile is always one. */
export const accountTabsOf = (viewer: Viewer): AccountTab[] => TABS.filter((entry) => !entry.needs || entry.needs.some((need) => viewer.can(need))).map((entry) => entry.tab);

/** The address of a tab. */
export const accountPathOf = (tab: AccountTab) => TABS.find((entry) => entry.tab === tab)?.path ?? "/account";

/** The tab an address shows among those the viewer may open, Profile for any other. */
export const accountTabOf = (path: string, tabs: readonly AccountTab[]): AccountTab => tabs.find((tab) => accountPathOf(tab) === path) ?? "profile";
