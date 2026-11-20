import { useId, useState, type FormEvent } from "react";
import type { ActivityData, CallOutcome } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { SearchBox } from "../ui/SearchBox";
import { Segmented } from "../ui/Segmented";
import { CHIP, PILL, button } from "../ui/styles";
import { dayOf, isNarrowed, lastWeek, periodOf, RANGES, type ActivityFilter, type ActivityPeriod, type Named } from "./activity";

const FILTER_OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending"];

const KEPT_DAYS = 30;

const APP_MENU_FROM = 10;

type Option<T extends string> = { value: T; text: string };

function Choice<T extends string>({ label, all, value, options, onChange }: { label: string; all: string; value: T | undefined; options: readonly Option<T>[]; onChange(value: T | undefined): void }) {
  return (
    <select aria-label={label} value={value ?? ""} onChange={(event) => onChange(options.find((option) => option.value === event.target.value)?.value)} className={`${PILL} max-w-48 pr-2`}>
      <option value="">{all}</option>
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.text}
        </option>
      ))}
    </select>
  );
}

function useEdited(value: string | undefined) {
  const [text, setText] = useState(value ?? "");
  const [shown, setShown] = useState(value);
  if (shown !== value) {
    setShown(value);
    setText(value ?? "");
  }
  return [text, setText] as const;
}

function Search({ value, onSearch }: { value: string | undefined; onSearch(search: string | undefined): void }) {
  const t = useT();
  const [text, setText] = useEdited(value);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    onSearch(text.trim() || undefined);
  };
  return (
    <form role="search" onSubmit={onSubmit}>
      <SearchBox
        label={t("activity.search")}
        value={text}
        onChange={(event) => {
          setText(event.target.value);
          if (!event.target.value && value) onSearch(undefined);
        }}
        className="w-56"
      />
    </form>
  );
}

function AppSearch({ apps, value, onChange }: { apps: readonly string[]; value: string | undefined; onChange(agent: string | undefined): void }) {
  const t = useT();
  const list = useId();
  const [text, setText] = useEdited(value);
  return (
    <>
      <SearchBox
        label={t("activity.app.all")}
        list={list}
        value={text}
        onChange={(event) => {
          const next = event.target.value;
          setText(next);
          if (!next) onChange(undefined);
          else if (apps.includes(next)) onChange(next);
        }}
        className="w-48"
      />
      <datalist id={list}>
        {apps.map((app) => (
          <option key={app} value={app} />
        ))}
      </datalist>
    </>
  );
}

type FilterRowProps = { filter: ActivityFilter; data: ActivityData | undefined; named: ReadonlyMap<string, Named>; onChange(filter: ActivityFilter): void };

/** The page's filters in one row: the range or custom days, the connector as a chip with its icon once chosen, the app (a searchable box past ten apps), the outcome, a search by function name, and Clear filters while anything narrows the page. */
export function FilterRow({ filter, data, named, onChange }: FilterRowProps) {
  const t = useT();
  const now = new Date();
  const titleOf = (id: string) => named.get(id)?.title ?? data?.connectors.find((count) => count.connector === id)?.alias ?? id;
  const connectors = (data?.connectors ?? []).flatMap((count) => (count.connector ? [{ value: count.connector, text: titleOf(count.connector) }] : []));
  const period = (next: ActivityPeriod) => onChange({ ...filter, ...next });
  return (
    <div role="group" aria-label={t("activity.filters")} className="mb-6 flex flex-wrap items-center gap-2">
      <Segmented
        label={t("activity.range")}
        size="sm"
        value={filter.range}
        onChange={(range) => period(range === "custom" ? lastWeek(now) : { range })}
        options={[...RANGES.map((range) => ({ value: range.value, text: t(`activity.range.${range.value}`) })), { value: "custom" as const, text: t("activity.range.custom") }]}
      />
      {filter.range === "custom" ? (
        <span className="swap-in flex items-center gap-1.5">
          <input type="date" aria-label={t("activity.from")} required min={dayOf(new Date(now.getFullYear(), now.getMonth(), now.getDate() - KEPT_DAYS))} max={filter.to} value={filter.from} onChange={(event) => event.target.value && period({ ...filter, from: event.target.value })} className={PILL} />
          <span aria-hidden="true" className="text-muted">
            –
          </span>
          <input type="date" aria-label={t("activity.to")} required min={filter.from} max={dayOf(now)} value={filter.to} onChange={(event) => event.target.value && period({ ...filter, to: event.target.value })} className={PILL} />
        </span>
      ) : null}
      {filter.connector ? (
        <button type="button" aria-pressed="true" title={t("activity.connector.clear", { name: titleOf(filter.connector) })} className={`${CHIP} swap-in h-8 gap-1.5 pr-2.5 pl-1.5 text-xs`} onClick={() => onChange({ ...filter, connector: undefined })}>
          <ConnectorIcon src={named.get(filter.connector)?.icon} title={titleOf(filter.connector)} size="xs" />
          <span className="max-w-40 truncate font-semibold">{titleOf(filter.connector)}</span>
          <Icon name="close" className="size-3.5 text-muted" />
        </button>
      ) : connectors.length > 0 ? (
        <Choice label={t("activity.connector")} all={t("activity.connectors.all")} value={undefined} options={connectors} onChange={(connector) => onChange({ ...filter, connector })} />
      ) : null}
      {data && (data.agents.length > 1 || filter.agent) ? (
        data.agents.length > APP_MENU_FROM ? (
          <AppSearch apps={data.agents} value={filter.agent} onChange={(agent) => onChange({ ...filter, agent })} />
        ) : (
          <Choice label={t("common.app")} all={t("activity.app.all")} value={filter.agent} options={data.agents.map((agent) => ({ value: agent, text: agent }))} onChange={(agent) => onChange({ ...filter, agent })} />
        )
      ) : null}
      <Choice<CallOutcome> label={t("activity.outcome")} all={t("activity.outcome.all")} value={filter.outcome} options={FILTER_OUTCOMES.map((outcome) => ({ value: outcome, text: t(`outcome.${outcome}`) }))} onChange={(outcome) => onChange({ ...filter, outcome })} />
      <Search value={filter.search} onSearch={(search) => onChange({ ...filter, search })} />
      {isNarrowed(filter) ? (
        <button type="button" className={`${button({ variant: "quiet", size: "xs" })} swap-in`} onClick={() => onChange(periodOf(filter))}>
          {t("activity.clearFilters")}
        </button>
      ) : null}
    </div>
  );
}
