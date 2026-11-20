import { useId, useState, type ReactNode } from "react";
import { Collapse } from "./Collapse";
import { Icon } from "./Icon";

type DisclosureProps = { summary: ReactNode; className?: string; children: ReactNode };

/** A summary bar that opens and closes what it holds in place, closed at first: a button with `aria-expanded`, a chevron that turns, and the content easing open. */
export function Disclosure({ summary, className = "", children }: DisclosureProps) {
  const id = useId();
  const [open, setOpen] = useState(false);
  return (
    <div className={className}>
      <button type="button" aria-expanded={open} aria-controls={id} onClick={() => setOpen((was) => !was)} className="flex w-full cursor-pointer items-center gap-3 px-4 py-2.5 text-left text-sm select-none pointer-coarse:min-h-11">
        <span className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-x-3 gap-y-0.5">{summary}</span>
        <Icon name="chevron" className={`size-4 text-muted transition-transform duration-250 ease-(--ease-glide) ${open ? "rotate-180" : ""}`} />
      </button>
      <Collapse id={id} open={open}>
        {children}
      </Collapse>
    </div>
  );
}
