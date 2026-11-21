import type { Connector, DeviceView, NotificationsData } from "../api";
import type { Locale } from "../i18n";

/** How a decided call ended, as the list names it. */
export type DecisionKind = "approved" | "denied" | "expired" | "cancelled";

/** How a call's state reads in the list: approved once it was allowed, whatever it did after; a state the console does not know has no kind. */
export function decisionOf(state: string): DecisionKind | undefined {
  switch (state) {
    case "approved":
    case "executing":
    case "executed":
    case "failed":
      return "approved";
    case "denied":
    case "expired":
    case "cancelled":
      return state;
    default:
      return undefined;
  }
}

/** A connector with the functions and items it holds at Ask. */
export type AskedConnector = { id: string; title: string; icon: string | undefined; names: string[] };

/** The connectors that have something set to Ask, by title, each with the names of what asks, in order; a connector with nothing at Ask is left out. */
export function askedOf(connectors: readonly Connector[]): AskedConnector[] {
  return connectors
    .flatMap((connector) => {
      const names = [...(connector.functions ?? []), ...(connector.items ?? [])].filter((row) => row.access === "ask").map((row) => row.name);
      return names.length > 0 ? [{ id: connector.id, title: connector.title, icon: connector.icon, names: names.sort() }] : [];
    })
    .sort((a, b) => a.title.localeCompare(b.title));
}

/** Where held calls reach the viewer besides the console: the apps that ask in the AI app itself, the computers paired for popups and the chat channels. */
export type Reach = { elicit: number; devices: DeviceView[]; channels: NotificationsData["channels"]; extra: boolean };

/** What the notification settings say about where a held call reaches the viewer; `extra` is whether anything but the console and the AI app is set up. */
export function reachOf(data: NotificationsData): Reach {
  return { elicit: data.apps.filter((app) => app.elicit).length, devices: data.devices, channels: data.channels, extra: data.devices.length + data.channels.length > 0 };
}

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** An instant read against now, in the language of the console: seconds under a minute and a half, minutes under an hour and a half, hours under a day and a half, days beyond; a future instant reads as "in ..." and a past one as "... ago". */
export function relativeOf(at: string, now: number, locale: Locale): string {
  const delta = Date.parse(at) - now;
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const size = Math.abs(delta);
  if (size < 1.5 * MINUTE) return format.format(Math.round(delta / 1000), "second");
  if (size < 1.5 * HOUR) return format.format(Math.round(delta / MINUTE), "minute");
  if (size < 1.5 * DAY) return format.format(Math.round(delta / HOUR), "hour");
  return format.format(Math.round(delta / DAY), "day");
}

const quote = (value: unknown): string => (typeof value === "string" ? value : JSON.stringify(value) ?? "");

/** A short line of what a call asked for, from its arguments: the first fields as `name: value`, cut at `max` characters with an ellipsis; nothing when there are none. */
export function previewOf(args: unknown, max = 140): string {
  if (args === undefined || args === null) return "";
  const text = typeof args === "object" && !Array.isArray(args) ? Object.entries(args).map(([name, value]) => `${name}: ${quote(value)}`).join(", ") : quote(args);
  const line = text.replace(/\s+/g, " ").trim();
  return line.length > max ? `${line.slice(0, max - 1).trimEnd()}…` : line;
}
