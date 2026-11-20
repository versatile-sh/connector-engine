import type { ReactNode } from "react";

/** An on/off choice as a labelled switch: the whole row is the target, the thumb slides, and on is paper, never lime. */
export function Switch({ checked, onChange, children, hint }: { checked: boolean; onChange(checked: boolean): void; children: ReactNode; hint?: ReactNode }) {
  return (
    <label className="flex cursor-pointer items-center justify-between gap-4 pointer-coarse:min-h-11">
      <span className="flex min-w-0 flex-col">
        <span className="text-sm font-medium">{children}</span>
        {hint ? <span className="text-[0.8125rem] leading-snug text-muted">{hint}</span> : null}
      </span>
      <input type="checkbox" role="switch" checked={checked} onChange={(event) => onChange(event.target.checked)} className="peer sr-only" />
      <span
        aria-hidden="true"
        className="relative h-6 w-11 shrink-0 rounded-full bg-line-strong transition-colors duration-250 peer-checked:bg-paper peer-focus-visible:outline-1 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-paper/50 after:absolute after:top-0.5 after:left-0.5 after:size-5 after:rounded-full after:bg-paper after:transition-[translate,background-color] after:duration-250 after:ease-(--ease-glide) peer-checked:after:translate-x-5 peer-checked:after:bg-ink"
      />
    </label>
  );
}
