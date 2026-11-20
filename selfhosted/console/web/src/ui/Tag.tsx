import type { ReactNode } from "react";

type TagProps = { tone?: "muted" | "paper" | "lime" | "amber"; dot?: boolean; title?: string | undefined; children: ReactNode };

const TONE = { muted: "text-muted", paper: "text-paper", lime: "text-lime", amber: "text-amber-400" } as const;

/** A machine fact as small mono text in a tone, as the site's function labels: a status, a count, a flag; `dot` marks a live or warning state. */
export function Tag({ tone = "muted", dot = false, title, children }: TagProps) {
  return (
    <span title={title} className={`inline-flex min-w-0 items-center gap-1.5 font-mono text-xs leading-5 ${TONE[tone]}`}>
      {dot ? <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-current" /> : null}
      {children}
    </span>
  );
}
