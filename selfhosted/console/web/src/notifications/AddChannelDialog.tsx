import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText, type ChannelKind } from "../api";
import { useT } from "../i18n/I18nProvider";
import { addChannel } from "../mutations";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { OPTION_CARD, button } from "../ui/styles";
import { SETUP, SETUPS, SLACK_MANIFEST, missingFields, settingsFor } from "./setup";

function SetupForm({ kind, onSaved }: { kind: ChannelKind; onSaved(): void }) {
  const t = useT();
  const setup = SETUP[kind];
  const [label, setLabel] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const save = useMutation(addChannel);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (missingFields(setup, values).length > 0) return;
    save.mutate({ kind, label: label.trim() || t(setup.title), settings: settingsFor(setup, values) }, { onSuccess: onSaved });
  };
  return (
    <form onSubmit={onSubmit} className="swap-in mt-5" aria-label={t(setup.title)}>
      {setup.interactive ? null : (
        <Notice live={false} className="mb-4">{t(kind === "ntfy" ? "notify.add.ntfyWarning" : "notify.add.notifyOnly")}</Notice>
      )}
      <ol className="flex flex-col gap-2.5">
        {setup.steps.map((step, index) => (
          <li key={step} className="flex gap-3 text-sm">
            <span aria-hidden="true" className="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong font-mono text-xs text-muted">
              {index + 1}
            </span>
            <span className="min-w-0 pt-0.5 text-pretty">{t(step)}</span>
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
            hint={field.optional ? t("common.optional") : field.secret ? t("notify.field.secretHint") : undefined}
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
      <p role="alert" className="min-h-5 text-sm text-amber-400">
        {save.isError ? errorText(save.error) : ""}
      </p>
      <button type="submit" disabled={save.isPending} className={`${button({ variant: "primary" })} mt-2 w-full`}>
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
        <p className="text-sm text-pretty text-muted">{t("notify.add.pick")}</p>
        <div role="group" aria-label={t("notify.add.kind")} className="mt-3 grid grid-cols-2 gap-2 @xl:grid-cols-3">
          {SETUPS.map((setup) => (
            <button key={setup.kind} type="button" aria-pressed={kind === setup.kind} className={OPTION_CARD} onClick={() => setKind(setup.kind)}>
              <span className="text-sm font-medium">{t(setup.title)}</span>
              <span className="font-mono text-xs text-muted">{t(setup.interactive ? "notify.add.buttons" : "notify.add.linkOnly")}</span>
            </button>
          ))}
        </div>
        {kind ? <SetupForm key={kind} kind={kind} onSaved={onClose} /> : null}
      </div>
    </Sheet>
  );
}
