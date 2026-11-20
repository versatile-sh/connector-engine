import { errorCode, errorStatus, type BackupKind, type Download } from "../api";
import type { MessageKey } from "../i18n";

/** A full backup older than this many days, or none at all, earns a nudge. */
export const NUDGE_DAYS = 30;

/** The shortest account password the console takes. */
export const PASSWORD_MIN = 10;

/** The shortest passphrase a full backup takes. */
export const PASSPHRASE_MIN = 12;

/** How long after a full import an answer that the console is ready may still come from the old process. */
export const RESTART_GRACE_MS = 4_000;

const DAY_MS = 86_400_000;

/** Whole days from an RFC 3339 time to now; nothing when it is absent, unreadable or the zero time of a console that never exported. */
export function daysSince(iso: string | undefined, now: Date): number | undefined {
  const at = iso ? Date.parse(iso) : Number.NaN;
  if (!(at > 0)) return undefined;
  return Math.max(0, Math.floor((now.getTime() - at) / DAY_MS));
}

/** Whether to nudge the owner to export: never exported, or longer ago than the nudge allows. */
export const needsExport = (days: number | undefined) => days === undefined || days > NUDGE_DAYS;

/** What a file is by its name: `.cebak` is a full backup, `.yaml` or `.yml` a setup; the console checks the bytes either way. */
export function backupKindOf(name: string): BackupKind | undefined {
  const lower = name.toLowerCase();
  if (lower.endsWith(".cebak")) return "full";
  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) return "setup";
  return undefined;
}

/** What is wrong with a new secret typed twice, if anything: too short, or the two differ. */
export function secretProblem(value: string, again: string, min: number): MessageKey | undefined {
  if (value.length < min) return "account.tooShort";
  if (value !== again) return "account.mismatch";
  return undefined;
}

/** The message for a refused password check: wrong password, or too many tries. */
export function passwordRefusal(error: unknown): MessageKey | undefined {
  if (errorCode(error) === "slow_down") return "consent.tooMany";
  if (errorStatus(error) === 401) return "account.wrongPassword";
  return undefined;
}

/** Whether the console came back after a full import: ready, and past the grace in which the old process may still answer. */
export const restarted = (ready: boolean, startedAt: number, now: number) => ready && now - startedAt >= RESTART_GRACE_MS;

/** Hands a file the console answered to the browser as a download, under its own name or the fallback. */
export function saveFile({ blob, name }: Download, fallback: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name ?? fallback;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1_000);
}
