import { useEffect, useState } from "react";
import { keptOf, ROW_MS, type Kept } from "./kept";

const still = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const staying = <T>(rows: readonly T[], keyOf: (row: T) => string): Kept<T>[] => rows.map((row) => ({ row, key: keyOf(row), phase: "stay" }));

/** The rows a list draws for `rows`, each with its phase: a new row enters and a gone one stays `ROW_MS` to leave; nothing moves on the first draw or under reduced motion. */
export function useKept<T>(rows: readonly T[], keyOf: (row: T) => string): readonly Kept<T>[] {
  const keys = rows.map(keyOf).join("\u0000");
  const [drawn, setDrawn] = useState(() => ({ keys, kept: staying(rows, keyOf) }));
  if (drawn.keys !== keys) setDrawn({ keys, kept: still() ? staying(rows, keyOf) : keptOf(drawn.kept, rows, keyOf) });
  const leaving = drawn.kept.some((kept) => kept.phase === "leave");
  useEffect(
    function dropLeftRows() {
      if (!leaving) return;
      const timer = window.setTimeout(() => setDrawn((now) => ({ ...now, kept: now.kept.filter((kept) => kept.phase !== "leave") })), ROW_MS);
      return () => window.clearTimeout(timer);
    },
    [leaving, drawn.keys],
  );
  const latest = new Map(rows.map((row) => [keyOf(row), row]));
  return drawn.kept.map((kept) => (kept.phase === "leave" ? kept : { ...kept, row: latest.get(kept.key) ?? kept.row }));
}
