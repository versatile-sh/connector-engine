import type { ReactNode } from "react";

type ChoiceRowProps = { name: string; title?: string | undefined; lead?: ReactNode; flags?: ReactNode; description?: ReactNode; children: ReactNode };

/** One row of a list of choices in a well, for a function, an item or an access key: an optional lead such as its pick, its name in mono with its title and flags, what it is under them, and its choice on the right, never under it. */
export function ChoiceRow({ name, title, lead, flags, description, children }: ChoiceRowProps) {
  return (
    <li className="@container border-b border-line px-4 py-3 last:border-b-0 sm:px-5">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 @md:gap-x-6">
        <div className="min-w-0">
          <div className="flex min-h-8 flex-wrap items-center gap-x-2 gap-y-1">
            {lead}
            <code className="min-w-0 font-mono text-sm font-medium text-paper [overflow-wrap:anywhere]" translate="no">
              {name}
            </code>
            {title && title !== name ? <span className="text-sm text-muted">{title}</span> : null}
            {flags}
          </div>
          {typeof description === "string" ? (
            <p title={description} className="line-clamp-2 text-sm text-pretty text-muted">
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
