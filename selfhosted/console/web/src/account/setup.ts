import type { Access, SetupConnector, SetupOptions, SlotChoice } from "../api";

/** The host of an address, or nothing when it does not parse. */
export function hostOf(raw: string | undefined): string | undefined {
  if (!raw) return undefined;
  try {
    return new URL(raw).host || undefined;
  } catch {
    return undefined;
  }
}

/** A choice the file sets higher than this console proposes: what it asks and what it is lowered to. */
export type Cap = { name: string; asks: Access; allows: Access };

/** The choices of a setup's connector that importing lowers, by name. */
export const capsOf = (c: SetupConnector): Cap[] =>
  Object.entries(c.above ?? {})
    .map(([name, allows]) => ({ name, asks: c.functions?.[name] ?? c.items?.[name] ?? "on", allows }))
    .sort((a, b) => a.name.localeCompare(b.name));

/** Whether a review still has a set to apply with no key picked for it. */
export const keyMissing = (choices: Readonly<Record<string, SlotChoice>>) => Object.values(choices).some((choice) => choice.action === "apply" && !choice.key);

/** The import's `options` for a review's answers: the sets not kept, and the switch for keys made already; nothing when every answer is the default. */
export function optionsOf(choices: Readonly<Record<string, SlotChoice>>, existingKeys: boolean): SetupOptions | undefined {
  const slots = Object.fromEntries(Object.entries(choices).filter(([, choice]) => choice.action !== "keep"));
  const options: SetupOptions = { ...(Object.keys(slots).length > 0 ? { slots } : {}), ...(existingKeys ? { existing_keys: true } : {}) };
  return Object.keys(options).length > 0 ? options : undefined;
}
