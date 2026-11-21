import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import type { CallOutcome } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import type { Locale } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { useViewer } from "../people/useViewer";
import { peopleQuery } from "../queries";
import { SearchBox } from "../ui/SearchBox";
import { Select, type SelectOption } from "../ui/Select";
import { CHIP, button } from "../ui/styles";
import { dayOf, daysBetween, inScope, isNarrowed, keptSince, lastWeek, clearedOf, RANGES, type ActivityFilter, type ActivityPeriod, type ActivityView, type Named } from "./activity";

const FILTER_OUTCOMES: readonly CallOutcome[] = ["ok", "error", "denied", "pending"];

const dayText = (day: string, locale: Locale) => {
  const [year = 0, month = 1, date = 1] = day.split("-").map(Number);
  return new Intl.DateTimeFormat(locale, { weekday: "short", day: "numeric", month: "short" }).format(new Date(year, month - 1, date));
};

const WIDTH = "w-full @2xl:w-40 @2xl:shrink-0";

function Choice<T extends string>({ label, all, value, options, onChange, className = WIDTH }: { label: string; all: string; value: T | undefined; options: readonly SelectOption<T>[]; onChange(value: T | undefined): void; className?: string }) {
  return <Select<T | ""> label={label} value={value ?? ""} options={[{ value: "", text: all }, ...options]} onChange={(next) => onChange(next === "" ? undefined : next)} disabled={options.length === 0 && value === undefined} className={className} />;
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
    <form role="search" onSubmit={onSubmit} className="min-w-0 flex-1">
      <SearchBox
        label={t("activity.search")}
        value={text}
        onChange={(event) => {
          setText(event.target.value);
          if (!event.target.value && value) onSearch(undefined);
        }}
        className="w-full"
      />
    </form>
  );
}

type FilterRowProps = { filter: ActivityFilter; data: ActivityView | undefined; named: ReadonlyMap<string, Named>; scoped?: boolean; keptDays: number; onChange(filter: ActivityFilter): void };

function usePeopleOptions(data: ActivityView | undefined, everyone: boolean): SelectOption<string>[] {
  const viewer = useViewer();
  const people = useQuery({ ...peopleQuery, enabled: everyone && viewer.can("people.read") });
  if (!everyone) return [];
  if (people.data) return people.data.people.map((person) => ({ value: person.person, text: person.username }));
  const seen = new Map((data?.calls ?? []).flatMap((call) => (call.owner ? [[call.owner.person, call.owner.username || call.owner.person] as const] : [])));
  return [...seen].map(([value, text]) => ({ value, text }));
}

/** The page's filters as a toolbar directly over its content that never reflows when a filter is set: the period (Mine or Everyone when `scoped`, the range as chips, custom days as two `Select`s) and the narrowing controls (connector, access key, person among everyone's, outcome, a search by function name and Clear filters) are always drawn, each in a box of one width whether chosen or not, so choosing a connector changes a label and nothing moves. Clear filters keeps its place, invisible and inert until something narrows the page. From a container of 64rem the two groups share one row that does not wrap, the search the one flexible item; below it, or whenever Everyone or custom days add controls, the period is the first row and the narrowing controls the second, wrapping into two columns on a phone. */
export function FilterRow({ filter, data, named, scoped = false, keptDays, onChange }: FilterRowProps) {
  const t = useT();
  const locale = useLocale();
  const everyone = scoped && filter.scope === "everyone";
  const people = usePeopleOptions(data, everyone);
  const now = new Date();
  const narrowed = isNarrowed(filter);
  const titleOf = (id: string) => named.get(id)?.title ?? data?.connectors.find((count) => count.connector === id)?.alias ?? id;
  const connectorOption = (id: string) => ({ value: id, text: titleOf(id), icon: <ConnectorIcon src={named.get(id)?.icon} title={titleOf(id)} size="xs" /> });
  const seen = (data?.connectors ?? []).flatMap((count) => (count.connector ? [count.connector] : []));
  const connectors = (filter.connector && !seen.includes(filter.connector) ? [...seen, filter.connector] : seen).map(connectorOption);
  const agents = data?.agents ?? [];
  const keys = filter.agent && !agents.includes(filter.agent) ? [...agents, filter.agent] : agents;
  const period = (next: ActivityPeriod) => onChange({ ...filter, ...next });
  const dayOption = (day: string) => ({ value: day, text: dayText(day, locale) });
  const roomy = !everyone && filter.range !== "custom";
  return (
    <div role="group" aria-label={t("activity.filters")} className={`@container flex flex-col gap-2 ${roomy ? "@5xl:flex-row @5xl:items-center" : ""}`}>
      <div className={`flex flex-wrap items-center gap-2 ${roomy ? "@5xl:shrink-0 @5xl:flex-nowrap" : ""}`}>
        {scoped ? <ScopeSwitch scope={everyone ? "everyone" : "mine"} onChange={(scope) => onChange(inScope(filter, scope))} /> : null}
        <div role="group" aria-label={t("activity.range")} className={`flex flex-wrap gap-2 ${roomy ? "@5xl:flex-nowrap" : ""}`}>
          {[...RANGES.map((range) => range.value), "custom" as const].map((range) => (
            <button key={range} type="button" aria-pressed={filter.range === range} className={`${CHIP} h-9 px-3.5 text-sm`} onClick={() => filter.range !== range && period(range === "custom" ? lastWeek(now) : { range })}>
              {t(`activity.range.${range}`)}
            </button>
          ))}
        </div>
        {filter.range === "custom" ? (
          <span className="swap-in flex items-center gap-1.5">
            <Select<string> label={t("activity.from")} value={filter.from} options={daysBetween(keptSince(now, keptDays), filter.to).map(dayOption)} onChange={(from) => period({ ...filter, from })} className="w-36" />
            <span aria-hidden="true" className="text-muted">
              –
            </span>
            <Select<string> label={t("activity.to")} value={filter.to} options={daysBetween(filter.from, dayOf(now)).map(dayOption)} onChange={(to) => period({ ...filter, to })} className="w-36" />
          </span>
        ) : null}
      </div>
      <div className={`grid grid-cols-2 gap-2 @2xl:flex @2xl:flex-wrap @2xl:items-center ${roomy ? "@5xl:min-w-0 @5xl:flex-1 @5xl:flex-nowrap" : ""}`}>
        <Choice label={t("activity.connector")} all={t("activity.connectors.all")} value={filter.connector} options={connectors} onChange={(connector) => onChange({ ...filter, connector })} className={`${WIDTH} @5xl:w-44`} />
        <Choice label={t("activity.key")} all={t("activity.app.all")} value={filter.agent} options={keys.map((agent) => ({ value: agent, text: agent }))} onChange={(agent) => onChange({ ...filter, agent })} />
        {everyone ? <Choice label={t("activity.person")} all={t("activity.person.all")} value={filter.person} options={people} onChange={(person) => onChange({ ...filter, person })} /> : null}
        <Choice<CallOutcome> label={t("activity.outcome")} all={t("activity.outcome.all")} value={filter.outcome} options={FILTER_OUTCOMES.map((outcome) => ({ value: outcome, text: t(`outcome.${outcome}`) }))} onChange={(outcome) => onChange({ ...filter, outcome })} />
        <div className="col-span-2 flex min-w-0 items-center gap-2 @2xl:min-w-48 @2xl:flex-1">
          <Search value={filter.search} onSearch={(search) => onChange({ ...filter, search })} />
          <button
            type="button"
            tabIndex={narrowed ? undefined : -1}
            aria-hidden={narrowed ? undefined : true}
            inert={!narrowed}
            className={`${button({ variant: "quiet", size: "xs" })} transition-opacity duration-250 ${narrowed ? "" : "invisible opacity-0"}`}
            onClick={() => onChange(clearedOf(filter))}
          >
            {t("activity.clearFilters")}
          </button>
        </div>
      </div>
    </div>
  );
}
