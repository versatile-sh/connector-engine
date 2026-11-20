import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, type Access, type Skill } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { skillsQuery } from "../queries";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { button } from "../ui/styles";
import { SkillFactsView } from "./SkillFactsView";
import { shortCommit } from "./summary";

function KeyChoice({ skill, agent, access }: { skill: Skill; agent: string; access: Access }) {
  const t = useT();
  const queryClient = useQueryClient();
  const set = useMutation({
    mutationFn: (next: Access) => api("PUT", `/api/skills/${encodeURIComponent(skill.id)}/keys/${encodeURIComponent(agent)}`, { access: next }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["skills"] }),
  });
  return (
    <li className="flex flex-col gap-2 border-b border-line py-3 last:border-b-0 sm:flex-row sm:items-center sm:gap-6">
      <div className="min-w-0 flex-1">
        <code className="font-mono text-sm font-semibold break-all text-paper" translate="no">
          {agent}
        </code>
        {set.isError ? (
          <p role="alert" className="mt-1 text-sm text-amber">
            {errorText(set.error)}
          </p>
        ) : null}
      </div>
      <AccessPicker name={t("skills.keyName", { skill: skill.alias, key: agent })} of="skill" value={set.isPending && set.variables ? set.variables : access} disabled={set.isPending} onChange={(next) => set.mutate(next)} />
    </li>
  );
}

function SkillCard({ skill }: { skill: Skill }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation({
    mutationFn: () => api("DELETE", `/api/skills/${encodeURIComponent(skill.id)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["skills"] }),
  });
  const on = skill.access.filter((key) => key.access !== "off").length;
  return (
    <li className="arrive bubble border border-line-strong bg-ink-raised p-5 sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-serif text-3xl leading-none font-normal tracking-[-0.02em] break-words">{skill.title}</h2>
          <p className="mt-1 truncate font-mono text-xs text-muted" translate="no">
            {skill.alias}@{shortCommit(skill.version)}
            {skill.publisher ? ` · ${t("import.by", { publisher: skill.publisher })}` : ""}
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {skill.category ? <Bubble>{skill.category}</Bubble> : null}
          <Bubble tone={on > 0 ? "accent" : "neutral"}>{t("skills.keysOn", { count: on, total: skill.access.length })}</Bubble>
          {skill.state === "quarantined" ? <Bubble tone="amber">{t("connectors.changed", { count: 1 })}</Bubble> : null}
        </div>
      </div>
      {skill.description ? <p className="mt-3 max-w-2xl text-sm text-muted">{skill.description}</p> : null}
      <div className="mt-5">
        <SkillFactsView skill={skill} unbound={skill.unbound} />
      </div>
      <h3 className="mt-6 text-base font-medium">{t("skills.perKey")}</h3>
      <p className="text-sm text-muted">{t("skills.perKeyIntro")}</p>
      {skill.access.length === 0 ? (
        <p className="py-4 text-sm text-muted">{t("skills.noKeys")}</p>
      ) : (
        <ul className="mt-1">
          {skill.access.map((key) => (
            <KeyChoice key={key.agent} skill={skill} agent={key.agent} access={key.access} />
          ))}
        </ul>
      )}
      <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
        {confirming ? (
          <div role="group" aria-label={t("skills.remove")} className="swap-in bubble flex flex-wrap items-center gap-3 border border-amber/40 bg-amber/5 py-2 pr-2 pl-4">
            <p className="text-sm">{t("skills.removeAsk", { name: skill.alias })}</p>
            <button type="button" autoFocus disabled={remove.isPending} className={button({ variant: "quiet", size: "xs" })} onClick={() => setConfirming(false)}>
              {t("common.cancel")}
            </button>
            <button type="button" disabled={remove.isPending} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.mutate()}>
              {t("skills.removeConfirm")}
            </button>
          </div>
        ) : (
          <button type="button" className={button({ variant: "danger", size: "xs" })} onClick={() => setConfirming(true)}>
            {t("skills.remove")}
          </button>
        )}
      </div>
      {remove.isError ? (
        <p role="alert" className="mt-2 text-right text-sm text-amber">
          {errorText(remove.error)}
        </p>
      ) : null}
    </li>
  );
}

/** Every installed skill: what it is, its warnings, and off, on or ask for each access key; Import reads a skill link like a connector link. */
export function SkillsPage() {
  const t = useT();
  const skills = useQuery(skillsQuery);
  const [importing, setImporting] = useState(false);
  const list = skills.data ?? [];
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div>
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
        <ul className="flex flex-col gap-4">
          {list.map((skill) => (
            <SkillCard key={skill.id} skill={skill} />
          ))}
        </ul>
      )}
      {importing ? <ImportDialog initialKind="listing" onClose={() => setImporting(false)} /> : null}
    </section>
  );
}
