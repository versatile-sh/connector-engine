import type { ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import { Display } from "./Display";
import { Marked } from "./Marked";
import { EYEBROW, LEAD } from "./styles";

type PageTitleProps = { section: string; title: string; count?: number | undefined; scope?: string | undefined; children?: ReactNode };

/** A page's title as on the site's directory: a mono eyebrow naming the gateway and the section, the lit serif headline with its lime word and the mono count of what it lists, after whose rows they are when a `scope` names it, then the lead under it. */
export function PageTitle({ section, title, count, scope, children }: PageTitleProps) {
  const t = useT();
  return (
    <>
      <p className={EYEBROW}>
        {t("brand.label")} · {section}
      </p>
      <div className="mt-4 flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <Display id="page-title">
          <Marked text={title} />
        </Display>
        {count === undefined ? null : <span className="font-mono text-sm text-muted tabular-nums">{scope ? `${scope} · ${count}` : count}</span>}
      </div>
      {children ? <div className={`${LEAD} mt-4`}>{children}</div> : null}
    </>
  );
}
