import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import type { Keep } from "../api";
import { deleteActivity, pruneActivity, saveRecording, type Recording } from "../mutations";
import { useViewer } from "../people/useViewer";
import { activityPruneQuery, activitySettingsQuery } from "../queries";
import { Icon } from "../ui/Icon";
import { Segmented } from "../ui/Segmented";
import { Select } from "../ui/Select";
import { Sheet } from "../ui/Sheet";
import { CAPTION, SKELETON, SURFACE, TITLE, button } from "../ui/styles";
import { Switch } from "../ui/Switch";
import { dayOf, daysBetween, keptDaysOf, keptSince, pruneBodyOf, type PruneChoice } from "./activity";

const KEEPS: readonly Keep[] = ["off", "7d", "30d", "90d", "forever"];

const OLDER_DAYS = [1, 7, 30, 90] as const;

function DeleteAll({ onClose }: { onClose(): void }) {
  const t = useT();
  const remove = useMutation(deleteActivity);
  return (
    <Sheet title={t("activity.settings.deleteTitle")} closeLabel={t("common.close")} onClose={onClose}>
      <p className="text-muted">{t("activity.settings.deleteBody")}</p>
      <p role="alert" className="mt-3 min-h-5 text-sm text-amber-400">
        {remove.isError ? errorText(remove.error) : ""}
      </p>
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" autoFocus className={button({ variant: "secondary" })} onClick={onClose}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={remove.isPending} className={button({ variant: "danger" })} onClick={() => remove.mutate(undefined, { onSuccess: onClose })}>
          <Icon name="trash" className="size-4" />
          {t("activity.settings.deleteConfirm")}
        </button>
      </div>
    </Sheet>
  );
}

function PrunePeriod({ keptDays, onClose }: { keptDays: number; onClose(): void }) {
  const t = useT();
  const now = new Date();
  const [choice, setChoice] = useState<PruneChoice>({ mode: "older", days: 30 });
  const remove = useMutation(pruneActivity);
  const body = pruneBodyOf(choice);
  const count = useQuery({ ...activityPruneQuery(body ?? { older_than_days: 0 }), enabled: body !== undefined });
  const first = choice.mode === "days" ? choice.from : dayOf(now);
  const last = choice.mode === "days" ? choice.to : dayOf(now);
  const dayOption = (day: string) => ({ value: day, text: day });
  return (
    <Sheet title={t("activity.prune.title")} closeLabel={t("common.close")} onClose={onClose}>
      <div className="flex flex-col gap-4">
        <Segmented<PruneChoice["mode"]>
          label={t("activity.prune.mode")}
          value={choice.mode}
          options={[
            { value: "older", text: t("activity.prune.older") },
            { value: "days", text: t("activity.prune.days") },
          ]}
          onChange={(mode) => setChoice(mode === "older" ? { mode, days: 30 } : { mode, from: dayOf(now), to: dayOf(now) })}
        />
        {choice.mode === "older" ? (
          <Select<string> label={t("activity.prune.older")} value={String(choice.days)} options={OLDER_DAYS.map((n) => ({ value: String(n), text: t("activity.prune.olderDays", { days: n }) }))} onChange={(n) => setChoice({ mode: "older", days: Number(n) })} className="w-56" />
        ) : (
          <span className="flex items-center gap-1.5">
            <Select<string> label={t("activity.from")} value={first} options={daysBetween(keptSince(now, keptDays), last).map(dayOption)} onChange={(from) => setChoice({ mode: "days", from, to: last })} className="w-40" />
            <span aria-hidden="true" className="text-muted">
              –
            </span>
            <Select<string> label={t("activity.to")} value={last} options={daysBetween(first, dayOf(now)).map(dayOption)} onChange={(to) => setChoice({ mode: "days", from: first, to })} className="w-40" />
          </span>
        )}
      </div>
      <p role="status" className="mt-4 min-h-6 text-muted">
        {count.data ? t("activity.prune.count", { count: count.data.count }) : count.isError ? errorText(count.error) : t("activity.prune.counting")}
      </p>
      <p role="alert" className="mt-1 min-h-5 text-sm text-amber-400">
        {remove.isError ? errorText(remove.error) : ""}
      </p>
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" autoFocus className={button({ variant: "secondary" })} onClick={onClose}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={!body || !count.data || count.data.count === 0 || remove.isPending} className={button({ variant: "danger" })} onClick={() => body && remove.mutate(body, { onSuccess: onClose })}>
          <Icon name="trash" className="size-4" />
          {t("activity.prune.confirm")}
        </button>
      </div>
    </Sheet>
  );
}

/** What the console keeps of each call: arguments on by default, result excerpts off, for how long (off, 7, 30 or 90 days, or for ever; 30 by default), and a way to delete a period or everything behind a confirmation that counts what goes, each shown only to a viewer the server lets change it. */
export function ActivitySettings() {
  const t = useT();
  const viewer = useViewer();
  const settings = useQuery(activitySettingsQuery);
  const [deleting, setDeleting] = useState<"all" | "period">();
  const save = useMutation(saveRecording);
  const current: Recording | undefined = save.isPending ? save.variables : settings.data;
  return (
    <section aria-labelledby="activity-settings" className={`${SURFACE} p-5`}>
      <h2 id="activity-settings" className={TITLE}>
        {t("activity.settings.title")}
      </h2>
      {!viewer.can("activity.settings") ? null : current ? (
        <div className="mt-4 flex flex-col gap-4">
          <Switch checked={current.record_arguments} hint={t("activity.settings.argumentsHint")} onChange={(on) => save.mutate({ ...current, record_arguments: on })}>
            {t("activity.settings.arguments")}
          </Switch>
          <Switch checked={current.record_results} hint={t("activity.settings.resultsHint")} onChange={(on) => save.mutate({ ...current, record_results: on })}>
            {t("activity.settings.results")}
          </Switch>
          <Select<Keep> label={t("activity.settings.keep")} value={settings.data?.retention ?? "30d"} options={KEEPS.map((keep) => ({ value: keep, text: t(`activity.keep.${keep}`) }))} onChange={(retention) => save.mutate({ ...current, retention })} className="w-full" />
          <p className={CAPTION}>{t(settings.data?.retention === "off" ? "activity.settings.keepOffNote" : settings.data?.retention === "forever" ? "activity.settings.keepForeverNote" : "activity.settings.keepNote")}</p>
        </div>
      ) : settings.isError ? (
        <p role="alert" className="mt-3 text-sm text-amber-400">
          {errorText(settings.error)}
        </p>
      ) : (
        <div aria-hidden="true" className={`${SKELETON} mt-4 h-28`} />
      )}
      <p role="alert" className="mt-2 text-sm text-amber-400 empty:hidden">
        {save.isError ? errorText(save.error) : ""}
      </p>
      {viewer.can("activity.delete") ? (
        <div className="mt-4 flex flex-wrap gap-2">
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => setDeleting("period")}>
            {t("activity.prune.open")}
          </button>
          <button type="button" className={button({ variant: "danger", size: "sm" })} onClick={() => setDeleting("all")}>
            <Icon name="trash" className="size-4" />
            {t("activity.settings.delete")}
          </button>
        </div>
      ) : null}
      {deleting === "all" ? <DeleteAll onClose={() => setDeleting(undefined)} /> : null}
      {deleting === "period" ? <PrunePeriod keptDays={keptDaysOf(settings.data?.retention)} onClose={() => setDeleting(undefined)} /> : null}
    </section>
  );
}
