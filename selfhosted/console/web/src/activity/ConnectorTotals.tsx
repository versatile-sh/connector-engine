import type { ConnectorCount } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { CAPTION, PRESS, SURFACE } from "../ui/styles";
import type { Named } from "./activity";

const ROW = `bubble-word flex w-full items-center gap-3 border border-transparent px-2.5 py-2 text-left hover:bg-paper/5 aria-pressed:border-line-strong aria-pressed:bg-paper/8 disabled:cursor-default disabled:hover:bg-transparent ${PRESS}`;

type ConnectorTotalsProps = { counts: readonly ConnectorCount[]; named: ReadonlyMap<string, Named>; selected: string | undefined; onSelect(connector: string | undefined): void };

/** Calls per connector or skill with its icon, busiest first; pressing one narrows the page to it, and All widens it again. */
export function ConnectorTotals({ counts, named, selected, onSelect }: ConnectorTotalsProps) {
  const t = useT();
  const total = counts.reduce((sum, count) => sum + count.total, 0);
  return (
    <section aria-labelledby="by-connector" className={`${SURFACE} p-4 sm:p-5`}>
      <h2 id="by-connector" className="px-1 text-lg leading-snug font-medium">
        {t("activity.connectors")}
      </h2>
      <ul className="mt-3 flex flex-col gap-1">
        <li>
          <button type="button" aria-pressed={selected === undefined} className={ROW} onClick={() => onSelect(undefined)}>
            <span className="min-w-0 flex-1 truncate text-sm font-semibold">{t("activity.connectors.all")}</span>
            <span className={`${CAPTION} tabular-nums`}>{total}</span>
          </button>
        </li>
        {counts.map((count) => {
          const connector = count.connector ? named.get(count.connector) : undefined;
          const title = connector?.title ?? (count.connector ? (count.alias ?? count.connector) : t("activity.connectors.unknown", { alias: count.alias || "?" }));
          return (
            <li key={count.connector ?? `alias:${count.alias ?? ""}`} className="arrive">
              <button type="button" aria-pressed={selected !== undefined && selected === count.connector} disabled={!count.connector} className={ROW} onClick={() => onSelect(count.connector === selected ? undefined : count.connector)}>
                <ConnectorIcon src={connector?.icon} title={title} size="sm" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-semibold">{title}</span>
                  <span className={`${CAPTION} block`}>
                    {count.skill ? `${t("activity.skill")} · ` : null}
                    {t("activity.connectors.calls", { count: count.total })}
                    {count.failed > 0 ? <span className="text-amber"> · {t("activity.connectors.failed", { count: count.failed })}</span> : null}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
