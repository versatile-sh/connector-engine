import type { MessageKey } from "../i18n";
import type { IconName } from "../ui/Icon";

/** The console's sections and the paths each one owns. */
export type Section = "dashboard" | "connectors" | "skills" | "access" | "approvals" | "activity" | "notifications" | "account";

/** One link of the sidebar: the section it opens, where, what it is called and its icon. */
export type NavItem = { section: Section; href: string; label: MessageKey; icon: IconName };

/** A titled run of sidebar links. */
export type NavGroup = { id: "gateway" | "settings"; label: MessageKey; items: readonly NavItem[] };

/** The sidebar's links, in the order they are drawn: what the gateway does, then how it is set up. */
export const NAV_GROUPS: readonly NavGroup[] = [
  {
    id: "gateway",
    label: "nav.group.gateway",
    items: [
      { section: "dashboard", href: "/", label: "nav.dashboard", icon: "dashboard" },
      { section: "connectors", href: "/connectors", label: "nav.connectors", icon: "plug" },
      { section: "skills", href: "/skills", label: "nav.skills", icon: "spark" },
      { section: "access", href: "/access", label: "nav.access", icon: "key" },
      { section: "approvals", href: "/approvals", label: "nav.approvals", icon: "shield" },
      { section: "activity", href: "/activity", label: "nav.activity", icon: "activity" },
    ],
  },
  {
    id: "settings",
    label: "nav.group.settings",
    items: [
      { section: "notifications", href: "/notifications", label: "nav.notifications", icon: "bell" },
      { section: "account", href: "/account", label: "nav.account", icon: "user" },
    ],
  },
];

/** A link as the sidebar draws it: whether it is the page the viewer is on, and how many calls it has waiting, when any. */
export type NavEntry = NavItem & { current: boolean; waiting: number };

/** The groups with their links marked: the one of `section` is current, and Approvals carries the count of held calls. */
export function navView(section: Section, waiting: number): readonly (Omit<NavGroup, "items"> & { items: readonly NavEntry[] })[] {
  return NAV_GROUPS.map((group) => ({
    ...group,
    items: group.items.map((item) => ({ ...item, current: item.section === section, waiting: item.section === "approvals" ? Math.max(waiting, 0) : 0 })),
  }));
}

const SECTION_OF: Readonly<Record<string, Section>> = {
  "/": "dashboard",
  "/connectors": "connectors",
  "/import": "connectors",
  "/skills": "skills",
  "/access": "access",
  "/agents": "access",
  "/approvals": "approvals",
  "/activity": "activity",
  "/notifications": "notifications",
  "/account": "account",
};

/** The section a path belongs to: `/` is the dashboard, `/connectors` and every connector under it (and `/import`, which opens the import window there) are the connectors, Account owns its tabs, and a path the console does not know opens the dashboard. */
export function sectionOf(path: string): Section {
  const exact = SECTION_OF[path];
  if (exact) return exact;
  if (path.startsWith("/connectors/")) return "connectors";
  if (path.startsWith("/account/")) return "account";
  return "dashboard";
}

/** The group and the link names of a section, for the line over the page. */
export function crumbOf(section: Section): { group: MessageKey; label: MessageKey } {
  for (const group of NAV_GROUPS) {
    const item = group.items.find((entry) => entry.section === section);
    if (item) return { group: group.label, label: item.label };
  }
  return { group: NAV_GROUPS[0]?.label ?? "nav.group.gateway", label: "nav.dashboard" };
}

/** Where the sidebar's width is kept between visits. */
export const COLLAPSED_KEY = "console.sidebar";

/** Whether a saved value means the rail: only the exact word, so any other value a store holds reads as open. */
export const parseCollapsed = (saved: string | null | undefined): boolean => saved === "collapsed";

/** The value kept for a sidebar that is a rail or open. */
export const serializeCollapsed = (collapsed: boolean): string => (collapsed ? "collapsed" : "expanded");

/** The part of a browser store the sidebar reads and writes. */
export type Store = Pick<Storage, "getItem" | "setItem">;

/** The saved width of the sidebar; a store that is missing or refuses reads as open. */
export function loadCollapsed(store: Store | undefined): boolean {
  try {
    return parseCollapsed(store?.getItem(COLLAPSED_KEY));
  } catch {
    return false;
  }
}

/** Keeps the sidebar's width; a store that refuses is left alone. */
export function saveCollapsed(store: Store | undefined, collapsed: boolean): void {
  try {
    store?.setItem(COLLAPSED_KEY, serializeCollapsed(collapsed));
  } catch {
    return;
  }
}

/** The shell's own state: whether the desktop sidebar is a rail, and the address the drawer was opened on, if it is open. */
export type ShellState = { collapsed: boolean; drawerAt: string | undefined };

/** What happens to the shell: the rail is chosen, the drawer opens on an address or closes, or the screen grows wide enough to need no drawer. */
export type ShellAction = { type: "rail"; collapsed: boolean } | { type: "open"; at: string } | { type: "close" } | { type: "widen" };

/** The shell at the start, with the saved width and the drawer shut. */
export const shellOf = (collapsed: boolean): ShellState => ({ collapsed, drawerAt: undefined });

/** The shell after an action; a wide screen shuts the drawer so it does not wait open for the next narrow one. */
export function shellReducer(state: ShellState, action: ShellAction): ShellState {
  switch (action.type) {
    case "rail":
      return { ...state, collapsed: action.collapsed };
    case "open":
      return { ...state, drawerAt: action.at };
    case "close":
    case "widen":
      return state.drawerAt === undefined ? state : { ...state, drawerAt: undefined };
  }
}

/** The state once the page is at `at`: a drawer opened on another address is shut for good, so coming back to its address does not open it again. */
export const shellAt = (state: ShellState, at: string): ShellState => (state.drawerAt === undefined || state.drawerAt === at ? state : shellReducer(state, { type: "close" }));

/** Whether the sidebar is drawn as the icon rail: only on a wide screen, and only when the viewer chose it. */
export const railOf = (state: ShellState, wide: boolean): boolean => wide && state.collapsed;

/** Whether the drawer is open as a modal: only on a narrow screen, and only on the address it was opened on, so moving to another page shuts it. */
export const drawerOpen = (state: ShellState, wide: boolean, at: string): boolean => !wide && state.drawerAt === at;

/** The screen width, in the CSS of `lg`, from which the sidebar stays beside the page. */
export const WIDE = "(min-width: 64rem)";

/** Where Tab goes from the `index`th of `count` controls of the drawer: round to the other end from the last one going on and the first one going back, `undefined` anywhere else, where the browser moves on its own. */
export function wrapFocus(count: number, index: number, backwards: boolean): number | undefined {
  if (count === 0) return undefined;
  if (backwards) return index <= 0 ? count - 1 : undefined;
  return index === count - 1 ? 0 : undefined;
}

/** Whether a key press shuts the open drawer: Escape does, unless an open menu inside it is the one that answers it. */
export const closesDrawer = (key: string, open: boolean, inMenu: boolean): boolean => key === "Escape" && open && !inMenu;

/** The first letter of a name, in capitals, for the round mark of the signed-in user. */
export const initialOf = (name: string): string => (name.trim().match(/\p{L}|\p{N}/u)?.[0] ?? "?").toLocaleUpperCase();
