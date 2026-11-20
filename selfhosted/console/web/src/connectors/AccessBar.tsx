import type { Access } from "../api";

const FILL = { on: "bg-paper", ask: "bg-paper/40" } as const;

/** How a set of choices splits between on, in paper, and ask, in paper at 40%, as one thin bar whose empty track is off. */
export function AccessBar({ counts, className = "" }: { counts: Record<Access, number>; className?: string }) {
  const total = counts.off + counts.on + counts.ask;
  return (
    <div aria-hidden="true" className={`flex h-1.5 overflow-hidden rounded-full bg-line ${className}`}>
      {(["on", "ask"] as const).map((access) => (counts[access] > 0 ? <span key={access} className={FILL[access]} style={{ width: `${(counts[access] / total) * 100}%` }} /> : null))}
    </div>
  );
}
