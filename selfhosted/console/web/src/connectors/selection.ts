import type { Access, Kind } from "../api";

/** How much of a set of rows is picked: all of them, some, or none. */
export type Picked = "all" | "some" | "none";

/** How much of `names` the selection holds; no names count as none. */
export const pickedOf = (selected: ReadonlySet<string>, names: readonly string[]): Picked => {
  const count = names.filter((name) => selected.has(name)).length;
  if (count === 0) return "none";
  return count === names.length ? "all" : "some";
};

/** The selection after its select-all over `names` is pressed: every one of them when not all were picked, none of them when all were; picks outside `names` stay. */
export const toggledAll = (selected: ReadonlySet<string>, names: readonly string[]): ReadonlySet<string> => {
  const next = new Set(selected);
  const all = pickedOf(selected, names) === "all";
  for (const name of names) {
    if (all) next.delete(name);
    else next.add(name);
  }
  return next;
};

/** The selection with one name picked or unpicked. */
export const toggledOne = (selected: ReadonlySet<string>, name: string): ReadonlySet<string> => {
  const next = new Set(selected);
  if (!next.delete(name)) next.add(name);
  return next;
};

const RANK: Record<Access, number> = { off: 0, ask: 1, on: 2 };

/** The widest default among the picked rows: a key held to the default can take nothing wider for any of them. */
export const widestOf = (bases: readonly Access[]): Access | undefined => bases.reduce<Access | undefined>((widest, base) => (widest === undefined || RANK[base] > RANK[widest] ? base : widest), undefined);

/** A one-click change of every function of a kind: reads on, changes ask, deletes off. */
export type Preset = { kind: Kind; access: Access };

/** The class presets, in the order the menu lists them. */
export const PRESETS: readonly Preset[] = [
  { kind: "read", access: "on" },
  { kind: "write", access: "ask" },
  { kind: "destructive", access: "off" },
];

/** The functions a preset changes: those whose nature is its kind. */
export const presetNames = (functions: readonly { name: string; kind: Kind }[], preset: Preset) => functions.filter((fn) => fn.kind === preset.kind).map((fn) => fn.name);
