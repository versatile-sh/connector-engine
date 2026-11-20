import type { SetupConnector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Notice } from "../ui/Notice";
import { Tag } from "../ui/Tag";
import { CAPTION, SUBTITLE } from "../ui/styles";
import { capsOf, hostOf } from "./setup";

/** A setup's connectors as the review shows them: each with its host, whether it is installed already, whether it reaches this machine, and every choice the file asks above what this console allows. */
export function SetupConnectors({ connectors }: { connectors: readonly SetupConnector[] }) {
  const t = useT();
  if (connectors.length === 0) return null;
  const local = connectors.filter((c) => c.local && !c.exists).map((c) => c.name);
  return (
    <section className="mt-5">
      <h3 className={SUBTITLE}>{t("backup.count.connectors")}</h3>
      {local.length > 0 ? (
        <Notice live={false} className="mt-2">
          {t("backup.local", { names: local.join(", ") })}
        </Notice>
      ) : null}
      <ul className="mt-2 flex flex-col gap-2.5 text-sm">
        {connectors.map((c) => {
          const host = hostOf(c.mcp ?? c.listing);
          const caps = c.exists ? [] : capsOf(c);
          return (
            <li key={c.name}>
              <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                <span className="min-w-0 font-mono [overflow-wrap:anywhere]">{c.name}</span>
                {host ? <span className={`${CAPTION} min-w-0 [overflow-wrap:anywhere]`}>{host}</span> : null}
                {c.exists ? <Tag>{t("backup.kept")}</Tag> : null}
                {c.local && !c.exists ? <Tag tone="amber" dot>{t("backup.localTag")}</Tag> : null}
                {c.new_keys_off && !c.exists ? <Tag>{t("backup.newKeysOff")}</Tag> : null}
              </div>
              {caps.length > 0 ? (
                <ul className="mt-1 flex flex-col gap-0.5 pl-4">
                  {caps.map((cap) => (
                    <li key={cap.name} className="text-[0.8125rem] text-muted">
                      <code className="font-mono [overflow-wrap:anywhere]" translate="no">
                        {cap.name}
                      </code>{" "}
                      {t("backup.cap", { asks: t(`access.${cap.asks}`), allows: t(`access.${cap.allows}`) })}
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          );
        })}
      </ul>
      <p className="mt-3 text-[0.8125rem] leading-snug text-pretty text-muted">{t("backup.capNote")}</p>
    </section>
  );
}
