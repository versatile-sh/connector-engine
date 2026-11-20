import type { ConnectorCount } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { rowMotion } from "../ui/kept";
import { LongList } from "../ui/LongList";
import { SURFACE, TITLE } from "../ui/styles";
import type { Named } from "./activity";

const ROW = "relative flex w-full items-center gap-3 py-2 pr-1 pl-4 text-left text-muted transition-colors duration-200 hover:text-paper aria-pressed:text-paper disabled:cursor-default disabled:hover:text-muted";

const MARK = "absolute top-1/2 left-0 -mt-2 h-4 w-0.5 rounded-full bg-lime transition-[opacity,scale] duration-250 ease-(--ease-settle)";

const COUNT = "font-mono text-xs tabular-nums transition-colors duration-200";

const keyOf = (count: ConnectorCount) => count.connector ?? `alias:${count.alias ?? ""}`;

type ConnectorTotalsProps = { counts: readonly ConnectorCount[]; named: ReadonlyMap<string, Named>; selected: string | undefined; onSelect(connector: string | undefined): void };

/** Calls per connector or skill, busiest first, as the site's directory filter: quiet rows whose chosen one gets the short lime mark and a lime count, the first few shown with a filter and Show all past them; pressing one narrows the page to it, and All widens it again. */
export function ConnectorTotals({ counts, named, selected, onSelect }: ConnectorTotalsProps) {
  const t = useT();
  const total = counts.reduce((sum, count) => sum + count.total, 0);
  const all = selected === undefined;
  const titleOf = (count: ConnectorCount) => (count.connector ? (named.get(count.connector)?.title ?? count.alias ?? count.connector) : t("activity.connectors.unknown", { alias: count.alias || "?" }));
  return (
    <section aria-labelledby="by-connector" className={`${SURFACE} p-5`}>
      <h2 id="by-connector" className={TITLE}>
        {t("activity.connectors")}
      </h2>
      <ul className="mt-3">
        <li>
          <button type="button" aria-pressed={all} className={ROW} onClick={() => onSelect(undefined)}>
            <span aria-hidden="true" className={`${MARK} ${all ? "scale-y-100 opacity-100" : "scale-y-0 opacity-0"}`} />
            <span className="min-w-0 flex-1 truncate text-sm">{t("activity.connectors.all")}</span>
            <span className={`${COUNT} ${all ? "text-lime" : "text-muted-light"}`}>{total}</span>
          </button>
        </li>
      </ul>
      <LongList rows={counts} textOf={titleOf} keyOf={keyOf} label={t("connectors.filter")}>
        {(shown) => (
          <ul>
            {shown.map(({ row: count, phase }) => {
              const connector = count.connector ? named.get(count.connector) : undefined;
              const title = titleOf(count);
              const on = selected !== undefined && selected === count.connector;
              return (
                <li key={keyOf(count)} {...rowMotion(phase)}>
                  <button type="button" aria-pressed={on} disabled={!count.connector} className={ROW} onClick={() => onSelect(on ? undefined : count.connector)}>
                    <span aria-hidden="true" className={`${MARK} ${on ? "scale-y-100 opacity-100" : "scale-y-0 opacity-0"}`} />
                    <ConnectorIcon src={connector?.icon} title={title} size="xs" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm">{title}</span>
                      {count.skill || count.failed > 0 ? (
                        <span className="block font-mono text-[0.6875rem] text-muted">
                          {count.skill ? t("activity.skill") : null}
                          {count.skill && count.failed > 0 ? " · " : null}
                          {count.failed > 0 ? <span className="text-amber-400">{t("activity.connectors.failed", { count: count.failed })}</span> : null}
                        </span>
                      ) : null}
                    </span>
                    <span aria-hidden="true" className={`${COUNT} ${on ? "text-lime" : "text-muted-light"}`}>
                      {count.total}
                    </span>
                    <span className="sr-only">{t("activity.connectors.calls", { count: count.total })}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </LongList>
    </section>
  );
}
