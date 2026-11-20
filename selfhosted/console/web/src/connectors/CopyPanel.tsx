import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type Connector, type Profile } from "../api";
import { useT } from "../i18n/I18nProvider";
import { copyProfile } from "../mutations";
import { Checkbox } from "../ui/Checkbox";
import { InlineConfirm } from "../ui/InlineConfirm";
import { rowMotion } from "../ui/kept";
import { LongList } from "../ui/LongList";
import { Select } from "../ui/Select";
import { CAPTION, WELL, button } from "../ui/styles";
import { overrideCount } from "./profiles";
import { toggledOne } from "./selection";

type CopyPanelProps = { connector: Connector; target?: Profile | undefined; onClose(): void };

/** Copies one key's profile on a connector, its mode and overrides, in one request: to the key shown from another key, or from the default to the keys picked; it asks first when the copy drops overrides. */
export function CopyPanel({ connector, target, onClose }: CopyPanelProps) {
  const t = useT();
  const copy = useMutation(copyProfile);
  const sources = connector.profiles.filter((p) => p.agent !== target?.agent);
  const [from, setFrom] = useState(sources[0]?.agent);
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [asking, setAsking] = useState(false);
  const targets = target ? [target.agent] : [...picked].filter((name) => name !== from);
  const losing = connector.profiles.filter((p) => targets.includes(p.agent) && overrideCount(p) > 0).map((p) => p.agent);
  const kept = copy.data?.keys.filter((key) => key.error) ?? [];
  const run = () => {
    if (!from) return;
    copy.mutate({ connector: connector.id, from, keys: targets }, { onSuccess: (data) => (data.keys.some((key) => key.error) ? setAsking(false) : onClose()) });
  };
  const title = target ? t("copy.titleKey", { key: target.agent }) : t("copy.titleMany");
  return (
    <section aria-label={title} className={`${WELL} swap-in px-4 py-3 sm:px-5`}>
      <p className="text-sm font-medium">{title}</p>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <span className={CAPTION}>{t("copy.from")}</span>
        <Select<string> label={t("copy.from")} value={from} options={sources.map((p) => ({ value: p.agent, text: p.agent }))} onChange={setFrom} className="max-w-56" />
      </div>
      {target ? null : (
        <div className="mt-3">
          <p className={CAPTION}>{t("copy.to")}</p>
          <LongList rows={connector.profiles.filter((p) => p.agent !== from)} textOf={(p) => p.agent} keyOf={(p) => p.agent} label={t("keys.filter")}>
            {(shown) => (
              <ul className="mt-1 flex flex-col">
                {shown.map(({ row: p, phase }) => (
                  <li key={p.agent} {...rowMotion(phase)}>
                    <Checkbox label={p.agent} visibleLabel checked={picked.has(p.agent)} onChange={() => setPicked(toggledOne(picked, p.agent))} />
                  </li>
                ))}
              </ul>
            )}
          </LongList>
        </div>
      )}
      {asking ? (
        <InlineConfirm
          label={title}
          question={t("copy.ask", { keys: losing.join(", ") })}
          confirm={t("copy.run")}
          working={t("copy.copying")}
          busy={copy.isPending}
          error={copy.isError ? errorText(copy.error) : undefined}
          onCancel={() => setAsking(false)}
          onConfirm={run}
        />
      ) : (
        <div className="mt-3 flex flex-wrap justify-end gap-2">
          <button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" disabled={!from || targets.length === 0 || copy.isPending} className={button({ variant: "secondary", size: "xs" })} onClick={() => (losing.length > 0 ? setAsking(true) : run())}>
            {copy.isPending ? t("copy.copying") : target ? t("copy.run") : t("copy.runMany", { count: targets.length })}
          </button>
        </div>
      )}
      <p role="alert" className="mt-2 text-sm text-amber-400 empty:hidden">
        {copy.isError && !asking ? errorText(copy.error) : kept.length > 0 ? t("copy.kept", { keys: kept.map((key) => key.agent).join(", ") }) : ""}
      </p>
    </section>
  );
}
