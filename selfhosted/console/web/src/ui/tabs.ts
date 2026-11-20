/** How many tabs a row holds before it also offers a `Select` of all of them. */
export const TABS_OVERFLOW_FROM = 6;

const STEPS: Record<string, (at: number, count: number) => number> = {
  ArrowRight: (at, count) => (at + 1) % count,
  ArrowLeft: (at, count) => (at - 1 + count) % count,
  Home: () => 0,
  End: (_at, count) => count - 1,
};

/** The tab a key moves to from tab `at` of `count`: the arrows wrap around, Home and End go to the ends; other keys move nowhere. */
export function stepOf(key: string, at: number, count: number): number | undefined {
  const step = STEPS[key];
  return step && count > 0 ? step(Math.max(at, 0), count) : undefined;
}

/** A tab in its scrolling row and the visible part of that row, in pixels. */
type Span = { left: number; width: number };

/** Where a row of tabs scrolls to so the tab `tab` shows whole with `room` to spare beside it, or nothing when it already shows. */
export function revealOf(tab: Span, row: Span, room: number): number | undefined {
  if (tab.left - room < row.left) return Math.max(0, tab.left - room);
  if (tab.left + tab.width + room > row.left + row.width) return tab.left + tab.width + room - row.width;
  return undefined;
}

/** Which ends of a scrolling row hide tabs, so they fade: the start once it scrolled, the end while more lies past it. */
export function edgesOf(scrollLeft: number, scrollWidth: number, clientWidth: number): { start: boolean; end: boolean } {
  return { start: scrollLeft > 1, end: scrollLeft + clientWidth < scrollWidth - 1 };
}
