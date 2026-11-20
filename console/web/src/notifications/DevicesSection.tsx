import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorText, type DeviceView, type PairedDevice } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { HAIRLINE, button } from "../ui/styles";
import { InlineConfirm } from "./InlineConfirm";
import { Section } from "./Section";
import { useRemove } from "./useRemove";

function Paired({ paired, onDone }: { paired: PairedDevice; onDone(): void }) {
  const t = useT();
  return (
    <div className="arrive bubble-sent relative mb-5 overflow-hidden border border-lime/50 bg-ink-sunken p-4">
      <span aria-hidden="true" className={HAIRLINE} />
      <p className="font-medium [overflow-wrap:anywhere]">{t("notify.devices.paired", { name: paired.device.name })}</p>
      <p className="mt-0.5 text-sm text-muted">{t("notify.devices.once")}</p>
      <CopyBox label={t("notify.devices.command")} text={paired.command} className="mt-3" />
      <CopyBox label={t("notify.devices.key")} text={paired.key} className="mt-3" />
      <p className="mt-3 text-[0.8125rem] leading-snug text-muted">{t("notify.devices.host")}</p>
      <button type="button" className={`${button({ variant: "secondary", size: "xs" })} mt-3`} onClick={onDone}>
        {t("notify.devices.done")}
      </button>
    </div>
  );
}

/** Desktop popups: pair this computer for `cnctl notify`, the key and command shown once, and every paired computer with an inline revoke. */
export function DevicesSection({ devices }: { devices: DeviceView[] }) {
  const t = useT();
  const locale = useLocale();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const pair = useMutation({
    mutationFn: () => api<PairedDevice>("POST", "/api/notifications/devices", { name: name.trim() }),
    onSuccess: () => {
      setName("");
      void queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
  });
  const remove = useRemove((id) => `/api/notifications/devices/${encodeURIComponent(id)}`);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    pair.mutate();
  };
  const date = (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
  return (
    <Section id="devices-title" icon="terminal" title={t("notify.devices.title")} intro={t("notify.devices.intro")}>
      {pair.data ? <Paired key={pair.data.key} paired={pair.data} onDone={() => pair.reset()} /> : null}
      <form onSubmit={onSubmit} className="grid gap-x-3 @md:grid-cols-[minmax(0,1fr)_auto] @md:items-start">
        <Field
          label={t("notify.devices.name")}
          hint={t("notify.devices.nameHint")}
          className="min-w-0"
          value={name}
          onChange={(event) => setName(event.target.value)}
          required
          autoComplete="off"
          spellCheck={false}
          error={pair.isError ? errorText(pair.error) : undefined}
        />
        <button type="submit" disabled={pair.isPending} className={`${button({ variant: "accent" })} mb-4 @md:mt-7 @md:mb-0`}>
          {pair.isPending ? t("notify.devices.pairing") : t("notify.devices.pair")}
        </button>
      </form>
      {devices.length === 0 ? (
        <p className="text-sm text-muted">{t("notify.devices.empty")}</p>
      ) : (
        <ul>
          {devices.map((device) => (
            <li key={device.id} className={`border-b border-line py-3 transition-opacity duration-300 last:border-b-0 ${remove.busy && remove.asking(device.id) ? "opacity-60" : ""}`}>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                <span title={device.name} className="min-w-0 grow basis-full truncate font-medium @md:basis-0">{device.name}</span>
                <span className="min-w-0 grow font-mono text-xs text-muted @md:grow-0">{device.last_seen ? t("notify.devices.seen", { when: date(device.last_seen) }) : t("notify.devices.never")}</span>
                {remove.asking(device.id) ? null : (
                  <button type="button" autoFocus={remove.returning(device.id)} aria-label={t("notify.devices.revokeLabel", { name: device.name })} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.ask(device.id)}>
                    {t("notify.revoke")}
                  </button>
                )}
              </div>
              {remove.asking(device.id) ? (
                <InlineConfirm
                  label={t("notify.devices.revokeLabel", { name: device.name })}
                  question={t("notify.devices.revokeAsk", { name: device.name })}
                  confirm={t("notify.revoke")}
                  working={t("notify.revoking")}
                  busy={remove.busy}
                  error={remove.error}
                  onCancel={remove.cancel}
                  onConfirm={remove.confirm}
                />
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}
