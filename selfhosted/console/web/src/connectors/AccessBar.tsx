import type { Access } from "../api";

const FILL = { on: "bg-paper/60", ask: "bg-amber-400/50" } as const;

/** How a set of choices splits between on, in paper at 60%, and ask, in amber at 50%, as a 2px hairline whose empty track, in line, is off. */
export function AccessBar({ counts, className = "" }: { counts: Record<Access, number>; className?: string }) {
  const total = counts.off + counts.on + counts.ask;
  return (
    <div aria-hidden="true" className={`flex h-0.5 overflow-hidden rounded-full bg-line ${className}`}>
      {(["on", "ask"] as const).map((access) => (counts[access] > 0 ? <span key={access} className={FILL[access]} style={{ width: `${(counts[access] / total) * 100}%` }} /> : null))}
    </div>
  );
}
