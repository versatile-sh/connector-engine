import type { ReactNode } from "react";

type BubbleProps = { tone?: "accent" | "neutral" | "amber" | "outline"; title?: string; children: ReactNode };

const TONE = {
  accent: "bg-lime text-ink",
  neutral: "border border-line-strong text-muted",
  outline: "border border-lime/60 text-lime",
  amber: "border border-amber/70 text-amber",
} as const;

/** A few words in a small bubble: a status, a count, a flag. */
export function Bubble({ tone = "neutral", title, children }: BubbleProps) {
  return (
    <span title={title} className={`bubble-word inline-flex max-w-full shrink-0 items-center gap-1 px-2 py-0.5 font-mono text-xs [overflow-wrap:anywhere] ${TONE[tone]}`}>
      {children}
    </span>
  );
}
