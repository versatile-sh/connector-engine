import type { ReactNode } from "react";
import { ACCENT_BAR } from "../ui/styles";

/** A plain amber caution with the left accent bar; it is read in place, never announced. */
export function Warning({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <p className={`bubble relative border border-amber/40 bg-amber/5 py-2.5 pr-3 pl-5 text-sm [overflow-wrap:anywhere] ${className}`}>
      <span aria-hidden="true" className={`${ACCENT_BAR} bg-amber`} />
      {children}
    </p>
  );
}
