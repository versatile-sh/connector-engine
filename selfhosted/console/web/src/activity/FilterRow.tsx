import { useState, type FormEvent } from "react";
import type { ActivityData, CallOutcome } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { SearchBox } from "../ui/SearchBox";
import { Select, type SelectOption } from "../ui/Select";
import { CHIP, CONTROL, button } from "../ui/styles";
import { dayOf, isNarrowed, lastWeek, periodOf, RANGES, type ActivityFilter, type ActivityPeriod, type Named } from "./activity";

const FILTER_OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending"];

const KEPT_DAYS = 30;

function Choice<T extends string>({ label, all, value, options, onChange }: { label: string; all: string; value: T | undefined; options: readonly SelectOption<T>[]; onChange(value: T | undefined): void }) {
  return <Select<T | ""> label={label} value={value ?? ""} options={[{ value: "", text: all }, ...options]} onChange={(next) => onChange(next === "" ? undefined : next)} className="max-w-48" />;
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

type FilterRowProps = { filter: ActivityFilter; data: ActivityData | undefined; named: ReadonlyMap<string, Named>; onChange(filter: ActivityFilter): void };

/** The page's filters in one wrapping row: the range as chips or custom days, the connector, the access key and the outcome as selects that filter past eight options, the connector turning into a chip with its icon once chosen, a search by function name, and Clear filters while anything narrows the page. */
export function FilterRow({ filter, data, named, onChange }: FilterRowProps) {
  const t = useT();
  const now = new Date();
  const titleOf = (id: string) => named.get(id)?.title ?? data?.connectors.find((count) => count.connector === id)?.alias ?? id;
  const connectors = (data?.connectors ?? []).flatMap((count) => (count.connector ? [{ value: count.connector, text: titleOf(count.connector), icon: <ConnectorIcon src={named.get(count.connector)?.icon} title={titleOf(count.connector)} size="xs" /> }] : []));
  const period = (next: ActivityPeriod) => onChange({ ...filter, ...next });
  return (
    <div role="group" aria-label={t("activity.filters")} className="mb-8 flex flex-wrap items-center gap-2">
      <div role="group" aria-label={t("activity.range")} className="flex flex-wrap gap-2">
        {[...RANGES.map((range) => range.value), "custom" as const].map((range) => (
          <button key={range} type="button" aria-pressed={filter.range === range} className={`${CHIP} h-9 px-3.5 text-sm`} onClick={() => filter.range !== range && period(range === "custom" ? lastWeek(now) : { range })}>
            {t(`activity.range.${range}`)}
          </button>
        ))}
      </div>
      {filter.range === "custom" ? (
        <span className="swap-in flex items-center gap-1.5">
          <input type="date" aria-label={t("activity.from")} required min={dayOf(new Date(now.getFullYear(), now.getMonth(), now.getDate() - KEPT_DAYS))} max={filter.to} value={filter.from} onChange={(event) => event.target.value && period({ ...filter, from: event.target.value })} className={CONTROL} />
          <span aria-hidden="true" className="text-muted">
            –
          </span>
          <input type="date" aria-label={t("activity.to")} required min={filter.from} max={dayOf(now)} value={filter.to} onChange={(event) => event.target.value && period({ ...filter, to: event.target.value })} className={CONTROL} />
        </span>
      ) : null}
      {filter.connector ? (
        <button type="button" aria-pressed="true" title={t("activity.connector.clear", { name: titleOf(filter.connector) })} className={`${CHIP} swap-in h-9 gap-2 pr-3 pl-2 text-sm`} onClick={() => onChange({ ...filter, connector: undefined })}>
          <ConnectorIcon src={named.get(filter.connector)?.icon} title={titleOf(filter.connector)} size="xs" />
          <span className="max-w-40 truncate">{titleOf(filter.connector)}</span>
          <Icon name="close" className="size-3.5 text-muted" />
        </button>
      ) : connectors.length > 0 ? (
        <Choice label={t("activity.connector")} all={t("activity.connectors.all")} value={undefined} options={connectors} onChange={(connector) => onChange({ ...filter, connector })} />
      ) : null}
      {data && (data.agents.length > 1 || filter.agent) ? (
        <Choice label={t("activity.key")} all={t("activity.app.all")} value={filter.agent} options={data.agents.map((agent) => ({ value: agent, text: agent }))} onChange={(agent) => onChange({ ...filter, agent })} />
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
