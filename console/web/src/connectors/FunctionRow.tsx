import type { ReactNode } from "react";

/** One function as a row: its name in mono with its flags, the description under it, and its access choice on the right, or under it when the list it sits in is narrower than 32rem. */
export function FunctionRow({ name, title, description, flags, children }: { name: string; title?: string | undefined; description?: string | undefined; flags?: ReactNode; children: ReactNode }) {
  return (
    <li className="flex flex-col gap-3 border-b border-line py-3.5 last:border-b-0 @lg:flex-row @lg:items-center @lg:gap-6">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <code className="font-mono text-sm font-semibold break-all text-paper" translate="no">
            {name}
          </code>
          {title && title !== name ? <span className="text-sm text-muted">{title}</span> : null}
          {flags}
        </div>
        {description ? (
          <p title={description} className="mt-1 line-clamp-2 text-sm text-muted [overflow-wrap:anywhere]">
            {description}
          </p>
        ) : null}
      </div>
      {children}
    </li>
  );
}
