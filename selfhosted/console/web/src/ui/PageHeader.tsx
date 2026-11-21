import type { ReactNode } from "react";
import { Display } from "./Display";
import { Marked } from "./Marked";
import { LEAD } from "./styles";

type PageTitleProps = { title: string; count?: number | undefined; scope?: string | undefined; children?: ReactNode };

/** A page's title as on the site's directory: the lit serif headline with its lime word and the mono count of what it lists, after whose rows they are when a `scope` names it, then the lead under it; the shell's sidebar and top strip already name the section. */
export function PageTitle({ title, count, scope, children }: PageTitleProps) {
  return (
    <>
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <Display id="page-title">
          <Marked text={title} />
        </Display>
        {count === undefined ? null : <span className="font-mono text-sm text-muted tabular-nums">{scope ? `${scope} · ${count}` : count}</span>}
      </div>
      {children ? <div className={`${LEAD} mt-4`}>{children}</div> : null}
    </>
  );
}
