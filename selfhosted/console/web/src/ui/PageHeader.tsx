import type { ReactNode } from "react";
import { Display } from "./Display";
import { Marked } from "./Marked";
import { LEAD } from "./styles";

type PageHeaderProps = { title: string; count?: number | undefined; scope?: string | undefined; actions?: ReactNode; children?: ReactNode };

/** A page's header: the lit serif headline with its lime word, the count of what the page lists as a quiet mono chip right after it, the primary actions on the same baseline at the far edge, and the lead under them; the shell's sidebar and top strip already name the section. */
export function PageHeader({ title, count, scope, actions, children }: PageHeaderProps) {
  return (
    <header className="mb-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-4">
        <Display id="page-title">
          <Marked text={title} />
        </Display>
        {count === undefined ? null : <span className="rounded-full bg-paper/10 px-2.5 py-0.5 font-mono text-xs text-muted tabular-nums">{scope ? `${scope} · ${count}` : count}</span>}
        {actions ? <div className="flex flex-wrap items-center gap-2 sm:ml-auto">{actions}</div> : null}
      </div>
      {children ? <div className={`${LEAD} mt-4`}>{children}</div> : null}
    </header>
  );
}
