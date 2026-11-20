/** How many rows a list shows before it offers a filter and Show all. */
export const LONG_LIST_FROM = 6;

/** What a long list shows: the rows whose text holds the filter, every row once asked, else the first few, and how many stay hidden. */
export function shownOf<T>(rows: readonly T[], textOf: (row: T) => string, { filter, all }: { filter: string; all: boolean }): { shown: readonly T[]; hidden: number } {
  const needle = filter.trim().toLowerCase();
  if (needle) return { shown: rows.filter((row) => textOf(row).toLowerCase().includes(needle)), hidden: 0 };
  if (all || rows.length <= LONG_LIST_FROM) return { shown: rows, hidden: 0 };
  return { shown: rows.slice(0, LONG_LIST_FROM), hidden: rows.length - LONG_LIST_FROM };
}
