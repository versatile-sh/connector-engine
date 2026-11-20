import type { CallOutcome, HourCount, Kind } from "../api";
import type { Locale, MessageKey } from "../i18n";

/** How far back the activity page looks. */
export type ActivityRange = "24h" | "7d" | "30d";

/** What the activity page shows, as its URL query holds it. */
export type ActivityFilter = { range: ActivityRange; connector?: string; agent?: string; outcome?: CallOutcome };

/** The ranges the page offers, with their length in hours. */
export const RANGES: readonly { value: ActivityRange; hours: number }[] = [
  { value: "24h", hours: 24 },
  { value: "7d", hours: 24 * 7 },
  { value: "30d", hours: 24 * 30 },
];

/** The outcomes a call can come to, in the order the page lists them. */
export const OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending", "running"];

const isRange = (value: string | null): value is ActivityRange => RANGES.some((range) => range.value === value);

const isOutcome = (value: string | null): value is CallOutcome => OUTCOMES.some((outcome) => outcome === value);

/** Reads the filter from the page's query, dropping what is not a known range or outcome. */
export function filterOf(query: URLSearchParams): ActivityFilter {
  const range = query.get("range");
  const connector = query.get("connector");
  const agent = query.get("agent");
  const outcome = query.get("outcome");
  const filter: ActivityFilter = { range: isRange(range) ? range : "24h" };
  if (connector) filter.connector = connector;
  if (agent) filter.agent = agent;
  if (isOutcome(outcome)) filter.outcome = outcome;
  return filter;
}

/** The page's own address for a filter; the default range is left out. */
export function hrefOf(filter: ActivityFilter): string {
  const query = new URLSearchParams();
  if (filter.range !== "24h") query.set("range", filter.range);
  if (filter.connector) query.set("connector", filter.connector);
  if (filter.agent) query.set("agent", filter.agent);
  if (filter.outcome) query.set("outcome", filter.outcome);
  const text = query.toString();
  return text ? `/activity?${text}` : "/activity";
}

/** The API query for a filter, its time range ending now. */
export function apiQueryOf(filter: ActivityFilter, now: Date): string {
  const hours = RANGES.find((range) => range.value === filter.range)?.hours ?? 24;
  const query = new URLSearchParams({ from: new Date(now.getTime() - hours * 3_600_000).toISOString(), to: now.toISOString() });
  if (filter.connector) query.set("connector", filter.connector);
  if (filter.agent) query.set("agent", filter.agent);
  if (filter.outcome) query.set("outcome", filter.outcome);
  return query.toString();
}

/** One bar of the chart: its start, whether it is an hour or a local day, and its calls by outcome. */
export type Bar = { start: string; unit: "hour" | "day"; ok: number; failed: number; denied: number; waiting: number; total: number };

const localDay = (iso: string) => {
  const at = new Date(iso);
  return `${at.getFullYear()}-${at.getMonth() + 1}-${at.getDate()}`;
};

/** Turns the API's hourly counts into bars: one per hour over a day, one per local day over longer ranges. */
export function barsOf(hours: readonly HourCount[], range: ActivityRange): Bar[] {
  const unit = range === "24h" ? "hour" : "day";
  const bars: Bar[] = [];
  let key = "";
  for (const hour of hours) {
    const next = unit === "hour" ? hour.start : localDay(hour.start);
    if (next !== key || bars.length === 0) {
      bars.push({ start: hour.start, unit, ok: 0, failed: 0, denied: 0, waiting: 0, total: 0 });
      key = next;
    }
    const bar = bars[bars.length - 1];
    if (!bar) continue;
    bar.ok += hour.ok;
    bar.failed += hour.error;
    bar.denied += hour.denied;
    bar.waiting += hour.pending + hour.running;
    bar.total += hour.ok + hour.error + hour.denied + hour.pending + hour.running;
  }
  return bars;
}

/** A duration in milliseconds as people read it. */
export function durationText(ms: number | undefined): string {
  if (ms === undefined) return "—";
  if (ms < 1_000) return `${ms} ms`;
  if (ms < 60_000) return `${(ms / 1_000).toFixed(ms < 10_000 ? 1 : 0)} s`;
  return `${Math.round(ms / 60_000)} min`;
}

/** Captured text as indented JSON when it parses, else as it is; a cut excerpt stays as it is. */
export function prettyJSON(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

/** The class the engine recorded, when it is one the console has a label for. */
export const kindOf = (value: string | undefined): Kind | undefined => (value === "read" || value === "write" || value === "destructive" ? value : undefined);

const APPROVAL_STATES: Record<string, MessageKey> = {
  pending: "approvalState.pending",
  approved: "approvalState.approved",
  executing: "approvalState.executing",
  executed: "approvalState.executed",
  failed: "approvalState.failed",
  denied: "approvalState.denied",
  expired: "approvalState.expired",
  cancelled: "approvalState.cancelled",
};

/** The message that names an approval's state, when the console knows the state. */
export const approvalStateKey = (state: string | undefined): MessageKey | undefined => (state ? APPROVAL_STATES[state] : undefined);

/** A call's time: the clock when it is today, the date and the clock before. */
export function timeOf(iso: string, locale: Locale, now = new Date()): string {
  const at = new Date(iso);
  const today = at.toDateString() === now.toDateString();
  return new Intl.DateTimeFormat(locale, today ? { hour: "2-digit", minute: "2-digit", second: "2-digit" } : { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }).format(at);
}
