import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type DeviceView, type PairedDevice } from "../api";
import { useT } from "../i18n/I18nProvider";
import { pairDevice, revokeDevice } from "../mutations";
import { CopyBox } from "../ui/CopyBox";
import { Icon } from "../ui/Icon";
import { LongList } from "../ui/LongList";
import { HAIRLINE, WELL, button } from "../ui/styles";
import { browserFacts, deviceName, pairedHere, rememberPairedHere } from "./device";
import { DeviceRow } from "./DeviceRow";
import { Section } from "./Section";
import { useRemove } from "./useRemove";

function Paired({ paired, onDone }: { paired: PairedDevice; onDone(): void }) {
  const t = useT();
  return (
    <div className={`${WELL} arrive relative mb-5 overflow-hidden p-4 sm:p-5`}>
      <span aria-hidden="true" className={HAIRLINE} />
      <p className="font-medium [overflow-wrap:anywhere]">{t("notify.devices.paired", { name: paired.device.name })}</p>
      <p className="mt-0.5 text-sm text-muted">{t("notify.devices.once")}</p>
      <CopyBox label={t("notify.devices.command")} text={paired.command} className="mt-3" />
      <CopyBox label={t("notify.devices.key")} text={paired.key} className="mt-3" />
      <p className="mt-3 text-[0.8125rem] leading-snug text-pretty text-muted">{t("notify.devices.host")}</p>
      <button type="button" className={`${button({ variant: "secondary", size: "xs" })} mt-3`} onClick={onDone}>
        {t("notify.devices.done")}
      </button>
    </div>
  );
}

/** Desktop popups: pair this computer for `cnctl notify` in one click, named after its system and browser, the key and command shown once, and every paired computer, the one paired here marked, renamed in place or revoked inline. */
export function DevicesSection({ devices }: { devices: DeviceView[] }) {
  const t = useT();
  const pair = useMutation(pairDevice);
  const remove = useRemove(revokeDevice);
  const [here, setHere] = useState(pairedHere);
  const onPair = () => pair.mutate(deviceName(browserFacts(navigator)), { onSuccess: (paired) => setHere(rememberPairedHere(paired.device.id)) });
  return (
    <Section
      id="devices-title"
      icon="terminal"
      title={t("notify.devices.title")}
      intro={t("notify.devices.intro")}
      action={
        <button type="button" disabled={pair.isPending} className={button({ variant: "primary", size: "sm" })} onClick={onPair}>
          <Icon name="plus" className="size-4" />
          {pair.isPending ? t("notify.devices.pairing") : t("notify.devices.pair")}
        </button>
      }
    >
      <p role="alert" className="mb-4 text-sm text-amber-400 empty:hidden">
        {pair.isError ? errorText(pair.error) : ""}
      </p>
      {pair.data ? <Paired key={pair.data.key} paired={pair.data} onDone={() => pair.reset()} /> : null}
      {devices.length === 0 ? (
        <p className="text-sm text-muted">{t("notify.devices.empty")}</p>
      ) : (
        <LongList rows={devices} textOf={(device) => device.name} label={t("notify.devices.filter")}>
          {(shown) => (
            <ul className={WELL}>
              {shown.map((device) => (
                <DeviceRow key={device.id} device={device} here={device.id === here} remove={remove} />
              ))}
            </ul>
          )}
        </LongList>
      )}
    </Section>
  );
}
