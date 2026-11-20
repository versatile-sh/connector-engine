import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { button } from "../ui/styles";
import { expandedOf, skillsHref, toggled } from "./expanded";
import { SkillCard } from "./SkillCard";
import { useReflow } from "./useReflow";

/** Every installed skill as a grid of compact cards; one at a time maximizes in place, named by `?skill=` so it survives a reload and Back. Import reads a skill link like a connector link. */
export function SkillsPage() {
  const t = useT();
  const { query } = useLocation();
  const skills = useQuery(skillsQuery);
  const [importing, setImporting] = useState(false);
  const list = skills.data ?? [];
  const open = expandedOf(query, list);
  const reflow = useReflow<HTMLUListElement>(open);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <PageTitle section={t("nav.skills")} title={t("skills.title")}>
            {skills.isSuccess ? <Bubble tone={list.length > 0 ? "accent" : "neutral"}>{list.length}</Bubble> : null}
          </PageTitle>
          <p className="mt-3 max-w-2xl text-muted">{t("skills.intro")}</p>
        </div>
        <button type="button" className={button({ variant: "accent" })} onClick={() => setImporting(true)}>
          <Icon name="plus" className="size-4" />
          {t("connectors.import")}
        </button>
      </header>
      {skills.isError ? (
        <Notice
          action={
            <button type="button" className={button({ variant: "secondary", size: "xs" })} onClick={() => void skills.refetch()}>
              {t("common.retry")}
            </button>
          }
        >
          {errorText(skills.error)}
        </Notice>
      ) : skills.isPending ? (
        <div aria-hidden="true" className="shimmer bubble h-48" />
      ) : list.length === 0 ? (
        <section className="arrive bubble grid overflow-hidden border border-line-strong lg:grid-cols-2">
          <div className="relative isolate bg-lime p-6 text-ink sm:p-8">
            <div aria-hidden="true" className="backdrop-grid-ink absolute inset-0 -z-10" />
            <h2 className="font-serif text-4xl leading-[0.98] font-normal tracking-[-0.02em] sm:text-5xl">{t("skills.empty.title")}</h2>
          </div>
          <div className="flex flex-col justify-center gap-3 bg-ink-raised p-6 sm:p-8">
            <p className="text-muted">{t("skills.empty.body")}</p>
          </div>
        </section>
      ) : (
        <ul ref={reflow.list} className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.map((skill) => (
            <SkillCard
              key={skill.id}
              skill={skill}
              open={skill.id === open}
              onToggle={(pressed) => {
                reflow.before(pressed);
                navigate(skillsHref(query, toggled(open, skill.id)), { keepScroll: true });
              }}
            />
          ))}
        </ul>
      )}
      {importing ? <ImportDialog initialKind="listing" onClose={() => setImporting(false)} /> : null}
    </section>
  );
}
