import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { deleteActivity, saveRecording, type Recording } from "../mutations";
import { activitySettingsQuery } from "../queries";
import { Icon } from "../ui/Icon";
import { Sheet } from "../ui/Sheet";
import { CAPTION, SKELETON, SURFACE, TITLE, button } from "../ui/styles";
import { Switch } from "../ui/Switch";

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

/** What the console keeps of each call: arguments on by default, result excerpts off, 30 days, and a way to delete it all behind a confirmation. */
export function ActivitySettings() {
  const t = useT();
  const settings = useQuery(activitySettingsQuery);
  const [deleting, setDeleting] = useState(false);
  const save = useMutation(saveRecording);
  const current: Recording | undefined = save.isPending ? save.variables : settings.data;
  return (
    <section aria-labelledby="activity-settings" className={`${SURFACE} p-5`}>
      <h2 id="activity-settings" className={TITLE}>
        {t("activity.settings.title")}
      </h2>
      {current ? (
        <div className="mt-4 flex flex-col gap-4">
          <Switch checked={current.record_arguments} hint={t("activity.settings.argumentsHint")} onChange={(on) => save.mutate({ ...current, record_arguments: on })}>
            {t("activity.settings.arguments")}
          </Switch>
          <Switch checked={current.record_results} hint={t("activity.settings.resultsHint")} onChange={(on) => save.mutate({ ...current, record_results: on })}>
            {t("activity.settings.results")}
          </Switch>
          <p className={CAPTION}>{t("activity.settings.retention", { days: settings.data?.retention_days ?? 30 })}</p>
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
      <button type="button" className={`${button({ variant: "danger", size: "sm" })} mt-4`} onClick={() => setDeleting(true)}>
        <Icon name="trash" className="size-4" />
        {t("activity.settings.delete")}
      </button>
      {deleting ? <DeleteAll onClose={() => setDeleting(false)} /> : null}
    </section>
  );
}
