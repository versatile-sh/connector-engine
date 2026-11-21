import type { ReactNode } from "react";
import { Display } from "./Display";
import { Marked } from "./Marked";
import { LEAD } from "./styles";

type PageHeaderProps = { title: string; count?: number | undefined; scope?: string | undefined; actions?: ReactNode; children?: ReactNode };

/** A page's header, centred in the surface as the landing site's heroes are: the lit serif headline with its lime word and, right after it on the same baseline, the count of what the page lists as a quiet mono chip, then the lead, then the page's primary actions; the shell's sidebar and top strip already name the section. */
export function PageHeader({ title, count, scope, actions, children }: PageHeaderProps) {
  return (
    <header className="mb-2 text-center">
      <div className="flex flex-wrap items-baseline justify-center gap-x-3 gap-y-2">
        <Display id="page-title">
          <Marked text={title} />
        </Display>
        {count === undefined ? null : <span className="rounded-full bg-paper/10 px-2.5 py-0.5 font-mono text-xs text-muted tabular-nums">{scope ? `${scope} · ${count}` : count}</span>}
      </div>
      {children ? <div className={`${LEAD} mx-auto mt-4`}>{children}</div> : null}
      {actions ? <div className="mt-6 flex flex-wrap items-center justify-center gap-2">{actions}</div> : null}
    </header>
  );
}
