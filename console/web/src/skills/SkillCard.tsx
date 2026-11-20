import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api, errorText, type Access, type Skill } from "../api";
import { AccessBar } from "../connectors/AccessBar";
import { AccessPicker } from "../connectors/AccessPicker";
import { accessCounts } from "../connectors/summary";
import { useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { ACCENT_BAR, ICON_BUTTON, LIT_EDGE, button } from "../ui/styles";
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
    <li className="flex flex-col gap-2 border-b border-line py-3 last:border-b-0 @lg:flex-row @lg:items-center @lg:gap-6">
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

function RemoveSkill({ skill }: { skill: Skill }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation({
    mutationFn: () => api("DELETE", `/api/skills/${encodeURIComponent(skill.id)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["skills"] }),
  });
  return (
    <>
      <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
        {confirming ? (
          <div role="group" aria-label={t("skills.remove")} className="swap-in bubble flex flex-wrap items-center gap-3 border border-amber/40 bg-amber/5 py-2 pr-2 pl-4">
            <p className="min-w-0 text-sm [overflow-wrap:anywhere]">{t("skills.removeAsk", { name: skill.alias })}</p>
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
    </>
  );
}

function SkillDetails({ id, skill }: { id: string; skill: Skill }) {
  const t = useT();
  return (
    <div id={id} className="swap-in @container mt-5 border-t border-line pt-5">
      <div className="grid gap-x-10 gap-y-6 @3xl:grid-cols-2">
        <SkillFactsView skill={skill} unbound={skill.unbound} />
        <div className="min-w-0">
          <h3 className="text-base font-medium">{t("skills.perKey")}</h3>
          <p className="text-sm text-muted">{t("skills.perKeyIntro")}</p>
          {skill.access.length === 0 ? (
            <p className="py-4 text-sm text-muted">{t("skills.noKeys")}</p>
          ) : (
            <ul className="@container mt-1">
              {skill.access.map((key) => (
                <KeyChoice key={key.agent} skill={skill} agent={key.agent} access={key.access} />
              ))}
            </ul>
          )}
        </div>
      </div>
      <RemoveSkill skill={skill} />
    </div>
  );
}

type SkillCardProps = { skill: Skill; open: boolean; onToggle(button: HTMLElement): void };

/** One installed skill as a card of the grid: who made it, two lines of what it does and how its keys are set; maximized, it spans the row and adds its warnings, licence, source, files and the off, on or ask of each key. */
export function SkillCard({ skill, open, onToggle }: SkillCardProps) {
  const t = useT();
  const details = useId();
  const counts = accessCounts(skill.access);
  const label = t(open ? "skills.minimize" : "skills.maximize", { name: skill.title });
  return (
    <li className={`bubble flex flex-col border border-line-strong bg-ink-raised p-5 transition-[border-color,transform] duration-250 ease-(--ease-glide) ${open ? "relative col-span-full" : LIT_EDGE}`}>
      {open ? <span aria-hidden="true" className={`${ACCENT_BAR} bg-lime`} /> : null}
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base leading-snug font-medium">{skill.title}</h2>
          <p className="truncate font-mono text-xs text-muted" translate="no">
            {skill.alias}@{shortCommit(skill.version)}
            {skill.publisher ? ` · ${t("import.by", { publisher: skill.publisher })}` : ""}
          </p>
        </div>
        <button type="button" aria-expanded={open} aria-controls={open ? details : undefined} aria-label={label} title={label} className={`${ICON_BUTTON} -mt-1.5 -mr-1.5 scroll-mt-32 scroll-mb-6`} onClick={(event) => onToggle(event.currentTarget)}>
          <Icon name={open ? "collapse" : "expand"} className="size-4" />
        </button>
      </div>
      <p className={`mt-3 text-sm text-muted [overflow-wrap:anywhere] ${open ? "max-w-2xl" : "line-clamp-2 min-h-10"}`}>{skill.description}</p>
      <AccessBar counts={counts} className="mt-4" />
      <div className="mt-3 flex flex-wrap gap-1.5">
        {skill.category ? <Bubble>{skill.category}</Bubble> : null}
        <Bubble tone={counts.on + counts.ask > 0 ? "accent" : "neutral"}>{t("skills.keysOn", { count: counts.on + counts.ask, total: skill.access.length })}</Bubble>
        {counts.on > 0 ? <Bubble>{t("access.count.on", { count: counts.on })}</Bubble> : null}
        {counts.ask > 0 ? <Bubble tone="outline">{t("access.count.ask", { count: counts.ask })}</Bubble> : null}
        {skill.state === "quarantined" ? <Bubble tone="amber">{t("connectors.changed", { count: 1 })}</Bubble> : null}
      </div>
      {open ? <SkillDetails id={details} skill={skill} /> : null}
    </li>
  );
}
