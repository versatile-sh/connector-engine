import type { ActivitySummary, Agent, Connector, HourCount } from "../api";

/** The ranges the dashboard's chart toggles between. */
export type ChartRange = "24h" | "7d";

/** The chart's ranges in the order the toggle lists them. */
export const CHART_RANGES: readonly ChartRange[] = ["24h", "7d"];

/** How a count compares with the one before it: nothing in either, new where there was none, equal, or up or down by a whole percent. */
export type Delta = { kind: "none" | "new" | "same" } | { kind: "up" | "down"; percent: number };

/** Compares the last day's calls with the day before's, rounding the change to a whole percent and calling a change that rounds to nothing the same. */
export function deltaOf(current: number, previous: number): Delta {
  if (current === 0 && previous === 0) return { kind: "none" };
  if (previous === 0) return { kind: "new" };
  const percent = Math.round((Math.abs(current - previous) / previous) * 100);
  if (percent === 0) return { kind: "same" };
  return { kind: current > previous ? "up" : "down", percent };
}

/** Whether the viewer's calls of the last seven days number none, so the dashboard shows its empty state and not a wall of zeros. */
export const isQuiet = (summary: ActivitySummary): boolean => summary.total === 0;

/** The hourly counts a chart range draws: the last 24 hours as the console counted them for the day's total, so the chart and the stat card agree, or the seven days whole, which the chart then groups by local day. */
export const hoursOf = (summary: ActivitySummary, range: ChartRange): HourCount[] => (range === "24h" ? summary.day_hours : summary.hours);

/** Whole percent of the busiest row that `total` is, for the hairline under a ranked row; a row with calls always shows at least a sliver. */
export const shareOf = (total: number, busiest: number): number => (total <= 0 || busiest <= 0 ? 0 : Math.max(4, Math.round((total / busiest) * 100)));

/** How many of the installed connectors are on and not blocked. */
export const activeConnectors = (connectors: readonly Connector[]): number => connectors.filter((connector) => connector.enabled && !connector.blocked).length;

/** How many access keys have not expired at `now`. */
export const liveKeys = (keys: readonly Agent[], now: number): number => keys.filter((key) => Date.parse(key.expires_at) > now).length;

/** The whole percent of the last day's calls that failed or were denied, or nothing when there were no calls. */
export const failedShare = (day: { total: number; failed: number }): number | undefined => (day.total === 0 ? undefined : Math.round((day.failed / day.total) * 100));
