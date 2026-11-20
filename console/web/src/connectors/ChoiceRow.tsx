import type { ReactNode } from "react";

type ChoiceRowProps = { name: string; title?: string | undefined; flags?: ReactNode; description?: ReactNode; children: ReactNode };

/** One row of a list of choices, for a function, an item or an access key: its name in mono with its title and flags, what it is under them, and its choice on the right, never under it. */
export function ChoiceRow({ name, title, flags, description, children }: ChoiceRowProps) {
  return (
    <li className="@container border-b border-line py-3 last:border-b-0">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 @md:gap-x-6">
        <div className="min-w-0">
          <div className="flex min-h-8 flex-wrap items-center gap-x-2 gap-y-1">
            <code className="min-w-0 font-mono text-sm font-semibold text-paper [overflow-wrap:anywhere]" translate="no">
              {name}
            </code>
            {title && title !== name ? <span className="text-sm text-muted [overflow-wrap:anywhere]">{title}</span> : null}
            {flags}
          </div>
          {typeof description === "string" ? (
            <p title={description} className="line-clamp-2 text-sm text-muted [overflow-wrap:anywhere]">
              {description}
            </p>
          ) : (
            description
          )}
        </div>
        {children}
      </div>
    </li>
  );
}
