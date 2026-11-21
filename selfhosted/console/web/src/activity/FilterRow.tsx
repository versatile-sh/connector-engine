import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import type { ActivityData, CallOutcome } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import type { Locale } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { useViewer } from "../people/useViewer";
import { peopleQuery } from "../queries";
import { Icon } from "../ui/Icon";
import { SearchBox } from "../ui/SearchBox";
import { Select, type SelectOption } from "../ui/Select";
import { CHIP, SURFACE, button } from "../ui/styles";
import { dayOf, daysBetween, inScope, isNarrowed, keptSince, lastWeek, clearedOf, RANGES, type ActivityFilter, type ActivityPeriod, type Named } from "./activity";

const FILTER_OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending"];

const dayText = (day: string, locale: Locale) => {
  const [year = 0, month = 1, date = 1] = day.split("-").map(Number);
  return new Intl.DateTimeFormat(locale, { weekday: "short", day: "numeric", month: "short" }).format(new Date(year, month - 1, date));
};

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

type FilterRowProps = { filter: ActivityFilter; data: ActivityData | undefined; named: ReadonlyMap<string, Named>; scoped?: boolean; onChange(filter: ActivityFilter): void };

function usePeopleOptions(data: ActivityData | undefined, everyone: boolean): SelectOption<string>[] {
  const viewer = useViewer();
  const people = useQuery({ ...peopleQuery, enabled: everyone && viewer.can("people.read") });
  if (!everyone) return [];
  if (people.data) return people.data.people.map((person) => ({ value: person.person, text: person.username }));
  const seen = new Map((data?.calls ?? []).flatMap((call) => (call.owner ? [[call.owner.person, call.owner.username || call.owner.person] as const] : [])));
  return [...seen].map(([value, text]) => ({ value, text }));
}

/** The page's filters in one wrapping row in a card of its own, as wide as the page's columns: the range as chips or custom days picked from the kept ones with two `Select`s, the connector, the access key and the outcome as selects that filter past eight options, the connector turning into a chip with its icon once chosen, a search by function name, and Clear filters while anything narrows the page; when `scoped`, Mine or Everyone leads the row, and Everyone adds a person. */
export function FilterRow({ filter, data, named, scoped = false, onChange }: FilterRowProps) {
  const t = useT();
  const locale = useLocale();
  const everyone = scoped && filter.scope === "everyone";
  const people = usePeopleOptions(data, everyone);
  const now = new Date();
  const titleOf = (id: string) => named.get(id)?.title ?? data?.connectors.find((count) => count.connector === id)?.alias ?? id;
  const connectors = (data?.connectors ?? []).flatMap((count) => (count.connector ? [{ value: count.connector, text: titleOf(count.connector), icon: <ConnectorIcon src={named.get(count.connector)?.icon} title={titleOf(count.connector)} size="xs" /> }] : []));
  const period = (next: ActivityPeriod) => onChange({ ...filter, ...next });
  const dayOption = (day: string) => ({ value: day, text: dayText(day, locale) });
  return (
    <div role="group" aria-label={t("activity.filters")} className={`${SURFACE} mb-6 flex flex-wrap items-center gap-2 p-3 sm:p-4`}>
      {scoped ? <ScopeSwitch scope={everyone ? "everyone" : "mine"} onChange={(scope) => onChange(inScope(filter, scope))} /> : null}
      <div role="group" aria-label={t("activity.range")} className="flex flex-wrap gap-2">
        {[...RANGES.map((range) => range.value), "custom" as const].map((range) => (
          <button key={range} type="button" aria-pressed={filter.range === range} className={`${CHIP} h-9 px-3.5 text-sm`} onClick={() => filter.range !== range && period(range === "custom" ? lastWeek(now) : { range })}>
            {t(`activity.range.${range}`)}
          </button>
        ))}
      </div>
      {filter.range === "custom" ? (
        <span className="swap-in flex items-center gap-1.5">
          <Select<string> label={t("activity.from")} value={filter.from} options={daysBetween(keptSince(now), filter.to).map(dayOption)} onChange={(from) => period({ ...filter, from })} className="w-40" />
          <span aria-hidden="true" className="text-muted">
            –
          </span>
          <Select<string> label={t("activity.to")} value={filter.to} options={daysBetween(filter.from, dayOf(now)).map(dayOption)} onChange={(to) => period({ ...filter, to })} className="w-40" />
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
      {everyone && people.length > 0 ? <Choice label={t("activity.person")} all={t("activity.person.all")} value={filter.person} options={people} onChange={(person) => onChange({ ...filter, person })} /> : null}
      <Choice<CallOutcome> label={t("activity.outcome")} all={t("activity.outcome.all")} value={filter.outcome} options={FILTER_OUTCOMES.map((outcome) => ({ value: outcome, text: t(`outcome.${outcome}`) }))} onChange={(outcome) => onChange({ ...filter, outcome })} />
      <Search value={filter.search} onSearch={(search) => onChange({ ...filter, search })} />
      {isNarrowed(filter) ? (
        <button type="button" className={`${button({ variant: "quiet", size: "xs" })} swap-in`} onClick={() => onChange(clearedOf(filter))}>
          {t("activity.clearFilters")}
        </button>
      ) : null}
    </div>
  );
}
