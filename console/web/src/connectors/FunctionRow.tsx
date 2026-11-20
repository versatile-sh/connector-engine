import type { ReactNode } from "react";

/** One function as a row: its name in mono with its flags, the description under it, and its access choice on the right (under it on a phone). */
export function FunctionRow({ name, title, description, flags, children }: { name: string; title?: string | undefined; description?: string | undefined; flags?: ReactNode; children: ReactNode }) {
  return (
    <li className="flex flex-col gap-3 border-b border-line py-3.5 last:border-b-0 sm:flex-row sm:items-center sm:gap-6">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <code className="font-mono text-sm font-semibold break-all text-fg" translate="no">
            {name}
          </code>
          {title && title !== name ? <span className="text-sm text-muted">{title}</span> : null}
          {flags}
        </div>
        {description ? (
          <p title={description} className="mt-1 line-clamp-2 text-sm text-muted">
            {description}
          </p>
        ) : null}
      </div>
      {children}
    </li>
  );
}
