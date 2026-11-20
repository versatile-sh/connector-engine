import type { Access, Item, ItemKind } from "../api";

/** A connector's items split by kind, each group in the order the server listed them. */
export type ItemGroups = Record<ItemKind, Item[]>;

/** The order the item sections are shown in. */
export const ITEM_KINDS: readonly ItemKind[] = ["instructions", "prompt", "resource"];

/** Splits items by kind; a missing list counts as empty. */
export const groupItems = (items: readonly Item[] | null | undefined): ItemGroups => {
  const groups: ItemGroups = { instructions: [], prompt: [], resource: [] };
  for (const item of items ?? []) groups[item.kind].push(item);
  return groups;
};

/** The choices an item of a kind takes: instructions are only off or on, prompts and resources also ask. */
export const accessChoices = (kind: ItemKind): readonly Access[] => (kind === "instructions" ? ["off", "on"] : ["off", "on", "ask"]);
