import type { ReactNode } from "react";
import { ACCENT_BAR } from "./styles";

/** A short message in a bubble with a coloured bar on its left: amber when something went wrong, lime when it went well. */
export function Notice({ tone = "amber", children, action }: { tone?: "amber" | "accent"; children: ReactNode; action?: ReactNode }) {
  return (
    <div role={tone === "amber" ? "alert" : "status"} className="arrive bubble relative flex items-center gap-3 border border-line-strong bg-ink-raised py-3 pr-3 pl-5 text-sm">
      <span aria-hidden="true" className={`${ACCENT_BAR} ${tone === "amber" ? "bg-amber" : "bg-lime"}`} />
      <p className="min-w-0 flex-1">{children}</p>
      {action}
    </div>
  );
}
