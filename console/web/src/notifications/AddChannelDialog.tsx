import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorText, type ChannelKind, type ChannelView } from "../api";
import { useT } from "../i18n/I18nProvider";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { Sheet } from "../ui/Sheet";
import { PRESS, button } from "../ui/styles";
import { Warning } from "../approve/Warning";
import { SETUP, SETUPS, SLACK_MANIFEST, missingFields, settingsFor } from "./setup";

const OPTION = `bubble flex flex-col items-start gap-0.5 border border-line-strong px-3.5 py-2.5 text-left hover:border-paper/50 aria-pressed:border-lime/60 aria-pressed:bg-lime/5 ${PRESS}`;

function SetupForm({ kind, onSaved }: { kind: ChannelKind; onSaved(): void }) {
  const t = useT();
  const queryClient = useQueryClient();
  const setup = SETUP[kind];
  const [label, setLabel] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const save = useMutation({
    mutationFn: () => api<ChannelView>("POST", "/api/notifications/channels", { kind, label: label.trim() || t(setup.title), settings: settingsFor(setup, values) }),
    onSuccess: () => {
      save.reset();
      void queryClient.invalidateQueries({ queryKey: ["notifications"] });
      onSaved();
    },
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (missingFields(setup, values).length === 0) save.mutate();
  };
  return (
    <form onSubmit={onSubmit} className="swap-in mt-5" aria-label={t(setup.title)}>
      {setup.interactive ? null : (
        <Warning className="mb-4">{t(kind === "ntfy" ? "notify.add.ntfyWarning" : "notify.add.notifyOnly")}</Warning>
      )}
      <ol className="flex flex-col gap-2.5">
        {setup.steps.map((step, index) => (
          <li key={step} className="flex gap-3 text-sm">
            <span aria-hidden="true" className="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong font-mono text-xs text-muted">
              {index + 1}
            </span>
            <span className="min-w-0 pt-0.5 [overflow-wrap:anywhere]">{t(step)}</span>
          </li>
        ))}
      </ol>
      {kind === "slack" ? <CopyBox label={t("notify.add.manifest")} text={SLACK_MANIFEST} className="mt-4" /> : null}
      <div className="mt-5 border-t border-line pt-4">
        <Field label={t("notify.field.label")} hint={t("notify.field.labelHint")} placeholder={t(setup.title)} value={label} onChange={(event) => setLabel(event.target.value)} autoComplete="off" />
        {setup.fields.map((field) => (
          <Field
            key={field.name}
            label={t(field.label)}
            hint={field.optional ? t("notify.field.optional") : field.secret ? t("notify.field.secretHint") : undefined}
            type={field.secret ? "password" : "text"}
            autoComplete="off"
            spellCheck={false}
            mono
            placeholder={field.placeholder}
            required={!field.optional}
            value={values[field.name] ?? ""}
            onChange={(event) => setValues({ ...values, [field.name]: event.target.value })}
          />
        ))}
      </div>
      <p role="alert" className="min-h-5 text-sm text-amber">
        {save.isError ? errorText(save.error) : ""}
      </p>
      <button type="submit" disabled={save.isPending} className={`${button({ variant: "accent" })} mt-2 w-full`}>
        {save.isPending ? t("notify.add.saving") : t("notify.add.save")}
      </button>
    </form>
  );
}

/** The centered window that adds a channel: pick a kind, follow its steps outside the console, paste the values back; secrets are never shown again. */
export function AddChannelDialog({ onClose }: { onClose(): void }) {
  const t = useT();
  const [kind, setKind] = useState<ChannelKind | undefined>();
  return (
    <Sheet title={t("notify.add.title")} closeLabel={t("common.close")} onClose={onClose} wide>
      <div className="@container">
        <p className="text-sm text-muted">{t("notify.add.pick")}</p>
        <div role="group" aria-label={t("notify.add.kind")} className="mt-3 grid grid-cols-2 gap-2 @xl:grid-cols-3">
          {SETUPS.map((setup) => (
            <button key={setup.kind} type="button" aria-pressed={kind === setup.kind} className={OPTION} onClick={() => setKind(setup.kind)}>
              <span className="text-sm font-semibold [overflow-wrap:anywhere]">{t(setup.title)}</span>
              <span className="font-mono text-xs text-muted">{t(setup.interactive ? "notify.add.buttons" : "notify.add.linkOnly")}</span>
            </button>
          ))}
        </div>
        {kind ? <SetupForm key={kind} kind={kind} onSaved={onClose} /> : null}
      </div>
    </Sheet>
  );
}
