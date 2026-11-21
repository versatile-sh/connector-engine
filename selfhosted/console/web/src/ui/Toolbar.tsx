import type { ReactNode } from "react";

type ToolbarProps = { label?: string | undefined; className?: string; children: ReactNode };

/** The row of controls directly over a page's content, as wide as the page's centred container and centring what it holds: one row from `sm` up, wrapping only below it, so the one flexible item (the search, `flex-1 min-w-0`, at most 24rem) takes up whatever the others leave. */
export function Toolbar({ label, className = "", children }: ToolbarProps) {
  return (
    <div role={label ? "group" : undefined} aria-label={label} className={`flex min-w-0 flex-wrap items-center justify-center gap-2 sm:flex-nowrap${className ? ` ${className}` : ""}`}>
      {children}
    </div>
  );
}
