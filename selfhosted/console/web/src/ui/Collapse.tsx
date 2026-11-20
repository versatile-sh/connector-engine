import type { ReactNode } from "react";

type CollapseProps = { id: string; open: boolean; children: ReactNode };

/** The part a disclosure opens and closes: its height and opacity ease over 250ms with the site's glide, and while closed it is inert. */
export function Collapse({ id, open, children }: CollapseProps) {
  return (
    <div id={id} inert={!open} className={`grid transition-[grid-template-rows,opacity] duration-250 ease-(--ease-glide) ${open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"}`}>
      <div className="min-h-0 overflow-hidden">{children}</div>
    </div>
  );
}
