import type { ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import { Marked } from "./Marked";
import { EYEBROW } from "./styles";

type PageTitleProps = { section: string; title: string; children?: ReactNode };

/** A page's title as on versatile.sh: a mono eyebrow naming the gateway and the section, then the lit serif headline with its lime word, and beside it whatever counts it. */
export function PageTitle({ section, title, children }: PageTitleProps) {
  const t = useT();
  return (
    <>
      <p className={EYEBROW}>
        {t("brand.label")} · {section}
      </p>
      <div className="mt-4 flex items-center gap-3">
        <h1 id="page-title" className="text-lit serif-room font-serif text-5xl leading-[0.98] font-normal tracking-[-0.02em] sm:text-6xl">
          <Marked text={title} />
        </h1>
        {children}
      </div>
    </>
  );
}
