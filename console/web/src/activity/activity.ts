import type { CallOutcome, Connector, HourCount, Kind, Skill } from "../api";
import type { Locale, MessageKey } from "../i18n";

/** A range that ends now. */
export type PresetRange = "24h" | "7d" | "30d";

/** How far back the activity page looks: a preset that ends now, or local days from and to, both included, as `YYYY-MM-DD`. */
export type ActivityPeriod = { range: PresetRange } | { range: "custom"; from: string; to: string };

/** What narrows the page within its period: a connector, an app, an outcome and a text in the function name. */
export type Narrowing = { connector?: string; agent?: string; outcome?: CallOutcome; search?: string };

/** What the activity page shows, as its URL query holds it. */
export type ActivityFilter = ActivityPeriod & Narrowing;

/** The preset ranges the page offers, with their length in hours. */
export const RANGES: readonly { value: PresetRange; hours: number }[] = [
  { value: "24h", hours: 24 },
  { value: "7d", hours: 24 * 7 },
  { value: "30d", hours: 24 * 30 },
];

/** The outcomes a call can come to, in the order the page lists them. */
export const OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending", "running"];

const HOUR = 3_600_000;

const isRange = (value: string | null): value is PresetRange => RANGES.some((range) => range.value === value);

const isOutcome = (value: string | null): value is CallOutcome => OUTCOMES.some((outcome) => outcome === value);

const DAY = /^(\d{4})-(\d{2})-(\d{2})$/;

const startOf = (day: string): Date | undefined => {
  const match = DAY.exec(day);
  if (!match) return undefined;
  const at = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return dayOf(at) === day ? at : undefined;
};

const setNarrowing = (query: URLSearchParams, narrowing: Narrowing) => {
  for (const name of ["connector", "agent", "outcome", "search"] as const) {
    const value = narrowing[name];
    if (value) query.set(name, value);
  }
};

/** A local day as `YYYY-MM-DD`. */
export const dayOf = (at: Date) => `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, "0")}-${String(at.getDate()).padStart(2, "0")}`;

/** The custom period the page opens with: the last seven local days, today included. */
export const lastWeek = (now: Date): ActivityPeriod => ({ range: "custom", from: dayOf(new Date(now.getFullYear(), now.getMonth(), now.getDate() - 6)), to: dayOf(now) });

/** Reads the filter from the page's query: two real days make a custom period, earliest first; otherwise an unknown range falls back to a day, and an unknown outcome is dropped. */
export function filterOf(query: URLSearchParams): ActivityFilter {
  const range = query.get("range");
  const from = query.get("from") ?? "";
  const to = query.get("to") ?? "";
  const filter: ActivityFilter = startOf(from) && startOf(to) ? { range: "custom", from: from < to ? from : to, to: from < to ? to : from } : { range: isRange(range) ? range : "24h" };
  for (const name of ["connector", "agent", "search"] as const) {
    const value = query.get(name);
    if (value) filter[name] = value;
  }
  const outcome = query.get("outcome");
  if (isOutcome(outcome)) filter.outcome = outcome;
  return filter;
}

/** The call open over the page, when the query names one. */
export const callOf = (query: URLSearchParams): string | undefined => query.get("call") || undefined;

/** The page's own address for a filter and the call open over it; the default range is left out. */
export function hrefOf(filter: ActivityFilter, call?: string): string {
  const query = new URLSearchParams();
  if (filter.range === "custom") {
    query.set("from", filter.from);
    query.set("to", filter.to);
  } else if (filter.range !== "24h") query.set("range", filter.range);
  setNarrowing(query, filter);
  if (call) query.set("call", call);
  const text = query.toString();
  return text ? `/activity?${text}` : "/activity";
}

/** The filter's period alone, with nothing narrowing it. */
export const periodOf = (filter: ActivityFilter): ActivityPeriod => (filter.range === "custom" ? { range: "custom", from: filter.from, to: filter.to } : { range: filter.range });

/** Whether anything narrows the page within its period. */
export const isNarrowed = (filter: ActivityFilter) => Boolean(filter.connector || filter.agent || filter.outcome || filter.search);

/** The API query for a filter: a preset ends now; custom days run from the first's local midnight to the end of the last or now, at most the 30 days the console keeps. */
export function apiQueryOf(filter: ActivityFilter, now: Date): string {
  let from: Date;
  let to = now;
  if (filter.range === "custom") {
    const first = startOf(filter.from) ?? now;
    const last = startOf(filter.to) ?? now;
    to = new Date(Math.min(now.getTime(), new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1).getTime()));
    from = new Date(Math.min(to.getTime(), Math.max(first.getTime(), to.getTime() - 30 * 24 * HOUR)));
  } else {
    const hours = RANGES.find((range) => range.value === filter.range)?.hours ?? 24;
    from = new Date(now.getTime() - hours * HOUR);
  }
  const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
  setNarrowing(query, filter);
  return query.toString();
}

/** One bar of the chart: its start, whether it is an hour or a local day, and its calls by outcome. */
export type Bar = { start: string; unit: "hour" | "day"; ok: number; failed: number; denied: number; waiting: number; total: number };

const localDay = (iso: string) => {
  const at = new Date(iso);
  return `${at.getFullYear()}-${at.getMonth() + 1}-${at.getDate()}`;
};

const HOURLY_UP_TO = 49;

/** Turns the API's hourly counts into bars: one per hour over up to two days, one per local day over longer ranges. */
export function barsOf(hours: readonly HourCount[]): Bar[] {
  const unit = hours.length <= HOURLY_UP_TO ? "hour" : "day";
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

/** A connector or a skill as the activity page names it. */
export type Named = { id: string; title: string; icon?: string | undefined; skill: boolean };

/** The installed connectors and skills by id, for the titles and icons of calls. */
export function namedOf(connectors: readonly Connector[], skills: readonly Skill[]): Map<string, Named> {
  const out = new Map<string, Named>();
  for (const c of connectors) out.set(c.id, { id: c.id, title: c.title, icon: c.icon, skill: false });
  for (const s of skills) out.set(s.id, { id: s.id, title: s.title || s.alias, skill: true });
  return out;
}

/** The file a skill load read: the one named, or SKILL.md. */
export const skillFile = (path: string | undefined) => path || "SKILL.md";
