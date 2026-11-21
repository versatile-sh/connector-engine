import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { useViewer } from "../people/useViewer";
import { skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { LongList } from "../ui/LongList";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { PANEL, SKELETON, button } from "../ui/styles";
import { expandedOf, skillsHref } from "./expanded";
import { SkillCard } from "./SkillCard";

/** Every installed skill as a grid of compact cards in one panel, with a filter past six of them; one at a time opens in a dialog over the grid, named by `?skill=` so it survives a reload and Back closes it. Import, for those the server lets manage skills, reads a skill link like a connector link. */
export function SkillsPage() {
  const t = useT();
  const { query } = useLocation();
  const skills = useQuery(skillsQuery);
  const [importing, setImporting] = useState(false);
  const manage = useViewer().can("skills.manage");
  const list = skills.data ?? [];
  const open = expandedOf(query, list);
  const pushed = useRef(false);
  const close = () => {
    if (pushed.current) window.history.back();
    else navigate(skillsHref(query), { replace: true, keepScroll: true });
  };
  return (
    <section aria-labelledby="page-title">
      <header className="mb-10 flex flex-wrap items-end justify-between gap-6">
        <div className="min-w-0">
          <PageTitle section={t("nav.skills")} title={t("skills.title")} count={skills.isSuccess ? list.length : undefined}>
            {t("skills.intro")}
          </PageTitle>
        </div>
        {manage ? (
          <button type="button" className={button()} onClick={() => setImporting(true)}>
            <Icon name="plus" className="size-4" />
            {t("connectors.import")}
          </button>
        ) : null}
      </header>
      <div className={`${PANEL} @container p-4 sm:p-6 lg:p-8`}>
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
          <div aria-hidden="true" className={`${SKELETON} h-48`} />
        ) : list.length === 0 ? (
          <EmptyState title={t("skills.empty.title")} body={t("skills.empty.body")} />
        ) : (
          <LongList rows={list} textOf={(skill) => `${skill.title} ${skill.alias} ${skill.publisher ?? ""} ${skill.description ?? ""}`} keyOf={(skill) => skill.id} label={t("skills.filter")} whole>
            {(shown) => (
              <ul className="grid gap-6 @xl:grid-cols-2 @4xl:grid-cols-3">
                {shown.map(({ row: skill, phase }) => (
                  <SkillCard
                    key={skill.id}
                    phase={phase}
                    skill={skill}
                    href={skillsHref(query, skill.id)}
                    open={skill.id === open}
                    onOpen={() => {
                      pushed.current = true;
                      navigate(skillsHref(query, skill.id), { keepScroll: true });
                    }}
                    onClose={close}
                  />
                ))}
              </ul>
            )}
          </LongList>
        )}
      </div>
      {importing ? <ImportDialog initialKind="listing" onClose={() => setImporting(false)} /> : null}
    </section>
  );
}
