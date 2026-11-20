import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText, type DeviceView } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { renameDevice } from "../mutations";
import { Bubble } from "../ui/Bubble";
import { FIELD, button } from "../ui/styles";
import { DEVICE_NAME_MAX } from "./device";
import { InlineConfirm } from "./InlineConfirm";
import type { useRemove } from "./useRemove";

function Rename({ device, onDone }: { device: DeviceView; onDone(): void }) {
  const t = useT();
  const [name, setName] = useState(device.name);
  const rename = useMutation(renameDevice);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    rename.mutate({ id: device.id, name: name.trim() }, { onSuccess: onDone });
  };
  return (
    <form
      onSubmit={onSubmit}
      onKeyDown={(event) => {
        if (event.key === "Escape") onDone();
      }}
      className="swap-in flex min-w-0 grow basis-full flex-wrap items-center gap-2 @md:basis-0"
    >
      <div className={`${FIELD} flex h-8 min-w-0 flex-1 basis-40 items-center`}>
        <input
          aria-label={t("notify.devices.newName", { name: device.name })}
          autoFocus
          onFocus={(event) => event.target.select()}
          value={name}
          maxLength={DEVICE_NAME_MAX}
          onChange={(event) => setName(event.target.value)}
          autoComplete="off"
          spellCheck={false}
          className="h-full min-w-0 flex-1 bg-transparent px-3 text-sm focus-visible:outline-none"
        />
      </div>
      <div className="flex shrink-0 gap-2">
        <button type="button" disabled={rename.isPending} className={button({ variant: "quiet", size: "xs" })} onClick={onDone}>
          {t("common.cancel")}
        </button>
        <button type="submit" disabled={rename.isPending} className={button({ variant: "secondary", size: "xs" })}>
          {t("common.save")}
        </button>
      </div>
      <p role="alert" className="w-full text-sm text-amber empty:hidden">
        {rename.isError ? errorText(rename.error) : ""}
      </p>
    </form>
  );
}

type DeviceRowProps = { device: DeviceView; here: boolean; remove: ReturnType<typeof useRemove> };

/** One paired computer: its name, renamed in place, a badge when this browser paired it, when its helper was last seen, and an inline Revoke. */
export function DeviceRow({ device, here, remove }: DeviceRowProps) {
  const t = useT();
  const locale = useLocale();
  const [step, setStep] = useState<"idle" | "renaming" | "renamed">("idle");
  const asking = remove.asking(device.id);
  const date = (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
  return (
    <li className={`border-b border-line py-3 transition-opacity duration-300 last:border-b-0 ${remove.busy && asking ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
        {step === "renaming" ? (
          <Rename device={device} onDone={() => setStep("renamed")} />
        ) : (
          <span className="flex min-w-0 grow basis-full items-center gap-2 @md:basis-0">
            <span title={device.name} className="min-w-0 truncate font-medium">
              {device.name}
            </span>
            {here ? <Bubble tone="accent">{t("notify.devices.thisComputer")}</Bubble> : null}
          </span>
        )}
        <span className="min-w-0 grow font-mono text-xs text-muted @md:grow-0">{device.last_seen ? t("notify.devices.seen", { when: date(device.last_seen) }) : t("notify.devices.never")}</span>
        {asking || step === "renaming" ? null : (
          <div className="flex shrink-0 gap-2">
            <button type="button" autoFocus={step === "renamed"} aria-label={t("notify.devices.renameLabel", { name: device.name })} className={button({ variant: "secondary", size: "xs" })} onClick={() => setStep("renaming")}>
              {t("notify.devices.rename")}
            </button>
            <button type="button" autoFocus={remove.returning(device.id)} aria-label={t("notify.devices.revokeLabel", { name: device.name })} className={button({ variant: "danger", size: "xs" })} onClick={() => remove.ask(device.id)}>
              {t("common.revoke")}
            </button>
          </div>
        )}
      </div>
      {asking ? (
        <InlineConfirm
          label={t("notify.devices.revokeLabel", { name: device.name })}
          question={t("notify.devices.revokeAsk", { name: device.name })}
          confirm={t("common.revoke")}
          working={t("notify.revoking")}
          busy={remove.busy}
          error={remove.error}
          onCancel={remove.cancel}
          onConfirm={remove.confirm}
        />
      ) : null}
    </li>
  );
}
