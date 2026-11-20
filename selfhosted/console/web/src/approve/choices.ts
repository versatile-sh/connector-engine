import type { ApprovalChoice } from "../api";

const ORDER: readonly ApprovalChoice[] = ["once", "session", "always", "deny"];

/** What a list row offers, and a record that names no choices: everything but the session answer, whose length only the call's own page names. */
export const DEFAULT_CHOICES: readonly ApprovalChoice[] = ["once", "always", "deny"];

/** The offered answers in the order the buttons show them; unknown values from the server are dropped. */
export const offeredChoices = (choices: readonly string[] | undefined): ApprovalChoice[] =>
  ORDER.filter((choice) => (choices ?? DEFAULT_CHOICES).includes(choice));


/** How long Allow for this session lasts from `now`, rounded up, in hours from 90 minutes up and in minutes below, or undefined when the record does not say. */
export function sessionSpan(until: string | undefined, now: number): { unit: "hours" | "minutes"; count: number } | undefined {
  const left = until ? new Date(until).getTime() - now : 0;
  if (!(left > 0)) return undefined;
  const minutes = Math.ceil(left / 60_000);
  return minutes < 90 ? { unit: "minutes", count: minutes } : { unit: "hours", count: Math.ceil(minutes / 60) };
}

/** The longest string shown whole in the arguments summary. */
export const MAX_VALUE = 160;

const shorten = (value: unknown, max: number, cut: { count: number }): unknown => {
  if (typeof value === "string") {
    if (value.length <= max) return value;
    cut.count += 1;
    return `${value.slice(0, max)}… (+${value.length - max})`;
  }
  if (Array.isArray(value)) return value.map((item) => shorten(item, max, cut));
  if (typeof value === "object" && value !== null) return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, shorten(item, max, cut)]));
  return value;
};

/** The call's arguments as pretty JSON with every long string cut to `max` characters, and how many were cut. */
export function summarizeArguments(value: unknown, max = MAX_VALUE): { text: string; cut: number } | undefined {
  if (value === undefined || value === null) return undefined;
  if (typeof value === "object" && Object.keys(value).length === 0) return undefined;
  const cut = { count: 0 };
  return { text: JSON.stringify(shorten(value, max, cut), null, 2), cut: cut.count };
}

/** Whole seconds until `expiresAt`, never below zero. */
export const secondsLeft = (expiresAt: string, now: number) => Math.max(0, Math.ceil((new Date(expiresAt).getTime() - now) / 1000));

/** A countdown as m:ss. */
export const clock = (seconds: number) => `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
