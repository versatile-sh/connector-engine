/** Where a row of a list is: shown since before, new since the last change, or gone and on its way out. */
export type RowPhase = "stay" | "enter" | "leave";

/** One row as a list draws it, with its key and its phase. */
export type Kept<T> = { row: T; key: string; phase: RowPhase };

/** How long a row takes to come in or go out, as the stylesheet's `[data-row]` animations. */
export const ROW_MS = 250;

/** The rows a list draws once `rows` replace the ones it drew: each new row entering, each kept one staying, and each one gone leaving at the place it held, before the next row that stays; rows already leaving are dropped. */
export function keptOf<T>(drawn: readonly Kept<T>[], rows: readonly T[], keyOf: (row: T) => string): Kept<T>[] {
  const now = new Set(rows.map(keyOf));
  const was = new Set(drawn.filter((kept) => kept.phase !== "leave").map((kept) => kept.key));
  const leavingBefore = new Map<string, Kept<T>[]>();
  let gone: Kept<T>[] = [];
  for (const kept of drawn) {
    if (kept.phase === "leave") continue;
    if (!now.has(kept.key)) gone.push({ ...kept, phase: "leave" });
    else if (gone.length > 0) {
      leavingBefore.set(kept.key, gone);
      gone = [];
    }
  }
  return [
    ...rows.flatMap((row) => {
      const key = keyOf(row);
      return [...(leavingBefore.get(key) ?? []), { row, key, phase: was.has(key) ? ("stay" as const) : ("enter" as const) }];
    }),
    ...gone,
  ];
}

/** The attributes that play a row's phase: `data-row` for the stylesheet's height and fade, and `inert` while it leaves. */
export const rowMotion = (phase: RowPhase | undefined) => ({ "data-row": phase, inert: phase === "leave" });
