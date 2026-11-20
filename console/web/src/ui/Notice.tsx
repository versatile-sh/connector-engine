import type { ReactNode } from "react";

/** A short message in a bubble with a coloured edge: amber when something went wrong, lime when it went well. */
export function Notice({ tone = "amber", children, action }: { tone?: "amber" | "accent"; children: ReactNode; action?: ReactNode }) {
  return (
    <div role={tone === "amber" ? "alert" : "status"} className={`arrive bubble flex items-center gap-3 border border-line-strong border-l-4 bg-ink-raised py-3 pr-3 pl-4 text-sm ${tone === "amber" ? "border-l-amber" : "border-l-lime"}`}>
      <p className="min-w-0 flex-1">{children}</p>
      {action}
    </div>
  );
}
